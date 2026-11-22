package cairnpost

import (
	"context"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

// An error never repeats what the email said or the key that sent it,
// whatever went wrong and whatever the server answered.
func TestNoErrorHoldsTheEmailsWordsOrTheKey(t *testing.T) {
	email := Email{From: "NAMEMARK <app@example.com>", To: []string{"ana@example.org"}, Subject: "SUBJECTMARK",
		Text: "TEXTMARK", HTML: "<p>HTMLMARK</p>", Headers: map[string]string{"X-Trace": "HEADERMARK"},
		Attachments: []Attachment{{Filename: "FILEMARK.txt", Content: []byte("BYTESMARK")}}}
	// The last is the file's bytes as they travel.
	marks := []string{"NAMEMARK", "SUBJECTMARK", "TEXTMARK", "HTMLMARK", "HEADERMARK", "KEYMARK", "FILEMARK", "BYTESMARK", "QllURVNNQVJL"}
	check := func(why string, err error) {
		t.Helper()
		if err == nil {
			t.Fatalf("%s: no error", why)
		}
		for _, mark := range marks {
			if strings.Contains(err.Error(), mark) {
				t.Errorf("%s: the error holds %q: %v", why, mark, err)
			}
		}
	}
	// A server that echoes the request in a body that is not CairnPost's.
	echo := func(status int, field string) *Client {
		s := &stand{t: t}
		s.answer = func(w http.ResponseWriter, n int) {
			w.WriteHeader(status)
			io.WriteString(w, `{"name":"BadRequestError","`+field+`":`+strconv.Quote(s.seen[n-1].body+s.seen[n-1].auth)+`}`)
		}
		srv := httptest.NewServer(s)
		t.Cleanup(srv.Close)
		c, _ := New(srv.URL, "cp_test_KEYMARK")
		return c
	}
	_, err := echo(500, "error").Send(ctx, email)
	check("an answer that echoes the request", err)
	// The same, in the shape of CairnPost's own error under another name.
	_, err = echo(400, "message").Send(ctx, email)
	check("an error of CairnPost's shape that is not CairnPost's", err)
	var problem *Error
	if !errors.As(err, &problem) || problem.Name != "" || problem.Message != "" {
		t.Errorf("a name that is not CairnPost's was believed: %+v", problem)
	}
	_, c := serve(t, 200, `SUBJECTMARK is not JSON, KEYMARK`)
	_, err = c.Send(ctx, email)
	check("an answer that is not JSON", err)
	// A server that is not there.
	gone := httptest.NewServer(http.NotFoundHandler())
	gone.Close()
	c, _ = New(gone.URL, "cp_test_KEYMARK")
	_, err = c.Send(ctx, email)
	check("no server", err)
	if !MayHaveSent(err) {
		t.Error("a send with no answer was said not to have gone")
	}
	// A call that runs out of time.
	_, c = serving(t, func(http.ResponseWriter, int) { time.Sleep(200 * time.Millisecond) })
	short, cancel := context.WithTimeout(ctx, 20*time.Millisecond)
	defer cancel()
	_, err = c.Send(short, email)
	check("a timeout", err)
	if !errors.Is(err, context.DeadlineExceeded) || !MayHaveSent(err) {
		t.Errorf("a timeout: %v", err)
	}
}

func TestRetryAfterAsADateIsRead(t *testing.T) {
	_, c := serving(t, func(w http.ResponseWriter, _ int) {
		w.Header().Set("Retry-After", time.Now().Add(90*time.Second).UTC().Format(http.TimeFormat))
		w.WriteHeader(503)
	})
	_, err := c.Send(ctx, Email{From: "a@example.com", To: []string{"b@example.org"}, Subject: "s", Text: "t"})
	var problem *Error
	if !errors.As(err, &problem) || problem.RetryAfter < 80*time.Second || problem.RetryAfter > 91*time.Second {
		t.Fatalf("%+v", problem)
	}
}

// What an error says to do is in the shared cases (cases_errors_test.go).

// The server's own sentence is kept, so the error says what to check; and
// no answer at all is not a wrong address.
func TestAWrongAddressSaysWhatToCheck(t *testing.T) {
	_, client := serve(t, 404, `{"statusCode":404,"name":"route_not_found","message":"There is nothing at this path. Check the path and the server's address."}`)
	if _, err := client.Get(ctx, "0199"); err == nil || !strings.Contains(err.Error(), "Check the path and the server's address") {
		t.Fatalf("%v", err)
	}
	if IsWrongAddress(nil) || IsWrongAddress(&TransportError{}) {
		t.Fatal("no answer at all is not a wrong address")
	}
}

// Every name the client has a constant for is one it believes: a constant
// left out of known would come back with no name and no message.
func TestEveryErrorNameIsKnown(t *testing.T) {
	file, err := parser.ParseFile(token.NewFileSet(), "errors.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	names := 0
	ast.Inspect(file, func(n ast.Node) bool {
		spec, ok := n.(*ast.ValueSpec)
		if !ok || len(spec.Names) != 1 || len(spec.Values) != 1 || !strings.HasPrefix(spec.Names[0].Name, "Err") {
			return true
		}
		lit, ok := spec.Values[0].(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			return true
		}
		name, _ := strconv.Unquote(lit.Value)
		if names++; !known[name] {
			t.Errorf("%s (%q) is not in known", spec.Names[0].Name, name)
		}
		return true
	})
	if names != len(known) || names < 20 {
		t.Errorf("%d constants, %d known names", names, len(known))
	}
}
