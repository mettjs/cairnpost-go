package cairnpost

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"
)

// Names of the errors CairnPost answers, as Error.Name holds them.
const (
	ErrValidation         = "validation_error"
	ErrRejectedByUpstream = "rejected_by_upstream"
	ErrRecipientSuppress  = "recipient_suppressed"
	ErrIdempotentMismatch = "invalid_idempotent_request"
	ErrIdempotentInFlight = "concurrent_idempotent_requests"
	ErrMissingKey         = "missing_api_key"
	ErrInvalidKey         = "invalid_api_key"
	ErrDomainNotAllowed   = "domain_not_allowed"
	ErrAdminKeyRequired   = "admin_key_required"
	ErrNotFound           = "not_found"
	ErrTooLarge           = "request_too_large"
	ErrRateLimited        = "rate_limit_exceeded"
	ErrUpstreamUnknown    = "upstream_unknown"
	ErrUpstreamDown       = "upstream_unavailable"
	ErrUnavailable        = "unavailable"
	ErrInternal           = "internal_error"
)

// known is every name above: an answer under one of them is CairnPost's.
var known = map[string]bool{
	ErrValidation: true, ErrRejectedByUpstream: true, ErrRecipientSuppress: true, ErrIdempotentMismatch: true,
	ErrIdempotentInFlight: true, ErrMissingKey: true, ErrInvalidKey: true, ErrDomainNotAllowed: true,
	ErrAdminKeyRequired: true, ErrNotFound: true, ErrTooLarge: true, ErrRateLimited: true,
	ErrUpstreamUnknown: true, ErrUpstreamDown: true, ErrUnavailable: true, ErrInternal: true,
}

// Error is CairnPost saying no. Status and Name say what to do:
//
//   - 422: the request is at fault and will never succeed. Do not retry.
//   - 409 invalid_idempotent_request: the key was used for another email,
//     which was sent. Do not retry.
//   - 409 concurrent_idempotent_requests, 429, 503: nothing was sent. Try
//     again, after RetryAfter when it is set.
//   - 502 upstream_unknown, and any 5xx that is not CairnPost's own: the
//     email may have gone. Try again with the same Idempotency-Key, and
//     only with it. MayHaveSent says which errors these are.
type Error struct {
	Status int
	// Name is one of the Err constants; "" when the answer was not
	// CairnPost's own (a proxy in between, say).
	Name string
	// Message is CairnPost's sentence for a person: what is wrong and
	// what to do. It never holds the email's subject or body; it may name
	// the field, or the header, that was refused. It is "" when the answer
	// was not CairnPost's own, whatever that answer said.
	Message string
	// EmailID is the email's id when the send got far enough to have one:
	// Get then says what became of it. With recipient_suppressed it is an
	// earlier try under the same Idempotency-Key that may have gone.
	EmailID string
	// RetryAfter is how long the server asked to wait; zero when it did
	// not say.
	RetryAfter time.Duration
}

func (e *Error) Error() string {
	switch {
	case e.Name == "" && e.Status >= 300 && e.Status < 400:
		return fmt.Sprintf("cairnpost: the server answered %d, a redirect: the base URL must be the server's own address", e.Status)
	case e.Name == "":
		return fmt.Sprintf("cairnpost: the server answered %d, and not as CairnPost does", e.Status)
	}
	return fmt.Sprintf("cairnpost: %s (%d): %s", e.Name, e.Status, e.Message)
}

// Retryable says that the same request may succeed later. It does not say
// that nothing was sent: ask MayHaveSent first, and when that is true
// retry only with the same Idempotency-Key.
func (e *Error) Retryable() bool {
	switch {
	case e.Name == ErrIdempotentInFlight, e.Status == http.StatusTooManyRequests:
		return true
	case e.Status == http.StatusConflict:
		return false
	}
	return e.Status >= 500
}

// problemOf reads an error answer. Only CairnPost's own fields are kept;
// a body of any other shape is not quoted.
func problemOf(res *http.Response, raw []byte) *Error {
	var body struct {
		Name    string `json:"name"`
		Message string `json:"message"`
		ID      string `json:"id"`
	}
	_ = json.Unmarshal(raw, &body)
	e := &Error{Status: res.StatusCode}
	// Only under a name of CairnPost's is the rest believed: anything else
	// at that address could say back what it was sent.
	if known[body.Name] {
		e.Name, e.Message, e.EmailID = body.Name, body.Message, body.ID
	}
	// Retry-After is seconds from CairnPost, and may be a date from a
	// proxy.
	after := res.Header.Get("Retry-After")
	if secs, err := strconv.Atoi(after); err == nil && secs > 0 {
		e.RetryAfter = time.Duration(secs) * time.Second
	} else if at, err := http.ParseTime(after); err == nil && time.Until(at) > 0 {
		e.RetryAfter = time.Until(at).Round(time.Second)
	}
	return e
}
