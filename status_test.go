package cairnpost

import (
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
	_, c = serve(t, 200, `{"data":[{"id":"c1","from":"app@example.com","to":"ana@example.org","subject":"Hello","text":"Body"}]}`)
	if held, err := c.Captured(ctx); err != nil || len(held) != 1 || held[0].Subject != "Hello" {
		t.Fatalf("%+v %v", held, err)
	}
}
