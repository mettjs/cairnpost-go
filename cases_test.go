package cairnpost

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

// The files in testdata/cases are the cases every CairnPost client is
// held to, whatever language it is written in: what each call sends, the
// answers a client must take and those it must refuse, what an error
// says to do, which API keys and idempotency keys it takes, and which
// webhooks are the endpoint's. They are a copy of
// the server's own (client-cases in its repository) and are changed
// there, never here. The tests in cases_*_test.go run them.

// said is an answer for a server to give: its body is either JSON, written
// in the file as it is, or a text.
type said struct {
	Status  int               `json:"status"`
	JSON    json.RawMessage   `json:"json"`
	Text    *string           `json:"text"`
	Headers map[string]string `json:"headers"`
}

// cases reads one of the shared files. A field this runner does not know
// fails the read: a case must never pass because part of it was not
// looked at.
func cases(t *testing.T, name string, into any) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", "cases", name))
	if err != nil {
		t.Fatal(err)
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(into); err != nil {
		t.Fatalf("%s: %v", name, err)
	}
}

// saying starts a server that gives one answer to every request, and a
// client of it.
func saying(t *testing.T, a said) (*stand, *Client) {
	t.Helper()
	if (a.JSON == nil) == (a.Text == nil) {
		t.Fatalf("an answer has a json or a text, and not both: %+v", a)
	}
	body := string(a.JSON)
	if a.Text != nil {
		body = *a.Text
	}
	return serving(t, func(w http.ResponseWriter, _ int) {
		if a.JSON != nil {
			w.Header().Set("Content-Type", "application/json")
		}
		for name, value := range a.Headers {
			w.Header().Set(name, value)
		}
		w.WriteHeader(a.Status)
		io.WriteString(w, body)
	})
}

// sameJSON says two JSON texts say the same. A time is the instant it
// names, however it is written.
func sameJSON(t *testing.T, got, want []byte) bool {
	t.Helper()
	var a, b any
	if err := json.Unmarshal(got, &a); err != nil {
		t.Fatalf("%s: %v", got, err)
	}
	if err := json.Unmarshal(want, &b); err != nil {
		t.Fatalf("%s: %v", want, err)
	}
	return reflect.DeepEqual(instants(a), instants(b))
}

// instants puts every time in a JSON value into one form.
func instants(v any) any {
	switch v := v.(type) {
	case string:
		if at, err := time.Parse(time.RFC3339Nano, v); err == nil {
			return at.UTC()
		}
	case []any:
		for i := range v {
			v[i] = instants(v[i])
		}
	case map[string]any:
		for k := range v {
			v[k] = instants(v[k])
		}
	}
	return v
}
