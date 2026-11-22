package cairnpost

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"
)

// Batch is several emails to send in one request.
type Batch struct {
	Emails []Email
	// IdempotencyKey makes the batch safe to send again as it was: the
	// server remembers each email under the key and its place, so a
	// second try sends only what did not go. Send the same emails in the
	// same order. An email's own IdempotencyKey is not used in a batch.
	// A key is printable ASCII with no space at either end; any other is
	// refused before anything is sent.
	IdempotencyKey string
}

// BatchResult is what became of one email of a batch: Sent, or Err.
type BatchResult struct {
	Sent *Sent
	// Err is an *Error, as Send would have answered for this email alone:
	// Retryable and MayHaveSent read it the same way.
	Err error
}

// SendBatch sends several emails and answers what became of each, in
// their order.
//
// The server checks every email before it sends any: one that could
// never be sent refuses the batch, and SendBatch answers that *Error
// with nothing sent. A nil error means the batch was taken, not that
// every email went: read each result. Failed counts those that did not.
//
// After a TransportError any of the emails may have gone. Send the same
// batch again with the same IdempotencyKey, and only with it.
func (c *Client) SendBatch(ctx context.Context, b Batch) ([]BatchResult, error) {
	body := make([]wire, len(b.Emails))
	for i, e := range b.Emails {
		if e.IdempotencyKey != "" {
			return nil, fmt.Errorf("cairnpost: email %d has an IdempotencyKey of its own: a batch has one, on the Batch; nothing was sent", i+1)
		}
		body[i] = wireOf(e)
	}
	k := call{method: http.MethodPost, path: []string{"emails", "batch"}, body: body}
	if b.IdempotencyKey != "" {
		if !headerSafe(b.IdempotencyKey) {
			return nil, errors.New("cairnpost: the IdempotencyKey must be printable ASCII, with no space at either end; nothing was sent")
		}
		k.header = http.Header{"Idempotency-Key": {b.IdempotencyKey}}
	}
	var out batchAnswer
	if err := c.do(ctx, k, &out); err != nil {
		return nil, err
	}
	if len(out.Data) != len(b.Emails) {
		// Not CairnPost's answer: which emails went is not known.
		return nil, &TransportError{cause: errNotCairnPost}
	}
	results := make([]BatchResult, len(out.Data))
	for i, d := range out.Data {
		switch {
		case d.Error != nil && d.Error.Status >= 400 && d.Error.Status <= 599:
			problem := &Error{Status: d.Error.Status}
			// Under a name this client does not know, only the status
			// is believed, as for an answer that is not CairnPost's.
			if known[d.Error.Name] {
				problem.Name, problem.Message, problem.EmailID = d.Error.Name, d.Error.Message, d.Error.ID
				problem.RetryAfter = waitSaid(d.Error.Wait)
			}
			results[i].Err = problem
		case d.Error == nil && d.ID != "" && d.At.IsZero() == b.Emails[i].ScheduledAt.IsZero():
			results[i].Sent = &Sent{ID: d.ID, Suppressed: d.Suppressed, ScheduledAt: d.At}
		default:
			return nil, &TransportError{cause: errNotCairnPost}
		}
	}
	return results, nil
}

// batchAnswer is the answer to a batch: one item for each email.
type batchAnswer struct {
	Data []struct {
		ID         string    `json:"id"`
		Suppressed []string  `json:"suppressed"`
		At         time.Time `json:"scheduled_at"`
		Error      *struct {
			Status  int    `json:"statusCode"`
			Name    string `json:"name"`
			Message string `json:"message"`
			ID      string `json:"id"`
			// Read as it was written: a wait that is no number is no
			// wait, not an answer that cannot be read.
			Wait json.RawMessage `json:"retry_after"`
		} `json:"error"`
	} `json:"data"`
}

// SendBatch holds each item to what it must say, having the emails to
// hold them against.
func (b *batchAnswer) whole() bool { return b.Data != nil }

// Failed counts the emails of a batch that did not go.
func Failed(results []BatchResult) int {
	n := 0
	for _, r := range results {
		if r.Err != nil {
			n++
		}
	}
	return n
}
