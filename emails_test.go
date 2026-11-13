package cairnpost

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestSendSaysOnlyWhatTheServerNames(t *testing.T) {
	s, c := serve(t, 200, `{"id":"msg-1"}`)
	id, err := c.Send(ctx, Email{From: "App <app@example.com>", To: "ana@example.org", Subject: "Hi <there>",
		Text: "Body", IdempotencyKey: "welcome-42"})
	if err != nil || id != "msg-1" {
		t.Fatalf("%q %v", id, err)
	}
	got := s.seen[0]
	if got.method != "POST" || got.path != "/emails" || got.auth != "Bearer cp_test_KEYMARK" || got.idem != "welcome-42" ||
		got.contentType != "application/json" || !strings.HasPrefix(got.agent, "cairnpost-go/") {
		t.Fatalf("%+v", got)
	}
	// The server refuses a field it does not name, so none is sent empty
	// that was not set, and none of the client's own.
	var fields map[string]any
	if err := json.Unmarshal([]byte(got.body), &fields); err != nil || len(fields) != 4 ||
		fields["from"] != "App <app@example.com>" || fields["subject"] != "Hi <there>" {
		t.Fatalf("%s %v", got.body, err)
	}
	// Without a key, no header.
	if _, err := c.Send(ctx, Email{From: "a@example.com", To: "b@example.org", Subject: "s", Text: "t"}); err != nil || s.seen[1].idem != "" {
		t.Fatalf("%v %+v", err, s.seen[1])
	}
}

func TestGetReadsARecordAndEscapesTheID(t *testing.T) {
	s, c := serve(t, 200, `{"id":"msg-1","from":"app@example.com","to":"ana@example.org","state":"bounced",
		"created_at":"2030-01-01T00:00:00Z","updated_at":"2030-01-01T00:00:05Z",
		"events":[{"type":"bounced","at":"2030-01-01T00:00:05Z","permanent":true,"code":"5.1.1","reason":"permanent_failure","detail":"no such user"}],
		"content":{"subject":"Hello","text":"Body","kept_until":"2030-01-02T00:00:00Z"}}`)
	m, err := c.Get(ctx, "a/b ?c")
	if err != nil || m.State != StateBounced || len(m.Events) != 1 || !m.Events[0].Permanent || m.Events[0].Detail != "no such user" ||
		m.Content == nil || m.Content.Subject != "Hello" || m.Content.KeptUntil.Year() != 2030 {
		t.Fatalf("%+v %v", m, err)
	}
	if s.seen[0].rawPath != "/emails/a%2Fb%20%3Fc" || s.seen[0].query != "" {
		t.Fatalf("the id went out as %q ? %q", s.seen[0].rawPath, s.seen[0].query)
	}
	if _, err := c.Get(ctx, ""); err == nil || len(s.seen) != 1 {
		t.Fatalf("an empty id was asked of the server: %v", err)
	}
	_, c = serve(t, 404, `{"name":"not_found","message":"No email has this id."}`)
	if _, err := c.Get(ctx, "nope"); !IsNotFound(err) {
		t.Fatalf("%v", err)
	}
}

// A 2xx that does not say what CairnPost says is not a send: whatever is
// at that address, the email is not known to have gone.
func TestAnAnswerThatIsNotCairnPostsIsNotASend(t *testing.T) {
	for _, body := range []string{`{}`, `null`, `{"ok":true}`, ``} {
		for _, status := range []int{200, 204} {
			_, c := serve(t, status, body)
			if id, err := c.Send(ctx, Email{From: "a@example.com", To: "b@example.org", Subject: "s", Text: "t"}); err == nil || id != "" {
				t.Errorf("%d %q was taken as a send: %q", status, body, id)
			}
			if m, err := c.Get(ctx, "m"); err == nil {
				t.Errorf("%d %q was taken as an email: %+v", status, body, m)
			}
			if w, err := c.CreateWebhook(ctx, NewWebhook{URL: "https://example.com/hook"}); err == nil {
				t.Errorf("%d %q was taken as an endpoint: %+v", status, body, w)
			}
		}
	}
}

// A key that a header would drop or change is refused before anything is
// sent, and is not mistaken for a send that may have gone.
func TestAnIdempotencyKeyAHeaderCannotCarryIsRefused(t *testing.T) {
	s, c := serve(t, 200, `{"id":"m"}`)
	for _, key := range []string{"  ", " padded", "two\nlines", "nul\x00"} {
		_, err := c.Send(ctx, Email{From: "a@example.com", To: "b@example.org", Subject: "s", Text: "t", IdempotencyKey: key})
		if err == nil || MayHaveSent(err) || len(s.seen) != 0 {
			t.Errorf("%q: %v, %d requests", key, err, len(s.seen))
		}
	}
}
