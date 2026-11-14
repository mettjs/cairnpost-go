//go:build integration

package cairnpost

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

// These tests run against a real CairnPost server: one started with the
// capture driver and CAIRNPOST_WEBHOOK_ALLOW_PRIVATE=true, so that nothing
// is sent to anybody and a webhook can reach this machine. They need its
// address and two keys, and fail without them rather than skip.
func live(t *testing.T) (send, admin *Client) {
	t.Helper()
	base, sendKey, adminKey := os.Getenv("CAIRNPOST_URL"), os.Getenv("CAIRNPOST_SEND_KEY"), os.Getenv("CAIRNPOST_ADMIN_KEY")
	if base == "" || sendKey == "" || adminKey == "" {
		t.Fatal("set CAIRNPOST_URL, CAIRNPOST_SEND_KEY and CAIRNPOST_ADMIN_KEY to a server running the capture driver")
	}
	send, err := New(base, sendKey)
	if err != nil {
		t.Fatal(err)
	}
	admin, err = New(base, adminKey)
	if err != nil {
		t.Fatal(err)
	}
	return send, admin
}

// named is the error's name, "" for anything that is not CairnPost's.
func named(err error) string {
	var problem *Error
	if errors.As(err, &problem) {
		return problem.Name
	}
	return ""
}

func TestSendingAgainstARealServer(t *testing.T) {
	send, admin := live(t)
	h, err := send.Health(ctx)
	if err != nil || !h.OK() || h.Driver != "capture" {
		t.Fatalf("the server must be up and on the capture driver: %+v %v", h, err)
	}
	if l, err := send.Limits(ctx); err != nil || l.Driver != "capture" || l.Quota != nil {
		t.Fatalf("%+v %v", l, err)
	}
	run := fmt.Sprintf("run-%d", time.Now().UnixNano())
	email := Email{From: "App <app@" + run + ".example.com>", To: []string{"ana@example.org"}, Subject: "Hello " + run,
		Text: "Body", HTML: "<p>Body & more</p>", ReplyTo: "help@example.com", Headers: map[string]string{"X-Run": run},
		IdempotencyKey: run}
	first, err := send.Send(ctx, email)
	if err != nil {
		t.Fatal(err)
	}
	id := first.ID
	// The same request again is the same email; another under its key is refused.
	if again, err := send.Send(ctx, email); err != nil || again.ID != id {
		t.Fatalf("a repeat answered %+v %v, want %q", again, err, id)
	}
	other := email
	other.Subject = "Another"
	if _, err := send.Send(ctx, other); named(err) != ErrIdempotentMismatch || MayHaveSent(err) {
		t.Fatalf("%v", err)
	}
	m, err := send.Get(ctx, id)
	if err != nil || m.ID != id || m.State != StateSent || m.To != "ana@example.org" || m.CreatedAt.IsZero() {
		t.Fatalf("%+v %v", m, err)
	}
	// A batch: each email has its own id, and the batch again is the same.
	batch := Batch{IdempotencyKey: run + "-batch", Emails: []Email{
		{From: email.From, To: []string{"ben@example.org"}, Subject: "One " + run, Text: "Body"},
		{From: email.From, To: []string{"cy@example.org"}, Subject: "Two " + run, Text: "Body"}}}
	sent, err := send.SendBatch(ctx, batch)
	if err != nil || len(sent) != 2 || Failed(sent) != 0 || sent[0].Sent.ID == sent[1].Sent.ID {
		t.Fatalf("the batch: %+v %v", sent, err)
	}
	if again, err := send.SendBatch(ctx, batch); err != nil || Failed(again) != 0 || again[1].Sent.ID != sent[1].Sent.ID {
		t.Fatalf("the batch again: %+v %v", again, err)
	}
	batch.Emails[1].To = nil
	if _, err := send.SendBatch(ctx, batch); named(err) != ErrValidation || MayHaveSent(err) {
		t.Fatalf("a batch with an email that cannot go: %v", err)
	}
	// An email asked for later waits, is moved and is cancelled.
	later := Email{From: email.From, To: []string{"dee@example.org"}, Subject: "Later " + run, Text: "Body", ScheduledAt: time.Now().Add(time.Hour)}
	waits, err := send.Send(ctx, later)
	if err != nil || waits.ScheduledAt.IsZero() {
		t.Fatalf("asked for later: %+v %v", waits, err)
	}
	if m, err := send.Get(ctx, waits.ID); err != nil || m.State != StateScheduled || m.ScheduledAt == nil {
		t.Fatalf("while it waits: %+v %v", m, err)
	}
	if err := send.Reschedule(ctx, waits.ID, time.Now().Add(2*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := send.Cancel(ctx, waits.ID); err != nil {
		t.Fatal(err)
	}
	if err := send.Cancel(ctx, waits.ID); named(err) != ErrNotScheduled {
		t.Fatalf("cancelled twice: %v", err)
	}
	if m, err := send.Get(ctx, waits.ID); err != nil || m.State != StateCancelled {
		t.Fatalf("once cancelled: %+v %v", m, err)
	}
	if _, err := send.Send(ctx, Email{From: email.From, To: []string{"dee@example.org"}, Subject: "s", Text: "t", ScheduledAt: time.Now().Add(-time.Hour)}); named(err) != ErrValidation {
		t.Fatalf("asked for in the past: %v", err)
	}
	held, err := admin.Captured(ctx)
	found := false
	for _, c := range held {
		found = found || (c.Subject == email.Subject && c.FromName == "App" && c.HTML == email.HTML && c.Headers["X-Run"] == run)
	}
	if err != nil || !found {
		t.Fatalf("the email is not among the %d captured: %v", len(held), err)
	}
	// Whatever the server refuses, the error repeats nothing the email
	// said: not its subject, its bodies, a header's value or the sender's
	// name.
	worded := Email{From: "NAMEMARKQ <app@" + run + ".example.com>", To: []string{"ana@example.org"}, Subject: "SUBJECTMARKQ",
		Text: "TEXTMARKQ", HTML: "<p>HTMLMARKQ</p>", Headers: map[string]string{"X-Trace": "HEADERMARKQ"}, IdempotencyKey: run + "-worded"}
	if _, err := send.Send(ctx, worded); err != nil {
		t.Fatal(err)
	}
	refused := map[string]func(*Email){
		ErrValidation + " (the recipient)": func(e *Email) { e.To = []string{"not an address"} },
		ErrValidation + " (a header)":      func(e *Email) { e.Headers = map[string]string{"Bad Header": "HEADERMARKQ"} },
		ErrValidation + " (no body)":       func(e *Email) { e.Text, e.HTML = "", "" },
		ErrIdempotentMismatch:              func(e *Email) { e.Subject = "SUBJECTMARKQ again" },
	}
	for want, change := range refused {
		e := worded
		change(&e)
		_, err := send.Send(ctx, e)
		if name, _, _ := strings.Cut(want, " "); named(err) != name {
			t.Fatalf("%s: %v", want, err)
		}
		for _, mark := range []string{"NAMEMARKQ", "SUBJECTMARKQ", "TEXTMARKQ", "HTMLMARKQ", "HEADERMARKQ"} {
			if strings.Contains(err.Error(), mark) {
				t.Errorf("%s: the server's error holds %q: %v", want, mark, err)
			}
		}
	}
	// What the server refuses comes back named.
	bad := email
	bad.To, bad.IdempotencyKey = []string{"not an address"}, ""
	if _, err := send.Send(ctx, bad); named(err) != ErrValidation {
		t.Fatalf("%v", err)
	}
	if _, err := send.Get(ctx, "no-such-id"); !IsNotFound(err) {
		t.Fatalf("%v", err)
	}
	if _, err := send.Captured(ctx); named(err) != ErrAdminKeyRequired {
		t.Fatalf("a send key read the captured emails: %v", err)
	}
	wrong, _ := New(os.Getenv("CAIRNPOST_URL"), "cp_00000000_not-a-key")
	if _, err := wrong.Get(ctx, id); named(err) != ErrInvalidKey {
		t.Fatalf("%v", err)
	}

	// A suppressed address is refused, listed, and sent to again once lifted.
	domain, gone := run+".example.com", "gone-"+run+"@example.org"
	if added, err := admin.Suppress(ctx, domain, gone, "by the test"); err != nil || !added {
		t.Fatalf("%v %v", added, err)
	}
	if added, err := admin.Suppress(ctx, domain, gone, ""); err != nil || added {
		t.Fatalf("a second time: %v %v", added, err)
	}
	blocked := Email{From: "app@" + domain, To: []string{gone}, Subject: "s", Text: "t"}
	if _, err := send.Send(ctx, blocked); named(err) != ErrRecipientSuppress {
		t.Fatalf("%v", err)
	}
	var listed []Suppression
	for s, err := range admin.AllSuppressions(ctx, domain) {
		if err != nil {
			t.Fatal(err)
		}
		listed = append(listed, s)
	}
	if len(listed) != 1 || listed[0].Address != gone || listed[0].Reason != ReasonManual || listed[0].Note != "by the test" {
		t.Fatalf("%+v", listed)
	}
	// A file goes with an email, and a development server says what it
	// was without holding what was in it.
	if _, err := send.Send(ctx, Email{From: "app@" + domain, To: []string{"files@example.org"}, Subject: "With a file " + run, Text: "t",
		Attachments: []Attachment{{Filename: "note-" + run + ".txt", Content: []byte("hello")}}}); err != nil {
		t.Fatal(err)
	}
	withFile, err := admin.Captured(ctx)
	if held := withFile; err != nil || len(held) == 0 || len(held[0].Attachments) != 1 || held[0].Attachments[0].Filename != "note-"+run+".txt" ||
		held[0].Attachments[0].Size != 5 || !strings.HasPrefix(held[0].Attachments[0].ContentType, "text/plain") {
		t.Fatalf("the captured file: %+v %v", held, err)
	}
	// Among several, a suppressed address is left out and named; the email
	// is one, with an outcome for each address it went to.
	several, err := send.Send(ctx, Email{From: "app@" + domain, To: []string{"ana@example.org", gone},
		Cc: []string{"bo@example.org"}, Bcc: []string{"cy@example.org"}, Subject: "s", Text: "t"})
	if err != nil || len(several.Suppressed) != 1 || several.Suppressed[0] != gone {
		t.Fatalf("%+v %v", several, err)
	}
	if m, err := send.Get(ctx, several.ID); err != nil || len(m.Recipients) != 3 || m.To != "ana@example.org" ||
		len(m.Suppressed) != 1 || m.Suppressed[0] != gone ||
		m.Recipients[1].Kind != KindCc || m.Recipients[2].Kind != KindBcc {
		t.Fatalf("%+v %v", m, err)
	}
	if err := admin.Lift(ctx, domain, gone); err != nil {
		t.Fatal(err)
	}
	if err := admin.Lift(ctx, domain, gone); !IsNotFound(err) {
		t.Fatalf("lifting what is not suppressed: %v", err)
	}
	if _, err := send.Send(ctx, blocked); err != nil {
		t.Fatalf("after the lift: %v", err)
	}
	for _, address := range []string{"a-" + gone, "b-" + gone} {
		if _, err := admin.Suppress(ctx, domain, address, ""); err != nil {
			t.Fatal(err)
		}
	}
	if page, err := admin.Suppressions(ctx, SuppressionQuery{Domain: domain, Limit: 1}); err != nil || len(page.Data) != 1 || page.Next == "" {
		t.Fatalf("%+v %v", page, err)
	}
	if n, err := admin.LiftAll(ctx, domain, ReasonManual); err != nil || n != 2 {
		t.Fatalf("%d %v", n, err)
	}
}
