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

func TestAWebhookSignedByTheEndpointsSecretIsRead(t *testing.T) {
	v := verifier(t)
	for _, prefix := range []string{"webhook-", "svix-"} {
		e, err := v.Verify(headers(prefix, "dlv-1", at, signature(key, "dlv-1", at, bounce)), []byte(bounce))
		if err != nil || e.ID != "dlv-1" || e.Type != EventBounced || e.At.Second() != 5 || e.Email == nil ||
			e.Email.ID != "m1" || e.Email.To[0] != "gone@example.org" || e.Email.Bounce.Type != BouncePermanent || !e.Email.Suppressed {
			t.Fatalf("%s: %+v %v", prefix, e, err)
		}
	}
	// One of several signatures is enough: a secret being changed over is
	// signed with both.
	other := signature([]byte("another key"), "dlv-1", at, bounce)
	if _, err := v.Verify(headers("webhook-", "dlv-1", at, other+" v2,zzz "+signature(key, "dlv-1", at, bounce)), []byte(bounce)); err != nil {
		t.Fatal(err)
	}
	// The test webhook is about no email, and a type not known is passed on.
	for body, want := range map[string]string{
		`{"type":"webhook.test","created_at":"2030-01-01T00:00:05Z","data":{}}`:                                      EventTest,
		`{"type":"email.opened","created_at":"2030-01-01T00:00:05Z","data":{"email_id":"m","to":["a@example.org"]}}`: "email.opened",
	} {
		e, err := v.Verify(headers("webhook-", "t", at, signature(key, "t", at, body)), []byte(body))
		if err != nil || e.Type != want || (want == EventTest) != (e.Email == nil) {
			t.Fatalf("%s: %+v %v", want, e, err)
		}
	}
}

func TestWhatIsNotTheEndpointsIsRefused(t *testing.T) {
	v := verifier(t)
	good := signature(key, "dlv-1", at, bounce)
	late := at.Add(-DefaultTolerance - time.Second)
	early := at.Add(DefaultTolerance + time.Second)
	cases := map[string]struct {
		header http.Header
		body   string
		want   error
	}{
		"another key":           {headers("webhook-", "dlv-1", at, signature([]byte("another key"), "dlv-1", at, bounce)), bounce, ErrSignature},
		"a changed body":        {headers("webhook-", "dlv-1", at, good), bounce + " ", ErrSignature},
		"another id":            {headers("webhook-", "dlv-2", at, good), bounce, ErrSignature},
		"a moved timestamp":     {headers("webhook-", "dlv-1", at.Add(time.Second), good), bounce, ErrSignature},
		"an old webhook":        {headers("webhook-", "dlv-1", late, signature(key, "dlv-1", late, bounce)), bounce, ErrStale},
		"one from the future":   {headers("webhook-", "dlv-1", early, signature(key, "dlv-1", early, bounce)), bounce, ErrStale},
		"no headers":            {http.Header{}, bounce, ErrUnsigned},
		"a time that is none":   {http.Header{"Webhook-Id": {"a"}, "Webhook-Timestamp": {"now"}, "Webhook-Signature": {good}}, bounce, ErrUnsigned},
		"a signature of no use": {headers("webhook-", "dlv-1", at, "v1,not base64!"), bounce, ErrSignature},
		"signed, not a webhook": {headers("webhook-", "x", at, signature(key, "x", at, "hello")), "hello", ErrPayload},
	}
	for name, c := range cases {
		if e, err := v.Verify(c.header, []byte(c.body)); !errors.Is(err, c.want) || e != nil {
			t.Errorf("%s: %+v %v", name, e, err)
		}
	}
	// Within the tolerance either way is taken, and the tolerance can be set.
	edge := at.Add(-DefaultTolerance)
	if _, err := v.Verify(headers("webhook-", "e", edge, signature(key, "e", edge, bounce)), []byte(bounce)); err != nil {
		t.Errorf("at the edge of the tolerance: %v", err)
	}
	v.Tolerance = time.Second
	if _, err := v.Verify(headers("webhook-", "e", edge, signature(key, "e", edge, bounce)), []byte(bounce)); !errors.Is(err, ErrStale) {
		t.Errorf("a tolerance of a second: %v", err)
	}
	// Signed, about an email, and not saying which: refused, not handed
	// on with nothing in it.
	for name, body := range map[string]string{
		"no data":                   `{"type":"email.bounced","created_at":"2030-01-01T00:00:05Z"}`,
		"a recipient not a list":    `{"type":"email.bounced","created_at":"2030-01-01T00:00:05Z","data":{"email_id":"m","to":"a@example.org","bounce":{"type":"Permanent"}}}`,
		"a bounce of no type":       `{"type":"email.bounced","created_at":"2030-01-01T00:00:05Z","data":{"email_id":"m","to":["a@example.org"]}}`,
		"a bounce of an empty type": `{"type":"email.bounced","created_at":"2030-01-01T00:00:05Z","data":{"email_id":"m","to":["a@example.org"],"bounce":{"type":""}}}`,
		"no time it happened at":    `{"type":"email.delivered","data":{"email_id":"m","to":["a@example.org"]}}`,
		"a time that is null":       `{"type":"webhook.test","created_at":null}`,
	} {
		v.Tolerance = 0
		if e, err := v.Verify(headers("webhook-", "p", at, signature(key, "p", at, body)), []byte(body)); !errors.Is(err, ErrPayload) || e != nil {
			t.Errorf("%s: %+v %v", name, e, err)
		}
	}
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
	for _, bad := range []string{"", "c2VjcmV0", "whsec_", "whsec_not base64!"} {
		if _, err := NewVerifier(bad); !errors.Is(err, ErrSecret) {
			t.Errorf("%q was taken as a secret", bad)
		}
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

// A webhook that tells of a suppression on its own names the address and
// no email; one that names no address, or no reason, is refused. While a secret is being
// changed a webhook carries two signatures, and either secret's verifier
// takes it.
func TestASuppressionToldOnItsOwnIsRead(t *testing.T) {
	v := verifier(t)
	body := `{"type":"address.suppressed","created_at":"2030-01-01T00:00:00Z","data":{"domain":"example.com","address":"ana@example.org","reason":"bounce"}}`
	e, err := v.Verify(headers("webhook-", "dlv-1", at, signature(key, "dlv-1", at, body)), []byte(body))
	if err != nil || e.Type != EventAddressSuppressed || e.Email != nil || e.Suppression == nil ||
		*e.Suppression != (EventSuppression{Domain: "example.com", Address: "ana@example.org", Reason: ReasonBounce}) {
		t.Fatalf("%+v %v", e, err)
	}
	for _, without := range []string{
		`{"type":"address.suppressed","created_at":"2030-01-01T00:00:00Z","data":{}}`,
		`{"type":"address.suppressed","created_at":"2030-01-01T00:00:00Z","data":{"domain":"example.com"}}`,
		`{"type":"address.suppressed","created_at":"2030-01-01T00:00:00Z","data":{"domain":"example.com","address":"ana@example.org"}}`,
	} {
		if _, err := v.Verify(headers("webhook-", "dlv-2", at, signature(key, "dlv-2", at, without)), []byte(without)); err != ErrPayload {
			t.Errorf("a suppression that names no address or no reason: %v", err)
		}
	}
	other := signature([]byte("the secret before the change"), "dlv-1", at, body)
	for _, both := range []string{other + " " + signature(key, "dlv-1", at, body), signature(key, "dlv-1", at, body) + " " + other} {
		if _, err := v.Verify(headers("svix-", "dlv-1", at, both), []byte(body)); err != nil {
			t.Errorf("two signatures, one of them ours: %v", err)
		}
	}
}

// A newer server may tell of something this client has no name for. It
// is the application's to ignore, not the verifier's to refuse.
func TestATypeNotKnownIsPassedOn(t *testing.T) {
	v := verifier(t)
	for body, withEmail := range map[string]bool{
		`{"type":"email.opened","created_at":"2030-01-01T00:00:05Z","data":{"link":"x"}}`:                            false,
		`{"type":"email.opened","created_at":"2030-01-01T00:00:05Z"}`:                                                false,
		`{"type":"email.opened","created_at":"2030-01-01T00:00:05Z","data":{"email_id":"m","to":["a@example.org"]}}`: true,
		`{"type":"domain.verified","created_at":"2030-01-01T00:00:05Z","data":{"domain":"example.com"}}`:             false,
	} {
		e, err := v.Verify(headers("webhook-", "n", at, signature(key, "n", at, body)), []byte(body))
		if err != nil || e == nil || (e.Email != nil) != withEmail || e.At.IsZero() {
			t.Errorf("%s: %+v %v", body, e, err)
		}
	}
}
