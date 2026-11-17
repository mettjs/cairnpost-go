package cairnpost

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

// seen is a request as the server got it.
type seen struct {
	method, path, rawPath, query, auth, idem, agent, contentType string
	body                                                         string
}

// stand is a server that records each request and answers what it is told.
type stand struct {
	t      *testing.T
	seen   []seen
	answer func(w http.ResponseWriter, n int)
}

func (s *stand) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	s.seen = append(s.seen, seen{r.Method, r.URL.Path, r.URL.EscapedPath(), r.URL.RawQuery, r.Header.Get("Authorization"),
		r.Header.Get("Idempotency-Key"), r.Header.Get("User-Agent"), r.Header.Get("Content-Type"), string(body)})
	s.answer(w, len(s.seen))
}

// serve starts a stand answering each request with a status and a body,
// and a client of it.
func serve(t *testing.T, status int, body string) (*stand, *Client) {
	t.Helper()
	return serving(t, func(w http.ResponseWriter, _ int) { w.WriteHeader(status); io.WriteString(w, body) })
}

func serving(t *testing.T, answer func(http.ResponseWriter, int)) (*stand, *Client) {
	t.Helper()
	s := &stand{t: t, answer: answer}
	srv := httptest.NewServer(s)
	t.Cleanup(srv.Close)
	c, err := New(srv.URL+"/", "cp_test_KEYMARK")
	if err != nil {
		t.Fatal(err)
	}
	return s, c
}

var ctx = context.Background()

func TestNewRefusesWhatCannotWork(t *testing.T) {
	for _, base := range []string{"", "mail.example.com", "ftp://mail.example.com", "https://"} {
		if _, err := New(base, "key"); err == nil {
			t.Errorf("%q was taken as a base URL", base)
		}
	}
	for _, key := range []string{"", " ", "cp_a\nb", "cp_a\x00b"} {
		if _, err := New("https://mail.example.com", key); err == nil {
			t.Errorf("%q was taken as a key", key)
		}
	}
}

func TestABaseURLWithAPathIsKept(t *testing.T) {
	s := &stand{t: t, answer: func(w http.ResponseWriter, _ int) {
		io.WriteString(w, `{"id":"m","created_at":"2030-01-01T00:00:00Z","updated_at":"2030-01-01T00:00:00Z"}`)
	}}
	srv := httptest.NewServer(s)
	defer srv.Close()
	c, err := New(srv.URL+"/mail/", "key", WithUserAgent("bad\nname"), WithUserAgent("billing/2"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Get(ctx, "m"); err != nil || s.seen[0].path != "/mail/emails/m" || !strings.HasPrefix(s.seen[0].agent, "billing/2 cairnpost-go/") {
		t.Fatalf("%v %+v", err, s.seen[0])
	}
}

// A key read from a file ends in a newline: it is sent without it.
func TestAKeyIsSentWithoutTheSpaceAroundIt(t *testing.T) {
	s := &stand{t: t, answer: func(w http.ResponseWriter, _ int) {
		io.WriteString(w, `{"id":"m","state":"sent","created_at":"2030-01-01T00:00:00Z","updated_at":"2030-01-01T00:00:00Z"}`)
	}}
	srv := httptest.NewServer(s)
	defer srv.Close()
	c, err := New(srv.URL, " cp_test_KEY\n")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Get(ctx, "m"); err != nil || s.seen[0].auth != "Bearer cp_test_KEY" {
		t.Fatalf("%v %q", err, s.seen[0].auth)
	}
}

// A redirect is not followed: it would turn a send into a request with no
// email in it. It is said as what it is.
func TestARedirectIsNotFollowed(t *testing.T) {
	s, c := serving(t, func(w http.ResponseWriter, _ int) {
		w.Header().Set("Location", "/elsewhere")
		w.WriteHeader(http.StatusMovedPermanently)
	})
	_, err := c.Send(ctx, Email{From: "a@example.com", To: []string{"b@example.org"}, Subject: "s", Text: "t"})
	var problem *Error
	if !errors.As(err, &problem) || problem.Status != 301 || len(s.seen) != 1 || IsNotFound(err) ||
		!strings.Contains(err.Error(), "redirect") {
		t.Fatalf("%v, %d requests", err, len(s.seen))
	}
}

// An answer longer than the client reads is said to be that.
func TestAnAnswerTooLargeSaysSo(t *testing.T) {
	s := &stand{t: t, answer: func(w http.ResponseWriter, _ int) {
		io.WriteString(w, `{"data":[{"id":"c1","at":"2030-01-01T00:00:00Z","subject":"`+strings.Repeat("x", 4096)+`"}]}`)
	}}
	srv := httptest.NewServer(s)
	defer srv.Close()
	small, _ := New(srv.URL, "key", WithMaxAnswerBytes(1024))
	if _, err := small.Captured(ctx); err == nil || !strings.Contains(err.Error(), "larger than the client reads") {
		t.Fatalf("%v", err)
	}
	roomy, _ := New(srv.URL, "key", WithMaxAnswerBytes(1<<20), WithHTTPClient(nil))
	if held, err := roomy.Captured(ctx); err != nil || len(held) != 1 {
		t.Fatalf("%v", err)
	}
}

// A refusal is known by its status, however long the page under it.
func TestALongRefusalKeepsItsStatus(t *testing.T) {
	page := "<html>" + strings.Repeat("x", 200<<10) + "</html>"
	for _, most := range []int64{1024, 1 << 20} {
		s := &stand{t: t, answer: func(w http.ResponseWriter, _ int) {
			w.WriteHeader(404)
			io.WriteString(w, page)
		}}
		srv := httptest.NewServer(s)
		c, _ := New(srv.URL, "key", WithMaxAnswerBytes(most))
		_, err := c.Send(ctx, Email{From: "a@example.com", To: []string{"b@example.org"}, Text: "t"})
		var problem *Error
		if !errors.As(err, &problem) || problem.Status != 404 || problem.Name != "" || MayHaveSent(err) || !IsWrongAddress(err) {
			t.Errorf("reading %d bytes: %v", most, err)
		}
		srv.Close()
	}
}

// The client makes each call once. net/http would by itself send again a
// request that carries an Idempotency-Key when a kept connection drops;
// the first might still be sending, and the second be told "in flight".
func TestASendIsMadeOnceEvenWhenTheConnectionDrops(t *testing.T) {
	for _, key := range []string{"", "welcome-42"} {
		var posts atomic.Int32
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodGet {
				io.WriteString(w, `{"status":"ok","driver":"capture","version":"dev"}`) // leaves a connection to reuse
				return
			}
			posts.Add(1)
			io.Copy(io.Discard, r.Body)
			if conn, _, err := w.(http.Hijacker).Hijack(); err == nil {
				conn.Close() // the request was read; hang up without an answer
			}
		}))
		c, _ := New(srv.URL, "key")
		if _, err := c.Health(ctx); err != nil {
			t.Fatal(err)
		}
		_, err := c.Send(context.Background(), Email{From: "a@example.com", To: []string{"b@example.org"}, Subject: "s", Text: "t", IdempotencyKey: key})
		var transport *TransportError
		if !errors.As(err, &transport) || !MayHaveSent(err) || posts.Load() != 1 {
			t.Errorf("key %q: %d requests reached the server, %v", key, posts.Load(), err)
		}
		srv.Close()
	}
}
