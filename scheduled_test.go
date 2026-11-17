package cairnpost

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

var nine = time.Date(2030, 1, 31, 9, 0, 0, 0, time.FixedZone("CET", 3600))

func TestAnEmailAskedForLaterWaits(t *testing.T) {
	s, c := serve(t, 200, `{"id":"msg-1","scheduled_at":"2030-01-31T08:00:00Z"}`)
	sent, err := c.Send(ctx, Email{From: "app@example.com", To: []string{"ana@example.org"}, Subject: "s", Text: "t", ScheduledAt: nine})
	if err != nil || sent.ID != "msg-1" || !sent.ScheduledAt.Equal(nine) {
		t.Fatalf("%+v %v", sent, err)
	}
	var fields map[string]any
	if err := json.Unmarshal([]byte(s.seen[0].body), &fields); err != nil || fields["scheduled_at"] != "2030-01-31T08:00:00Z" {
		t.Fatalf("%s %v", s.seen[0].body, err)
	}
	// An answer that does not say the email waits is not trusted: it may
	// have been sent now.
	_, c = serve(t, 200, `{"id":"msg-1"}`)
	if _, err := c.Send(ctx, Email{From: "app@example.com", To: []string{"ana@example.org"}, Subject: "s", Text: "t", ScheduledAt: nine}); !MayHaveSent(err) {
		t.Fatalf("an answer with no time: %v", err)
	}
	// Without a time, none is sent.
	s, c = serve(t, 200, `{"id":"msg-2"}`)
	if _, err := c.Send(ctx, Email{From: "app@example.com", To: []string{"ana@example.org"}, Subject: "s", Text: "t"}); err != nil ||
		strings.Contains(s.seen[0].body, "scheduled_at") {
		t.Fatalf("%v %s", err, s.seen[0].body)
	}
}

func TestAWaitingEmailIsCancelledAndMoved(t *testing.T) {
	s, c := serving(t, func(w http.ResponseWriter, n int) {
		if n == 1 {
			io.WriteString(w, `{"id":"a/b","state":"cancelled"}`)
			return
		}
		io.WriteString(w, `{"id":"msg-1","scheduled_at":"2030-01-31T08:00:00Z"}`)
	})
	if err := c.Cancel(ctx, "a/b"); err != nil || s.seen[0].method != "POST" || s.seen[0].rawPath != "/emails/a%2Fb/cancel" {
		t.Fatalf("%v %+v", err, s.seen[0])
	}
	if err := c.Reschedule(ctx, "msg-1", nine); err != nil || s.seen[1].method != "PATCH" || s.seen[1].path != "/emails/msg-1" ||
		s.seen[1].body != `{"scheduled_at":"2030-01-31T08:00:00Z"}`+"\n" {
		t.Fatalf("%v %+v", err, s.seen[1])
	}
	if err := c.Reschedule(ctx, "msg-1", time.Time{}); err == nil || len(s.seen) != 2 {
		t.Fatalf("moved to no time: %v", err)
	}
	if err := c.Cancel(ctx, ""); err == nil || len(s.seen) != 2 {
		t.Fatalf("an empty id was asked of the server: %v", err)
	}
	_, c = serve(t, 409, `{"name":"not_scheduled","message":"It does not wait."}`)
	var problem *Error
	if err := c.Cancel(ctx, "msg-1"); !errors.As(err, &problem) || problem.Name != ErrNotScheduled || problem.Retryable() || MayHaveSent(err) {
		t.Fatalf("%v", err)
	}
	_, c = serve(t, 503, `{"name":"schedule_full","message":"Full."}`)
	if _, err := c.Send(ctx, Email{From: "a@example.com", To: []string{"b@example.org"}, Subject: "s", Text: "t", ScheduledAt: nine}); !errors.As(err, &problem) ||
		problem.Name != ErrScheduleFull || !problem.Retryable() || MayHaveSent(err) {
		t.Fatalf("%v", err)
	}
}

func TestAWaitingEmailIsReadAsOne(t *testing.T) {
	_, c := serve(t, 200, `{"id":"msg-1","from":"app@example.com","to":"ana@example.org","state":"cancelled","reason":"key_revoked",
		"scheduled_at":"2030-01-31T08:00:00Z","created_at":"2030-01-01T00:00:00Z","updated_at":"2030-01-01T00:00:05Z","events":[]}`)
	m, err := c.Get(ctx, "msg-1")
	if err != nil || m.State != StateCancelled || m.Reason != "key_revoked" || m.ScheduledAt == nil || !m.ScheduledAt.Equal(nine) {
		t.Fatalf("%+v %v", m, err)
	}
}
