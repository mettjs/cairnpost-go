package cairnpost

import (
	"errors"
	"net/http"
)

// TransportError is a call that got no answer it could read: the server
// was not reached, the connection broke, or the time ran out. For a send
// it leaves open whether the email went.
type TransportError struct{ cause error }

func (e *TransportError) Error() string {
	return "cairnpost: no answer from the server: " + e.cause.Error()
}

// Unwrap answers what went wrong underneath (a timeout, a refused
// connection), so errors.Is(err, context.DeadlineExceeded) works.
func (e *TransportError) Unwrap() error { return e.cause }

// MayHaveSent says whether, after a failed Send, the email may have gone
// all the same: the mail provider's answer was lost (upstream_unknown),
// the server's own answer never came or came from something in between
// (a proxy's 502), or the server failed after it began. Then the only
// safe retry is one with the same Idempotency-Key. It is also true of a
// 408 that is not CairnPost's own, which a proxy may answer after it
// passed the request on, and of a
// recipient_suppressed that carries an EmailID: nothing was sent now, but
// that earlier try may have gone. That one is not to be retried, since
// the address is suppressed: read the EmailID with Get instead. After any
// other error nothing was sent.
func MayHaveSent(err error) bool {
	var (
		problem   *Error
		transport *TransportError
	)
	switch {
	case errors.As(err, &transport):
		return true
	case !errors.As(err, &problem):
		return false
	case problem.Status == http.StatusRequestTimeout:
		// CairnPost's own 408 is a request it never had whole. One from
		// something in between may come after the server took the email.
		return problem.Name != ErrRequestIncomplete
	case problem.Name == ErrRecipientSuppress:
		// Refused now; but an id with it is an earlier try under the same
		// key whose answer never came.
		return problem.EmailID != ""
	case problem.Status >= 500:
		// The server's send does not stop when its answer is lost on the
		// way, so only its own word that nothing went counts.
		return problem.Name != ErrUpstreamDown && problem.Name != ErrUnavailable && problem.Name != ErrNotStarted &&
			problem.Name != ErrScheduleFull
	}
	return false
}

// IsNotFound says that what was asked for is not there: no such email,
// endpoint, delivery or suppression, or not one this key may see. It is
// CairnPost's own word for it: a path the server does not have, or a 404
// from something else at that address, is not this. IsWrongAddress says
// those.
//
// It needs the server's own answer to arrive. Behind a proxy that puts
// its own page in place of every 404, no record is ever said to be
// missing and every such answer reads as a wrong address: have the proxy
// pass the server's error answers through.
func IsNotFound(err error) bool {
	var problem *Error
	return errors.As(err, &problem) && problem.Status == http.StatusNotFound && problem.Name == ErrNotFound
}

// IsWrongAddress says that the request did not reach a route of
// CairnPost's: the server has nothing at that path or does not take that
// method there, or something else answered in its place with a 404, a
// 405 or a redirect. The base URL given to New is most likely wrong;
// nothing was sent, and no record is known to be missing.
func IsWrongAddress(err error) bool {
	var problem *Error
	if !errors.As(err, &problem) {
		return false
	}
	switch problem.Name {
	case ErrRouteNotFound, ErrMethodNotAllowed:
		return true
	case "":
		return problem.Status == http.StatusNotFound || problem.Status == http.StatusMethodNotAllowed ||
			(problem.Status >= 300 && problem.Status < 400)
	}
	return false
}
