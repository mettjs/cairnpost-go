package cairnpost

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// Names of the errors CairnPost answers, as Error.Name holds them.
const (
	ErrValidation         = "validation_error"
	ErrRejectedByUpstream = "rejected_by_upstream"
	// ErrSenderDomainNotReady: the mail provider refused the email, and
	// the sending domain's DNS lacks a record the provider asks. The
	// domain is likely not set up with the provider, which then refuses
	// every email from it: this is the sender failing, not one bad
	// email. A server before 0.1.1 says ErrRejectedByUpstream instead.
	ErrSenderDomainNotReady = "sender_domain_not_ready"
	ErrRecipientSuppress    = "recipient_suppressed"
	ErrIdempotentMismatch   = "invalid_idempotent_request"
	ErrIdempotentInFlight   = "concurrent_idempotent_requests"
	ErrMissingKey           = "missing_api_key"
	ErrInvalidKey           = "invalid_api_key"
	ErrDomainNotAllowed     = "domain_not_allowed"
	ErrAdminKeyRequired     = "admin_key_required"
	ErrNotFound             = "not_found"
	// ErrRouteNotFound: the server has nothing at the path asked for. The
	// base URL is wrong, or the server is older than this client.
	ErrRouteNotFound = "route_not_found"
	// ErrMethodNotAllowed: the path is there and does not take the
	// method. The server is older or newer than this client.
	ErrMethodNotAllowed = "method_not_allowed"
	ErrTooLarge         = "request_too_large"
	// ErrRequestIncomplete: the request did not arrive whole in time.
	// Nothing was done with it; try again.
	ErrRequestIncomplete = "request_incomplete"
	ErrRateLimited       = "rate_limit_exceeded"
	ErrUpstreamUnknown   = "upstream_unknown"
	ErrUpstreamDown      = "upstream_unavailable"
	ErrUnavailable       = "unavailable"
	// ErrNotScheduled: the email to cancel or move does not wait to be
	// sent: it is being sent, was sent, or was cancelled.
	ErrNotScheduled = "not_scheduled"
	// ErrScheduleFull: the emails waiting to be sent hold as much as the
	// server keeps. Nothing was kept or sent.
	ErrScheduleFull = "schedule_full"
	// ErrNotStarted: a batch ended before this email's turn. Nothing of
	// it was sent.
	ErrNotStarted = "not_started"
	// ErrNameTaken: the sending domain already has a template of that
	// name.
	ErrNameTaken = "name_taken"
	// ErrChangedMeanwhile: what was to be changed was changed by others
	// at the same time. Read it and try again.
	ErrChangedMeanwhile = "changed_meanwhile"
	ErrInternal         = "internal_error"
)

// known is every name above: an answer under one of them is CairnPost's.
var known = map[string]bool{
	ErrValidation: true, ErrRejectedByUpstream: true, ErrSenderDomainNotReady: true, ErrRecipientSuppress: true, ErrIdempotentMismatch: true,
	ErrIdempotentInFlight: true, ErrMissingKey: true, ErrInvalidKey: true, ErrDomainNotAllowed: true,
	ErrAdminKeyRequired: true, ErrNotFound: true, ErrRouteNotFound: true, ErrMethodNotAllowed: true,
	ErrTooLarge: true, ErrRequestIncomplete: true, ErrRateLimited: true,
	ErrUpstreamUnknown: true, ErrUpstreamDown: true, ErrUnavailable: true, ErrInternal: true,
	ErrNotStarted: true, ErrNotScheduled: true, ErrScheduleFull: true, ErrNameTaken: true, ErrChangedMeanwhile: true,
}

// Error is CairnPost saying no. Status and Name say what to do:
//
//   - 422: the request is at fault and will never succeed. Do not retry.
//     Under sender_domain_not_ready no email from that domain will be
//     sent until the domain is set up with the mail provider.
//   - 409 invalid_idempotent_request: the key was used for another email,
//     which was sent. Do not retry.
//   - 409 concurrent_idempotent_requests, 408 request_incomplete, 429,
//     503: nothing was sent. Try again, after RetryAfter when it is set.
//   - 409 changed_meanwhile: nothing was changed. Read it and try again.
//   - 502 upstream_unknown, and any 5xx or 408 that is not CairnPost's
//     own: the email may have gone. Try again with the same Idempotency-Key, and
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
	// not say. It is whole seconds, at least one and never more than
	// about 31 years: a wait outside that is read as none.
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
	case e.Name == ErrIdempotentInFlight, e.Status == http.StatusTooManyRequests, e.Status == http.StatusRequestTimeout:
		// The last is a request that did not arrive whole in time. Under
		// CairnPost's own name nothing was done with it; MayHaveSent says
		// what one from something in between leaves open.
		return true
	case e.Name == ErrChangedMeanwhile:
		// Changed by others at the same time: read it and try again.
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
	e.RetryAfter = waitOf(res)
	return e
}
