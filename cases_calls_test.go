package cairnpost

import (
	"encoding/json"
	"net/url"
	"reflect"
	"testing"
)

// sharedCall is a call as calls.json describes it.
type sharedCall struct {
	Name  string `json:"name"`
	Route string `json:"route"`
	Key   string `json:"key"`
	Sends struct {
		Method         string            `json:"method"`
		Path           string            `json:"path"`
		Query          map[string]string `json:"query"`
		IdempotencyKey string            `json:"idempotency_key"`
		Body           json.RawMessage   `json:"body"`
	} `json:"sends"`
	Answer said `json:"answer"`
}

func sharedCalls(t *testing.T) []sharedCall {
	t.Helper()
	var file struct {
		Calls []sharedCall `json:"calls"`
	}
	cases(t, "calls.json", &file)
	if len(file.Calls) == 0 {
		t.Fatal("calls.json holds no call")
	}
	return file.Calls
}

// Each call sends what every client sends for it, and takes the answer
// the server gives.
func TestEachCallSendsWhatItShouldAndTakesTheServersAnswer(t *testing.T) {
	described := map[string]bool{}
	for _, c := range sharedCalls(t) {
		described[c.Name] = true
		s, client := saying(t, c.Answer)
		if err := ask(t, c.Name)(ctx, client); err != nil {
			t.Errorf("%s refused the server's own answer: %v", c.Name, err)
		}
		if len(s.seen) != 1 {
			t.Fatalf("%s made %d requests", c.Name, len(s.seen))
		}
		got := s.seen[0]
		auth := "Bearer cp_test_KEYMARK"
		if c.Key == "none" {
			auth = ""
		}
		query, err := url.ParseQuery(got.query)
		if err != nil {
			t.Fatalf("%s: %v", c.Name, err)
		}
		asked := map[string]string{}
		for name, values := range query {
			asked[name] = values[0]
			if len(values) != 1 {
				t.Errorf("%s says %s %d times", c.Name, name, len(values))
			}
		}
		if c.Sends.Query == nil {
			c.Sends.Query = map[string]string{}
		}
		if got.method != c.Sends.Method || got.rawPath != c.Sends.Path || !reflect.DeepEqual(asked, c.Sends.Query) ||
			got.auth != auth || got.idem != c.Sends.IdempotencyKey {
			t.Errorf("%s sent %s %s ?%s (key %q, idempotency key %q), not %+v", c.Name, got.method, got.rawPath, got.query, got.auth, got.idem, c.Sends)
		}
		switch {
		case c.Sends.Body == nil && (got.body != "" || got.contentType != ""):
			t.Errorf("%s sent a body where none is sent: %s (%s)", c.Name, got.body, got.contentType)
		case c.Sends.Body != nil && (got.contentType != "application/json" || !sameJSON(t, []byte(got.body), c.Sends.Body)):
			t.Errorf("%s sent %s (%s), not %s", c.Name, got.body, got.contentType, c.Sends.Body)
		}
	}
	for _, a := range asking {
		if !described[a.name] {
			t.Errorf("%s is a call of this client that calls.json does not describe", a.name)
		}
	}
}
