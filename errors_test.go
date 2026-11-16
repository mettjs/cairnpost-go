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

func TestAnErrorSaysWhatToDo(t *testing.T) {
	cases := []struct {
		status          int
		body, after     string
		name, id        string
		retry, mayBeOut bool
	}{
		{422, `{"statusCode":422,"name":"validation_error","message":"The subject is missing."}`, "", ErrValidation, "", false, false},
		{422, `{"statusCode":422,"name":"sender_domain_not_ready","message":"Not set up.","id":"msg-9"}`, "", ErrSenderDomainNotReady, "msg-9", false, false},
		{422, `{"name":"recipient_suppressed","message":"Suppressed.","id":"msg-9"}`, "", ErrRecipientSuppress, "msg-9", false, true},
		{409, `{"name":"invalid_idempotent_request","message":"Used."}`, "", ErrIdempotentMismatch, "", false, false},
		{409, `{"name":"concurrent_idempotent_requests","message":"In flight."}`, "7", ErrIdempotentInFlight, "", true, false},
		{408, `{"name":"request_incomplete","message":"Not whole."}`, "", ErrRequestIncomplete, "", true, false},
		// A 408 from something in between may come after the server took it.
		{408, `<html>request timeout</html>`, "", "", "", true, true},
		{429, `{"name":"rate_limit_exceeded","message":"Limit.","id":"msg-2"}`, "30", ErrRateLimited, "msg-2", true, false},
		{502, `{"name":"upstream_unknown","message":"Lost.","id":"msg-3"}`, "", ErrUpstreamUnknown, "msg-3", true, true},
		{503, `{"name":"upstream_unavailable","message":"Down."}`, "", ErrUpstreamDown, "", true, false},
		{401, `{"name":"invalid_api_key","message":"No."}`, "", ErrInvalidKey, "", false, false},
		{503, `{"name":"unavailable","message":"Down."}`, "", ErrUnavailable, "", true, false},
		{500, `{"name":"internal_error","message":"Failed."}`, "", ErrInternal, "", true, true},
		// Something in between answered: the server may have sent all the same.
		{502, `<html>bad gateway</html>`, "", "", "", true, true},
		{504, `{"name":"GatewayTimeout","message":"SUBJECTMARK"}`, "", "", "", true, true},
		// Without an id, a suppression means nothing went.
		{422, `{"name":"recipient_suppressed","message":"Suppressed."}`, "", ErrRecipientSuppress, "", false, false},
	}
	for _, want := range cases {
		_, c := serving(t, func(w http.ResponseWriter, _ int) {
			if want.after != "" {
				w.Header().Set("Retry-After", want.after)
			}
			w.WriteHeader(want.status)
			io.WriteString(w, want.body)
		})
		_, err := c.Send(ctx, Email{From: "a@example.com", To: []string{"b@example.org"}, Subject: "s", Text: "t"})
		var problem *Error
		if !errors.As(err, &problem) {
			t.Fatalf("%d: %v", want.status, err)
		}
		wait, _ := time.ParseDuration(want.after + "s")
		if problem.Status != want.status || problem.Name != want.name || problem.EmailID != want.id ||
			problem.RetryAfter != wait || problem.Retryable() != want.retry || MayHaveSent(err) != want.mayBeOut ||
			(problem.Message == "") != (want.name == "") {
			t.Errorf("%d %s: %+v, retryable %v, may have sent %v", want.status, want.name, problem, problem.Retryable(), MayHaveSent(err))
		}
	}
}

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

// A path the server does not have is not a missing record: a client
// pointed at a wrong address must not read every email as gone. Nor is a
// 404 from something else at that address.
func TestAWrongAddressIsNotAMissingRecord(t *testing.T) {
	for name, c := range map[string]struct {
		status       int
		body         string
		missing, off bool
	}{
		"a record that is not there": {404, `{"statusCode":404,"name":"not_found","message":"No email has this id."}`, true, false},
		"a path that is not there":   {404, `{"statusCode":404,"name":"route_not_found","message":"There is nothing at this path."}`, false, true},
		"a method the path refuses":  {405, `{"statusCode":405,"name":"method_not_allowed","message":"This path does not take PUT."}`, false, true},
		"a proxy's own 404":          {404, `<html>not here</html>`, false, true},
		"a refusal":                  {422, `{"statusCode":422,"name":"validation_error","message":"no"}`, false, false},
	} {
		_, client := serve(t, c.status, c.body)
		_, err := client.Get(ctx, "0199")
		if IsNotFound(err) != c.missing || IsWrongAddress(err) != c.off {
			t.Errorf("%s: missing %v, wrong address %v: %v", name, IsNotFound(err), IsWrongAddress(err), err)
		}
	}
	// The server's own sentence is kept, so the error says what to check.
	_, client := serve(t, 404, `{"statusCode":404,"name":"route_not_found","message":"There is nothing at this path. Check the path and the server's address."}`)
	if _, err := client.Get(ctx, "0199"); err == nil || !strings.Contains(err.Error(), "Check the path and the server's address") {
		t.Fatalf("%v", err)
	}
	// A redirect is something else answering at that address too.
	_, client = serving(t, func(w http.ResponseWriter, _ int) {
		w.Header().Set("Location", "/elsewhere")
		w.WriteHeader(http.StatusPermanentRedirect)
	})
	if _, err := client.Get(ctx, "0199"); !IsWrongAddress(err) || IsNotFound(err) {
		t.Fatalf("a redirect: %v", err)
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
