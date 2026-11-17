package cairnpost

import (
	"net/http"
	"testing"
)

func TestAReceivedEmailIsReadWithWhatItSays(t *testing.T) {
	one := `{"id":"in-1","to":"help@mail.example.com","from":"ana@example.org","received_at":"2030-01-01T00:00:00Z",
		"kept_until":"2030-01-08T00:00:00Z","size":120,"content":{"subject":"A question","from":"Ana <ana@example.org>",
		"to":["help@mail.example.com"],"text":"hello","headers":[{"name":"Subject","value":"A question"}],
		"attachments":[{"filename":"a.pdf","content_type":"application/pdf","size":6}],"partial":true}}`
	s, c := serve(t, 200, one)
	got, err := c.Received(ctx, "a/b")
	if err != nil || got.ID != "in-1" || got.To != "help@mail.example.com" || got.KeptUntil.Day() != 8 || got.Content == nil ||
		got.Content.Subject != "A question" || got.Content.Text != "hello" || !got.Content.Partial || len(got.Content.Attachments) != 1 ||
		got.Content.Headers[0].Name != "Subject" || s.seen[0].rawPath != "/inbound/a%2Fb" {
		t.Fatalf("%+v %v", got, err)
	}
	if _, err := c.Received(ctx, ""); err == nil || len(s.seen) != 1 {
		t.Fatalf("an empty id was asked of the server: %v", err)
	}
	_, c = serve(t, 200, `{}`)
	if _, err := c.Received(ctx, "in-1"); err == nil {
		t.Fatal("an empty answer was taken for an email")
	}
	s, c = serve(t, 200, `{"data":[{"id":"in-2","to":"help@mail.example.com","received_at":"2030-01-01T00:00:00Z","kept_until":"2030-01-08T00:00:00Z"}],"next":"in-2"}`)
	page, err := c.ReceivedList(ctx, ReceivedQuery{Limit: 1, After: "in-3"})
	if err != nil || len(page.Data) != 1 || page.Data[0].Content != nil || page.Data[0].From != "" || page.Next != "in-2" ||
		s.seen[0].query != "after=in-3&limit=1" {
		t.Fatalf("%+v %v %+v", page, err, s.seen[0])
	}
	// Reading them all asks for the largest page a server gives.
	s, c = serve(t, 200, `{"data":[{"id":"in-9","to":"help@mail.example.com","received_at":"2030-01-01T00:00:00Z","kept_until":"2030-01-08T00:00:00Z"}]}`)
	for in, err := range c.AllReceived(ctx) {
		if err != nil || in.ID != "in-9" {
			t.Fatalf("%+v %v", in, err)
		}
	}
	if len(s.seen) != 1 || s.seen[0].query != "limit=200" {
		t.Fatalf("%+v", s.seen)
	}
	s, c = serve(t, 204, "")
	if err := c.DeleteReceived(ctx, "in-1"); err != nil || s.seen[0].method != "DELETE" || s.seen[0].path != "/inbound/in-1" {
		t.Fatalf("%v %+v", err, s.seen[0])
	}
}

func TestAReceivedEmailsBytesAreReadAsTheyCame(t *testing.T) {
	s, c := serving(t, func(w http.ResponseWriter, _ int) {
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Write([]byte("Subject: hi\r\n\r\n\x00\xff"))
	})
	raw, err := c.ReceivedRaw(ctx, "in-1")
	if err != nil || string(raw) != "Subject: hi\r\n\r\n\x00\xff" || s.seen[0].path != "/inbound/in-1/raw" {
		t.Fatalf("%q %v", raw, err)
	}
	if got, err := c.ReceivedFile(ctx, "in-1", 2); err != nil || len(got) == 0 || s.seen[1].path != "/inbound/in-1/attachments/2" {
		t.Fatalf("%q %v", got, err)
	}
	if _, err := c.ReceivedFile(ctx, "in-1", 0); err == nil || len(s.seen) != 2 {
		t.Fatalf("place 0: %v", err)
	}
	_, c = serve(t, 404, `{"statusCode":404,"name":"not_found","message":"There is no such email."}`)
	if _, err := c.ReceivedRaw(ctx, "in-1"); !IsNotFound(err) {
		t.Fatalf("%v", err)
	}
}

func TestAnEmailThatCameIsToldByItsOwnEvent(t *testing.T) {
	v := verifier(t)
	body := `{"type":"email.received","created_at":"2030-01-01T00:00:00Z","data":{"inbound_id":"in-1","from":"","to":["help@mail.example.com"],"received_at":"2030-01-01T00:00:00Z"}}`
	e, err := v.Verify(headers("webhook-", "dlv-1", at, signature(key, "dlv-1", at, body)), []byte(body))
	if err != nil || e.Type != EventReceived || e.Email != nil || e.Received == nil || e.Received.ID != "in-1" || e.Received.From != "" ||
		e.Received.To[0] != "help@mail.example.com" || e.Received.ReceivedAt.Year() != 2030 {
		t.Fatalf("%+v %v", e, err)
	}
	for _, without := range []string{
		`{"type":"email.received","created_at":"2030-01-01T00:00:00Z","data":{}}`,
		`{"type":"email.received","created_at":"2030-01-01T00:00:00Z","data":{"inbound_id":"in-1"}}`,
		`{"type":"email.received","created_at":"2030-01-01T00:00:00Z","data":{"to":["help@mail.example.com"]}}`,
	} {
		if _, err := v.Verify(headers("webhook-", "dlv-2", at, signature(key, "dlv-2", at, without)), []byte(without)); err != ErrPayload {
			t.Errorf("one that names no email or no address: %v", err)
		}
	}
}
