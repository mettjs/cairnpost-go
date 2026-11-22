package cairnpost

import (
	"strings"
	"testing"
)

// A key is taken when it could be one: what a server gives, with at most
// a space, a tab or a line break around it, and it is sent without them.
// Any other is refused when the client is made, before a call, by an
// error that does not repeat it.
func TestAKeyIsTakenOnlyWhenItCouldBeOne(t *testing.T) {
	var file struct {
		Taken []struct {
			Given string `json:"given"`
			Sends string `json:"sends"`
		} `json:"taken"`
		Refused []string `json:"refused"`
	}
	cases(t, "keys.json", &file)
	if len(file.Taken) == 0 || len(file.Refused) == 0 {
		t.Fatal("keys.json holds no key to take, or none to refuse")
	}
	// Any call that needs a key shows what is sent for one.
	var call sharedCall
	for _, c := range sharedCalls(t) {
		if c.Key != "none" {
			call = c
			break
		}
	}
	for _, k := range file.Taken {
		s, standing := saying(t, call.Answer)
		c, err := New(standing.base.String(), k.Given)
		if err != nil {
			t.Errorf("%q was refused as a key: %v", k.Given, err)
			continue
		}
		if err := ask(t, call.Name)(ctx, c); err != nil || len(s.seen) != 1 {
			t.Fatalf("%s with the key %q: %d requests, %v", call.Name, k.Given, len(s.seen), err)
		}
		if got := s.seen[0].auth; got != "Bearer "+k.Sends {
			t.Errorf("%q was sent as %q, not as %q", k.Given, got, "Bearer "+k.Sends)
		}
	}
	for _, key := range file.Refused {
		c, err := New("https://mail.example.com", key)
		if err == nil {
			t.Errorf("%q was taken as a key, and would be sent as %q", key, c.key)
			continue
		}
		for i := 0; i+8 <= len(key); i++ {
			if strings.Contains(err.Error(), key[i:i+8]) {
				t.Errorf("the refusal of %q repeats part of it: %v", key, err)
				break
			}
		}
	}
}
