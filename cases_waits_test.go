package cairnpost

import (
	"errors"
	"io"
	"net/http"
	"testing"
	"time"
)

type waitCase struct {
	Given   string   `json:"given"`
	Seconds *float64 `json:"seconds"`
}

func (c waitCase) wait() time.Duration {
	if c.Seconds == nil {
		return 0
	}
	return time.Duration(*c.Seconds * float64(time.Second))
}

// readCase is a value read at a time the case says.
type readCase struct {
	Now *int64 `json:"now"`
	waitCase
}

// A caller sleeps for the wait an error says, so every client reads the
// same wait in the same answer: in the Retry-After header of a refusal,
// in what a batch's answer says of one email, and in a value read at a
// time that is said.
func TestAWaitIsReadAsEveryClientReadsIt(t *testing.T) {
	var file struct {
		Header []waitCase `json:"header"`
		Batch  []waitCase `json:"batch"`
		Read   []readCase `json:"read"`
	}
	cases(t, "waits.json", &file)
	if len(file.Header) == 0 || len(file.Batch) == 0 || len(file.Read) == 0 {
		t.Fatal("waits.json holds no header to read, no batch, or no value read at a said time")
	}
	for _, k := range file.Read {
		if k.Now == nil {
			t.Fatalf("%q is read at no time", k.Given)
		}
		if got := waitIn(k.Given, time.UnixMilli(*k.Now)); got != k.wait() {
			t.Errorf("%q read at %d: a wait of %v, where the cases say %v", k.Given, *k.Now, got, k.wait())
		}
	}
	email := Email{From: "app@example.com", To: []string{"ana@example.org"}, Subject: "Welcome", Text: "Hello."}

	_, c := serving(t, func(w http.ResponseWriter, n int) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Retry-After", file.Header[n-1].Given)
		w.WriteHeader(503)
		io.WriteString(w, `{"statusCode":503,"name":"unavailable","message":"Down."}`)
	})
	for _, k := range file.Header {
		_, err := c.Send(ctx, email)
		var problem *Error
		if !errors.As(err, &problem) || problem.Name != ErrUnavailable {
			t.Fatalf("Retry-After %q: the refusal was not read: %v", k.Given, err)
		}
		if problem.RetryAfter != k.wait() {
			t.Errorf("Retry-After %q: a wait of %v, where the cases say %v", k.Given, problem.RetryAfter, k.wait())
		}
	}

	_, c = serving(t, func(w http.ResponseWriter, n int) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(207)
		// As it is written in the cases, not read and written again.
		io.WriteString(w, `{"data":[{"error":{"statusCode":429,"name":"rate_limit_exceeded","message":"Later.","retry_after":`+file.Batch[n-1].Given+`}}]}`)
	})
	for _, k := range file.Batch {
		got, err := c.SendBatch(ctx, Batch{Emails: []Email{email}})
		if err != nil || len(got) != 1 {
			t.Errorf("retry_after %s: the batch's answer was not read: %v", k.Given, err)
			continue
		}
		var problem *Error
		if !errors.As(got[0].Err, &problem) || problem.Name != ErrRateLimited || MayHaveSent(got[0].Err) {
			t.Errorf("retry_after %s: the email's error was not read: %v", k.Given, got[0].Err)
			continue
		}
		if problem.RetryAfter != k.wait() {
			t.Errorf("retry_after %s: a wait of %v, where the cases say %v", k.Given, problem.RetryAfter, k.wait())
		}
	}
}
