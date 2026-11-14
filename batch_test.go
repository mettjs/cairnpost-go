package cairnpost

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"testing"
	"time"
)

var two = Batch{IdempotencyKey: "run-7", Emails: []Email{
	{From: "app@example.com", To: []string{"ana@example.org"}, Subject: "Hi", Text: "Body"},
	{From: "app@example.com", To: []string{"ben@example.org"}, Subject: "Hi", Text: "Body"}}}

func TestSendBatchAnswersEachEmailInItsPlace(t *testing.T) {
	s, c := serving(t, func(w http.ResponseWriter, _ int) {
		w.Header().Set("Retry-After", "30")
		w.WriteHeader(207)
		io.WriteString(w, `{"data":[{"id":"msg-1","suppressed":["cy@example.org"]},
			{"error":{"statusCode":429,"name":"rate_limit_exceeded","message":"Later."}}]}`)
	})
	got, err := c.SendBatch(ctx, two)
	if err != nil || len(got) != 2 || got[0].Err != nil || got[0].Sent.ID != "msg-1" || len(got[0].Sent.Suppressed) != 1 || Failed(got) != 1 {
		t.Fatalf("%+v %v", got, err)
	}
	var problem *Error
	if !errors.As(got[1].Err, &problem) || problem.Name != ErrRateLimited || !problem.Retryable() ||
		problem.RetryAfter != 30*time.Second || MayHaveSent(got[1].Err) || got[1].Sent != nil {
		t.Fatalf("the second email: %+v", got[1].Err)
	}
	seen := s.seen[0]
	var list []map[string]any
	if err := json.Unmarshal([]byte(seen.body), &list); err != nil || seen.path != "/emails/batch" || seen.idem != "run-7" ||
		len(list) != 2 || len(list[1]) != 4 || list[1]["subject"] != "Hi" {
		t.Fatalf("%+v %v", seen, err)
	}
}

func TestAnEmailWhoseTurnDidNotComeWasNotSent(t *testing.T) {
	_, c := serve(t, 207, `{"data":[{"id":"msg-1"},
		{"error":{"statusCode":503,"name":"not_started","message":"Send it again."}}]}`)
	got, err := c.SendBatch(ctx, two)
	if err != nil || MayHaveSent(got[1].Err) || !got[1].Err.(*Error).Retryable() {
		t.Fatalf("%+v %v", got, err)
	}
}

func TestABatchRefusedWholeIsAnError(t *testing.T) {
	_, c := serve(t, 422, `{"statusCode":422,"name":"validation_error","message":"Email 2 of the batch: no. Nothing was sent."}`)
	got, err := c.SendBatch(ctx, two)
	var problem *Error
	if got != nil || !errors.As(err, &problem) || problem.Name != ErrValidation || MayHaveSent(err) {
		t.Fatalf("%+v %v", got, err)
	}
}

func TestABatchIsNotSentWithAKeyOnAnEmail(t *testing.T) {
	s, c := serve(t, 200, `{"data":[]}`)
	own := Batch{Emails: []Email{{From: "a@example.com", To: []string{"b@example.org"}, Subject: "s", Text: "t", IdempotencyKey: "mine"}}}
	if _, err := c.SendBatch(ctx, own); err == nil || len(s.seen) != 0 {
		t.Fatalf("%v, %d requests", err, len(s.seen))
	}
	if _, err := c.SendBatch(ctx, Batch{IdempotencyKey: "a\nb", Emails: two.Emails}); err == nil || len(s.seen) != 0 {
		t.Fatalf("%v, %d requests", err, len(s.seen))
	}
}

// An answer that does not say what became of each email is not
// CairnPost's: any of them may have gone.
func TestABatchAnswerThatIsNotCairnPostsIsNotTrusted(t *testing.T) {
	for _, body := range []string{`{}`, `{"data":[{"id":"msg-1"}]}`, `{"data":[{"id":"msg-1"},{}]}`,
		`{"data":[{"id":"msg-1"},{"error":{"statusCode":500,"name":"made_up","message":"x"}}]}`} {
		_, c := serve(t, 200, body)
		got, err := c.SendBatch(ctx, two)
		var transport *TransportError
		if got != nil || !errors.As(err, &transport) || !MayHaveSent(err) {
			t.Errorf("%s: %+v %v", body, got, err)
		}
	}
}
