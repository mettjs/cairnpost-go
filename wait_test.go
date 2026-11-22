package cairnpost

import (
	"errors"
	"net/http"
	"testing"
	"time"
)

// A header said more than once is read as one value with a comma
// between, as the other clients are handed it: so two waits are none,
// and a date cut in two at its comma is that date.
func TestARetryAfterSaidTwiceIsReadAsOneValue(t *testing.T) {
	date := time.Now().Add(90 * time.Second).UTC().Format(http.TimeFormat)
	said := [][]string{{"7", "9"}, {"7", "7"}, {"soon", "9"}, {date[:3], date[5:]}}
	_, c := serving(t, func(w http.ResponseWriter, n int) {
		for _, value := range said[n-1] {
			w.Header().Add("Retry-After", value)
		}
		w.WriteHeader(503)
	})
	for i, values := range said {
		_, err := c.Get(ctx, "x")
		var problem *Error
		if !errors.As(err, &problem) || problem.Status != 503 {
			t.Fatalf("Retry-After said as %q: %v", values, err)
		}
		if whole := i == len(said)-1; whole != (problem.RetryAfter > 80*time.Second && problem.RetryAfter <= 91*time.Second) || !whole && problem.RetryAfter != 0 {
			t.Errorf("Retry-After said as %q: a wait of %v", values, problem.RetryAfter)
		}
	}
}
