package cairnpost

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"testing"
	"time"
)

const bounce = `{"type":"email.bounced","created_at":"2030-01-01T00:00:05Z","data":{"email_id":"m1","from":"app@example.com",` +
	`"to":["gone@example.org"],"created_at":"2030-01-01T00:00:00Z","code":"5.1.1","reason":"permanent_failure",` +
	`"bounce":{"type":"Permanent"},"suppressed":true}}`

var (
	key    = []byte("a signing key of thirty-two bytes")
	secret = "whsec_" + base64.StdEncoding.EncodeToString(key)
	at     = time.Date(2030, 1, 1, 0, 0, 10, 0, time.UTC)
)

// signature is what a sender holding k would put in the header.
func signature(k []byte, id string, when time.Time, body string) string {
	mac := hmac.New(sha256.New, k)
	mac.Write([]byte(id + "." + strconv.FormatInt(when.Unix(), 10) + "." + body))
	return "v1," + base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

func headers(prefix, id string, when time.Time, sig string) http.Header {
	h := http.Header{}
	h.Set(prefix+"id", id)
	h.Set(prefix+"timestamp", strconv.FormatInt(when.Unix(), 10))
	h.Set(prefix+"signature", sig)
	return h
}

func verifier(t *testing.T) *Verifier {
	t.Helper()
	v, err := NewVerifier(secret)
	if err != nil {
		t.Fatal(err)
	}
	v.Now = func() time.Time { return at }
	return v
}

// Which webhooks are read and which refused is in the shared cases
// (cases_webhooks_test.go). What follows is this client's own.

func TestAVerifierWithNoSecretTakesNothing(t *testing.T) {
	// A verifier not made by NewVerifier has no key: it takes nothing,
	// not even what was signed with no key at all.
	var zero Verifier
	zero.Now = func() time.Time { return at }
	forged := headers("webhook-", "dlv-1", at, signature(nil, "dlv-1", at, bounce))
	if e, err := zero.Verify(forged, []byte(bounce)); !errors.Is(err, ErrSecret) || e != nil {
		t.Errorf("a verifier with no secret took a webhook: %+v %v", e, err)
	}
	if _, err := (*Verifier)(nil).Verify(forged, []byte(bounce)); !errors.Is(err, ErrSecret) {
		t.Errorf("a nil verifier: %v", err)
	}
}

// A verified event passed on as JSON, to a queue say, comes back whole.
func TestAnEventKeepsItsIDAndEmailAsJSON(t *testing.T) {
	e, err := verifier(t).Verify(headers("webhook-", "dlv-1", at, signature(key, "dlv-1", at, bounce)), []byte(bounce))
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(e)
	if err != nil {
		t.Fatal(err)
	}
	var back Event
	if err := json.Unmarshal(raw, &back); err != nil || back.ID != "dlv-1" || back.Type != EventBounced || !back.At.Equal(e.At) ||
		back.Email == nil || back.Email.ID != "m1" || back.Email.Bounce.Type != BouncePermanent || !back.Email.Suppressed {
		t.Fatalf("%s -> %+v %v", raw, back, err)
	}
}
