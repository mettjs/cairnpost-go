package cairnpost

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// DefaultTolerance is how far a webhook's timestamp may be from now.
const DefaultTolerance = 5 * time.Minute

// What Verify refuses.
var (
	// ErrSecret: the signing secret is not one CairnPost gives, or the
	// Verifier was not made by NewVerifier.
	ErrSecret = errors.New("cairnpost: the signing secret must be the whsec_ value the server gave for the endpoint")
	// ErrUnsigned: the request lacks the webhook's id, timestamp or
	// signature headers.
	ErrUnsigned = errors.New("cairnpost: the webhook's signature headers are missing")
	// ErrSignature: no signature matches. The request is not from
	// CairnPost, was changed on the way, was signed for another endpoint,
	// or the endpoint's secret was changed and this one no longer signs.
	ErrSignature = errors.New("cairnpost: the webhook's signature does not match")
	// ErrStale: the timestamp is too far from now. A webhook caught on
	// the way and replayed later is refused by this.
	ErrStale = errors.New("cairnpost: the webhook's timestamp is too far from now")
	// ErrPayload: signed by CairnPost, but not a webhook this client can
	// read.
	ErrPayload = errors.New("cairnpost: the webhook's body could not be read")
)

// Verifier checks that a webhook is CairnPost's. Make one with
// NewVerifier: the zero value has no secret and verifies nothing.
type Verifier struct {
	key []byte
	// Tolerance is how far the timestamp may be from now, either way.
	// Zero means DefaultTolerance.
	Tolerance time.Duration
	// Now is the clock; nil means time.Now.
	Now func() time.Time
}

// NewVerifier returns a verifier for an endpoint's signing secret: the
// "whsec_..." value CreateWebhook answered or, once the secret was
// changed, the one RotateWebhookSecret did. Each is said only once.
func NewVerifier(secret string) (*Verifier, error) {
	encoded, ok := strings.CutPrefix(strings.TrimSpace(secret), "whsec_")
	key, err := base64.StdEncoding.DecodeString(encoded)
	if !ok || err != nil || len(key) == 0 {
		return nil, ErrSecret
	}
	return &Verifier{key: key}, nil
}

// Verify checks a webhook and reads it. body must be the request's body
// exactly as it arrived: read it once, verify, and only then decode. An
// error means the request must not be trusted; answer it 400 or 401.
//
// In the Event answered, Email is set for every type that starts with
// "email.", and Email.Bounce for email.bounced; Suppression is set for
// address.suppressed; both are nil for webhook.test.
//
// CairnPost signs as Standard Webhooks and Svix do, and sends the headers
// under both names; either set is accepted.
func (v *Verifier) Verify(header http.Header, body []byte) (*Event, error) {
	if v == nil || len(v.key) == 0 {
		// A verifier not made by NewVerifier has no key, and would take
		// anything signed with none.
		return nil, ErrSecret
	}
	id, stamp, signatures := signed(header)
	if id == "" || stamp == "" || signatures == "" {
		return nil, ErrUnsigned
	}
	seconds, err := strconv.ParseInt(stamp, 10, 64)
	if err != nil {
		return nil, ErrUnsigned
	}
	mac := hmac.New(sha256.New, v.key)
	mac.Write([]byte(id + "." + stamp + "."))
	mac.Write(body)
	want := mac.Sum(nil)
	matched := false
	for _, candidate := range strings.Fields(signatures) {
		encoded, isV1 := strings.CutPrefix(candidate, "v1,")
		got, err := base64.StdEncoding.DecodeString(encoded)
		if isV1 && err == nil && hmac.Equal(got, want) {
			matched = true
		}
	}
	if !matched {
		return nil, ErrSignature
	}
	// The time is checked after the signature: only a signed timestamp
	// means anything.
	now, tolerance := time.Now, v.Tolerance
	if v.Now != nil {
		now = v.Now
	}
	if tolerance == 0 {
		tolerance = DefaultTolerance
	}
	if age := now().Sub(time.Unix(seconds, 0)); age > tolerance || age < -tolerance {
		return nil, ErrStale
	}
	// The wire form: the email is under "data", which is empty for a
	// webhook about no email.
	var wire struct {
		Type      string          `json:"type"`
		CreatedAt time.Time       `json:"created_at"`
		Data      json.RawMessage `json:"data"`
	}
	if json.Unmarshal(body, &wire) != nil || wire.Type == "" {
		return nil, ErrPayload
	}
	e := &Event{ID: id, Type: wire.Type, At: wire.CreatedAt}
	var email EventEmail
	readable := json.Unmarshal(wire.Data, &email) == nil && email.ID != "" && len(email.To) > 0
	switch {
	case readable:
		e.Email = &email
	case strings.HasPrefix(wire.Type, "email."):
		// An event about an email must say which: one that does not is
		// refused here, not handed on with nothing in it.
		return nil, ErrPayload
	}
	if e.Type == EventBounced && e.Email.Bounce == nil {
		return nil, ErrPayload
	}
	if e.Type == EventAddressSuppressed {
		// It must say which address and why, as an email's event must
		// say which email.
		var s EventSuppression
		if json.Unmarshal(wire.Data, &s) != nil || s.Domain == "" || s.Address == "" || s.Reason == "" {
			return nil, ErrPayload
		}
		e.Suppression = &s
	}
	return e, nil
}

// signed reads the signature headers under whichever names they came.
func signed(h http.Header) (id, stamp, signatures string) {
	for _, prefix := range []string{"webhook-", "svix-"} {
		id, stamp, signatures = h.Get(prefix+"id"), h.Get(prefix+"timestamp"), h.Get(prefix+"signature")
		if id != "" && stamp != "" && signatures != "" {
			return id, stamp, signatures
		}
	}
	return id, stamp, signatures
}
