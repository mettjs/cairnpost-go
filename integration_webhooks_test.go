//go:build integration

package cairnpost

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

// The verifier is held against the server's own signer: an endpoint is
// made, the server sends it a test webhook, and what arrives is verified
// with the secret the server gave.
func TestWebhooksAgainstARealServer(t *testing.T) {
	_, admin := live(t)
	type arrival struct {
		header http.Header
		body   []byte
	}
	got := make(chan arrival, 4)
	receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		got <- arrival{r.Header, body}
	}))
	defer receiver.Close()
	run := fmt.Sprintf("hooks-%d.example.com", time.Now().UnixNano())
	w, err := admin.CreateWebhook(ctx, NewWebhook{URL: receiver.URL + "/hook", Domain: run,
		Events: []string{EventBounced, EventComplained}, Description: "the client's test"})
	if err != nil || !strings.HasPrefix(w.Secret, "whsec_") || !w.Enabled || len(w.Events) != 2 {
		t.Fatalf("%+v %v (the server needs CAIRNPOST_WEBHOOK_ALLOW_PRIVATE=true)", w, err)
	}
	defer admin.DeleteWebhook(ctx, w.ID)

	res, err := admin.TestWebhook(ctx, w.ID)
	if err != nil || !res.Delivered || res.Status != 200 {
		t.Fatalf("%+v %v", res, err)
	}
	a := <-got
	v, err := NewVerifier(w.Secret)
	if err != nil {
		t.Fatal(err)
	}
	e, err := v.Verify(a.header, a.body)
	if err != nil || e.Type != EventTest || e.ID == "" || e.Email != nil || time.Since(e.At) > time.Minute {
		t.Fatalf("the server's own webhook: %+v %v", e, err)
	}
	// The same arrival, a byte changed or under another secret, is refused.
	if _, err := v.Verify(a.header, append([]byte(" "), a.body...)); !errors.Is(err, ErrSignature) {
		t.Fatalf("a changed body: %v", err)
	}
	stranger, _ := NewVerifier("whsec_YW5vdGhlciBlbmRwb2ludCdzIHNlY3JldA==")
	if _, err := stranger.Verify(a.header, a.body); !errors.Is(err, ErrSignature) {
		t.Fatalf("another endpoint's secret: %v", err)
	}

	// The secret is said once; the rest is read, changed and removed.
	read, err := admin.Webhook(ctx, w.ID)
	if err != nil || read.Secret != "" || read.Domain != run || read.Description != "the client's test" {
		t.Fatalf("%+v %v", read, err)
	}
	list, err := admin.Webhooks(ctx)
	if err != nil || len(list) == 0 {
		t.Fatalf("%d %v", len(list), err)
	}
	off, every, note := false, []string{}, "changed"
	changed, err := admin.UpdateWebhook(ctx, w.ID, WebhookChange{Enabled: &off, Events: &every, Description: &note})
	if err != nil || changed.Enabled || len(changed.Events) != 0 || changed.Description != "changed" || changed.URL != w.URL {
		t.Fatalf("%+v %v", changed, err)
	}
	if page, err := admin.Deliveries(ctx, w.ID, DeliveryQuery{State: DeliveryFailed, Limit: 5}); err != nil || len(page.Data) != 0 {
		t.Fatalf("%+v %v", page, err)
	}
	for _, err := range admin.AllDeliveries(ctx, w.ID, "") {
		t.Fatalf("an endpoint that was owed nothing: %v", err)
	}
	if err := admin.RetryDelivery(ctx, w.ID, "00000000-0000-7000-8000-000000000000"); !IsNotFound(err) {
		t.Fatalf("%v", err)
	}
	if _, err := admin.CreateWebhook(ctx, NewWebhook{URL: "not a url"}); named(err) != ErrValidation {
		t.Fatalf("%v", err)
	}
	if err := admin.DeleteWebhook(ctx, w.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := admin.Webhook(ctx, w.ID); !IsNotFound(err) {
		t.Fatalf("after it was removed: %v", err)
	}
}

// A changed secret verifies beside the one before it: the server signs
// with both, and a Verifier made with either takes the same webhook. An
// endpoint that is not paused is resumed to no effect, and a base URL
// that is wrong is told apart from a record that is missing.
func TestRotationResumeAndAWrongAddressAgainstARealServer(t *testing.T) {
	_, admin := live(t)
	type arrival struct {
		header http.Header
		body   []byte
	}
	got := make(chan arrival, 4)
	receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		got <- arrival{r.Header, body}
	}))
	defer receiver.Close()
	w, err := admin.CreateWebhook(ctx, NewWebhook{URL: receiver.URL + "/hook", Description: "the client's rotation test",
		Events: []string{EventAddressSuppressed}})
	if err != nil || len(w.Events) != 1 || w.Events[0] != EventAddressSuppressed {
		t.Fatalf("%+v %v (the server needs CAIRNPOST_WEBHOOK_ALLOW_PRIVATE=true)", w, err)
	}
	defer admin.DeleteWebhook(ctx, w.ID)
	changed, err := admin.RotateWebhookSecret(ctx, w.ID)
	if err != nil || !strings.HasPrefix(changed.Secret, "whsec_") || changed.Secret == w.Secret ||
		changed.PreviousSecretUntil == nil || !changed.PreviousSecretUntil.After(time.Now()) {
		t.Fatalf("%+v %v", changed, err)
	}
	if again, err := admin.Webhook(ctx, w.ID); err != nil || again.Secret != "" || again.PreviousSecretUntil == nil {
		t.Fatalf("a later read: %+v %v", again, err)
	}
	if res, err := admin.TestWebhook(ctx, w.ID); err != nil || !res.Delivered {
		t.Fatalf("%+v %v", res, err)
	}
	select {
	case a := <-got:
		for which, secret := range map[string]string{"the new secret": changed.Secret, "the old secret": w.Secret} {
			v, err := NewVerifier(secret)
			if err != nil {
				t.Fatal(err)
			}
			if e, err := v.Verify(a.header, a.body); err != nil || e.Type != EventTest {
				t.Errorf("%s does not verify the webhook: %v", which, err)
			}
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the test webhook did not arrive")
	}
	if resumed, err := admin.ResumeWebhook(ctx, w.ID); err != nil || resumed.PausedAt != nil || resumed.ID != w.ID {
		t.Fatalf("resuming one that is not paused: %+v %v", resumed, err)
	}

	// The server's own word for each: a record that is not there, and a
	// path that is not.
	if _, err := admin.Get(ctx, "01900000-0000-7000-8000-000000000000"); !IsNotFound(err) || IsWrongAddress(err) {
		t.Fatalf("an email that is not there: %v", err)
	}
	astray, err := New(strings.TrimRight(os.Getenv("CAIRNPOST_URL"), "/")+"/not-the-server", os.Getenv("CAIRNPOST_ADMIN_KEY"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := astray.Get(ctx, "01900000-0000-7000-8000-000000000000"); IsNotFound(err) || !IsWrongAddress(err) || named(err) != ErrRouteNotFound {
		t.Fatalf("a base URL with a wrong path: %v", err)
	}
	if _, err := astray.Health(ctx); !IsWrongAddress(err) {
		t.Fatalf("health at a wrong path: %v", err)
	}
	if _, err := astray.Captured(ctx); !IsWrongAddress(err) || errors.Is(err, ErrNotCapturing) {
		t.Fatalf("captured emails at a wrong path: %v", err)
	}
}
