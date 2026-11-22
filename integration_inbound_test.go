//go:build integration

package cairnpost

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// handOver gives the server an email as its mail provider would: signed
// with CAIRNPOST_INBOUND_SECRET, which the server must be started with.
func handOver(t *testing.T, raw []byte, to, from string) {
	t.Helper()
	secret := os.Getenv("CAIRNPOST_INBOUND_SECRET")
	if secret == "" {
		t.Fatal("set CAIRNPOST_INBOUND_SECRET to the one the server was started with")
	}
	stamp := strconv.FormatInt(time.Now().Unix(), 10)
	sum := sha256.Sum256(raw)
	digest := hex.EncodeToString(sum[:])
	mac := hmac.New(sha256.New, []byte(secret))
	for _, part := range []string{stamp, to, from, digest} {
		mac.Write(binary.BigEndian.AppendUint64(nil, uint64(len(part))))
		mac.Write([]byte(part))
	}
	req, _ := http.NewRequest(http.MethodPost, strings.TrimRight(os.Getenv("CAIRNPOST_URL"), "/")+"/inbound", bytes.NewReader(raw))
	for name, value := range map[string]string{"Content-Type": "message/rfc822", "X-CairnPost-Timestamp": stamp, "X-CairnPost-To": to,
		"X-CairnPost-From": from, "X-CairnPost-Digest": digest, "X-CairnPost-Signature": "v1=" + hex.EncodeToString(mac.Sum(nil))} {
		req.Header.Set(name, value)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil || res.StatusCode < 200 || res.StatusCode > 299 {
		t.Fatalf("the server did not take the email handed over: %v %v", res, err)
	}
	res.Body.Close()
}

// An email that comes for an address is told to the endpoint in a
// webhook the verifier takes, is read with what it says, and a try that
// failed is tried again when asked.
func TestAnEmailThatComesAgainstARealServer(t *testing.T) {
	send, admin := live(t)
	type arrival struct {
		header http.Header
		body   []byte
	}
	got := make(chan arrival, 8)
	var answer atomic.Int32
	answer.Store(500) // the first try fails, so that there is one to try again
	receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		got <- arrival{r.Header, body}
		w.WriteHeader(int(answer.Load()))
	}))
	defer receiver.Close()
	w, err := admin.CreateWebhook(ctx, NewWebhook{URL: receiver.URL + "/hook", Events: []string{EventReceived}})
	if err != nil {
		t.Fatalf("%v (the server needs CAIRNPOST_WEBHOOK_ALLOW_PRIVATE=true)", err)
	}
	defer admin.DeleteWebhook(ctx, w.ID)
	v, err := NewVerifier(w.Secret)
	if err != nil {
		t.Fatal(err)
	}
	to := fmt.Sprintf("help@run-%d.example.com", time.Now().UnixNano())
	raw := []byte("From: Ana <ana@example.org>\r\nTo: " + to + "\r\nSubject: Hello\r\nDate: Tue, 01 Jan 2030 00:00:00 +0000\r\n" +
		"Message-ID: <1@example.org>\r\nMIME-Version: 1.0\r\nContent-Type: multipart/mixed; boundary=b\r\n\r\n--b\r\n" +
		"Content-Type: text/plain; charset=utf-8\r\n\r\nhello\r\n--b\r\nContent-Type: application/octet-stream\r\n" +
		"Content-Disposition: attachment; filename=a.bin\r\nContent-Transfer-Encoding: base64\r\n\r\nAAH/\r\n--b--\r\n")
	handOver(t, raw, to, "ana@example.org")
	arrived := func() *Event {
		t.Helper()
		select {
		case a := <-got:
			e, err := v.Verify(a.header, a.body)
			if err != nil || e.Type != EventReceived || e.Email != nil || e.Received == nil || e.Received.From != "ana@example.org" ||
				len(e.Received.To) != 1 || e.Received.To[0] != to || e.Received.ReceivedAt.IsZero() || e.At.IsZero() {
				t.Fatalf("the webhook about the email that came: %+v %v", e, err)
			}
			return e
		case <-time.After(20 * time.Second):
			t.Fatal("the webhook about the email that came did not arrive")
			return nil
		}
	}
	told := arrived()
	id := told.Received.ID
	defer admin.DeleteReceived(ctx, id)
	r, err := admin.Received(ctx, id)
	if err != nil || r.To != to || r.From != "ana@example.org" || r.Size != len(raw) || r.Content == nil || r.Content.Subject != "Hello" ||
		strings.TrimSpace(r.Content.Text) != "hello" || len(r.Content.Attachments) != 1 || r.Content.Attachments[0].Filename != "a.bin" {
		t.Fatalf("%+v %v", r, err)
	}
	if whole, err := admin.ReceivedRaw(ctx, id); err != nil || !bytes.Equal(whole, raw) {
		t.Fatalf("as it came: %d bytes %v", len(whole), err)
	}
	if file, err := admin.ReceivedFile(ctx, id, 1); err != nil || !bytes.Equal(file, []byte{0, 1, 255}) {
		t.Fatalf("its file: %v %v", file, err)
	}
	if page, err := admin.ReceivedList(ctx, ReceivedQuery{Limit: 1}); err != nil || len(page.Data) != 1 {
		t.Fatalf("%+v %v", page, err)
	}
	// A send key reads none of it.
	if _, err := send.Received(ctx, id); named(err) != ErrAdminKeyRequired {
		t.Fatalf("a send key read an email that came: %v", err)
	}

	// The try that failed is on record, and is tried again when asked.
	var owed *Delivery
	for until := time.Now().Add(20 * time.Second); owed == nil && time.Now().Before(until); time.Sleep(200 * time.Millisecond) {
		page, err := admin.Deliveries(ctx, w.ID, DeliveryQuery{State: DeliveryPending})
		if err != nil {
			t.Fatal(err)
		}
		for _, d := range page.Data {
			if d.InboundID == id && d.Attempts > 0 {
				owed = &d
			}
		}
	}
	if owed == nil || owed.ID != told.ID || owed.Type != EventReceived || owed.LastStatus != 500 || owed.NextAttemptAt == nil {
		t.Fatalf("the failed try on record: %+v", owed)
	}
	answer.Store(204)
	if err := admin.RetryDelivery(ctx, w.ID, owed.ID); err != nil {
		t.Fatal(err)
	}
	if again := arrived(); again.ID != told.ID {
		t.Fatalf("the same webhook under another id: %s, %s", again.ID, told.ID)
	}
	if err := admin.DeleteReceived(ctx, id); err != nil {
		t.Fatal(err)
	}
	if _, err := admin.Received(ctx, id); !IsNotFound(err) {
		t.Fatalf("a deleted email: %v", err)
	}
}
