package cairnpost

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

// An idempotency key goes out as it was given, or the send is not made:
// a key a header cannot carry as it is would be changed on the way, and
// the send would quietly not be idempotent. So for a send and for a
// batch alike, and the refusal does not repeat the key.
func TestAnIdempotencyKeyIsSentAsItIsOrNotAtAll(t *testing.T) {
	var file struct {
		Taken   []string `json:"taken"`
		Refused []string `json:"refused"`
	}
	cases(t, "idempotency.json", &file)
	if len(file.Taken) == 0 || len(file.Refused) == 0 {
		t.Fatal("idempotency.json holds no key to send, or none to refuse")
	}
	email := Email{From: "app@example.com", To: []string{"ana@example.org"}, Subject: "Welcome", Text: "Hello."}
	// The two calls that take a key, under the names the cases know them by.
	sends := map[string]func(*Client, string, int) error{
		"emails.send": func(c *Client, key string, _ int) error {
			e := email
			e.IdempotencyKey = key
			_, err := c.Send(ctx, e)
			return err
		},
		"emails.send_batch": func(c *Client, key string, emails int) error {
			b := Batch{IdempotencyKey: key}
			for range emails {
				b.Emails = append(b.Emails, email)
			}
			_, err := c.SendBatch(ctx, b)
			return err
		},
	}
	for _, call := range sharedCalls(t) {
		send, ok := sends[call.Name]
		if !ok {
			continue
		}
		delete(sends, call.Name)
		// As many emails as the batch's answer in the cases tells of.
		var answered struct {
			Data []json.RawMessage `json:"data"`
		}
		_ = json.Unmarshal(call.Answer.JSON, &answered)
		for _, key := range file.Taken {
			s, c := saying(t, call.Answer)
			if err := send(c, key, len(answered.Data)); err != nil {
				t.Errorf("%s refused the key %q: %v", call.Name, key, err)
				continue
			}
			if len(s.seen) != 1 || s.seen[0].idem != key {
				t.Errorf("%s with the key %q made %d requests, the last under %+v", call.Name, key, len(s.seen), s.seen)
			}
		}
		s, c := saying(t, call.Answer)
		for _, key := range file.Refused {
			err := send(c, key, len(answered.Data))
			if err == nil || len(s.seen) != 0 {
				t.Errorf("%s with the key %q: %d requests made, and the error %v", call.Name, key, len(s.seen), err)
				s.seen = nil
				continue
			}
			// Refused by this client as a wrong argument is, not by
			// net/http on the way out: that reads as an email that may
			// have gone, when nothing was sent.
			var unreached *TransportError
			if errors.As(err, &unreached) || MayHaveSent(err) {
				t.Errorf("%s with the key %q was stopped on the way out, not refused: %v", call.Name, key, err)
			}
			for i := 0; i+6 <= len(key); i++ {
				if strings.Contains(err.Error(), key[i:i+6]) {
					t.Errorf("the refusal of the key %q repeats part of it: %v", key, err)
					break
				}
			}
		}
	}
	for name := range sends {
		t.Errorf("the cases describe no call named %s", name)
	}
}
