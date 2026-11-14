package cairnpost

import (
	"errors"
	"io"
	"net/http"
	"testing"
)

func TestLimitsHealthAndCaptured(t *testing.T) {
	_, c := serve(t, 200, `{"driver":"capture","quota":null}`)
	if l, err := c.Limits(ctx); err != nil || l.Quota != nil || l.Driver != "capture" {
		t.Fatalf("%+v %v", l, err)
	}
	_, c = serve(t, 200, `{"driver":"cloudflare","quota":{"limit":200,"per":"day","used":7,"over":false,"resets_at":"2030-01-02T00:00:00Z"}}`)
	if l, err := c.Limits(ctx); err != nil || l.Quota == nil || l.Quota.Limit != 200 || l.Quota.ResetsAt == nil {
		t.Fatalf("%+v %v", l, err)
	}
	// A server that is up with its database down is answered, not an error.
	s, c := serve(t, 503, `{"status":"unavailable","driver":"capture","version":"dev","events":"off","webhooks":"sending","pruning":"failing"}`)
	h, err := c.Health(ctx)
	if err != nil || h.OK() || h.Pruning != "failing" || s.seen[0].auth != "" {
		t.Fatalf("%+v %v, key sent: %q", h, err, s.seen[0].auth)
	}
	_, c = serve(t, 502, `<html>bad gateway</html>`)
	if _, err := c.Health(ctx); err == nil {
		t.Fatal("something that is not CairnPost was taken as healthy")
	}
	_, c = serve(t, 200, `{"data":[{"id":"c1","from":"app@example.com","to":["ana@example.org"],"cc":["bo@example.org"],"subject":"Hello","text":"Body"}]}`)
	if held, err := c.Captured(ctx); err != nil || len(held) != 1 || held[0].Subject != "Hello" {
		t.Fatalf("%+v %v", held, err)
	}
}

// A base URL that is wrong fails Health as it fails any call, so a
// configuration can be checked with it.
func TestHealthSaysAWrongAddress(t *testing.T) {
	for name, c := range map[string]struct {
		status int
		body   string
	}{
		"a path the server does not have": {404, `{"statusCode":404,"name":"route_not_found","message":"There is nothing at this path."}`},
		"something else at that address":  {404, `<html>not here</html>`},
	} {
		_, client := serve(t, c.status, c.body)
		if _, err := client.Health(ctx); !IsWrongAddress(err) {
			t.Errorf("%s: %v", name, err)
		}
	}
}

// A server that delivers for real has no captured emails to show, which
// is not a wrong address: the server is there. With nothing of
// CairnPost's at the address at all, the wrong address is what is said.
func TestCapturedOfAServerThatDeliversForReal(t *testing.T) {
	const nothing = `{"statusCode":404,"name":"route_not_found","message":"There is nothing at this path."}`
	s, c := serving(t, func(w http.ResponseWriter, n int) {
		if n == 1 {
			w.WriteHeader(404)
			io.WriteString(w, nothing)
			return
		}
		io.WriteString(w, `{"status":"ok","driver":"cloudflare","version":"dev"}`)
	})
	if held, err := c.Captured(ctx); !errors.Is(err, ErrNotCapturing) || held != nil || IsWrongAddress(err) ||
		len(s.seen) != 2 || s.seen[1].path != "/health" {
		t.Fatalf("%+v %v %+v", held, err, s.seen)
	}
	_, c = serve(t, 404, nothing)
	if _, err := c.Captured(ctx); errors.Is(err, ErrNotCapturing) || !IsWrongAddress(err) {
		t.Fatalf("a wrong base URL: %v", err)
	}
}
