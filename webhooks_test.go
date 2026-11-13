package cairnpost

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"testing"
)

// A change says exactly what is set: false and an empty list are sent,
// and what is nil is not sent at all.
func TestAWebhookChangeCanSwitchOffAndClear(t *testing.T) {
	s, c := serve(t, 200, `{"id":"w1","url":"https://example.com/hook","events":[],"enabled":false,"created_at":"2030-01-01T00:00:00Z"}`)
	off, none := false, []string{}
	w, err := c.UpdateWebhook(ctx, "w1", WebhookChange{Enabled: &off, Events: &none})
	if err != nil || w.Enabled || s.seen[0].method != "PATCH" || s.seen[0].path != "/webhooks/w1" {
		t.Fatalf("%+v %v %+v", w, err, s.seen[0])
	}
	var sent map[string]json.RawMessage
	if err := json.Unmarshal([]byte(s.seen[0].body), &sent); err != nil || len(sent) != 2 ||
		string(sent["enabled"]) != "false" || string(sent["events"]) != "[]" {
		t.Fatalf("%s", s.seen[0].body)
	}
	// A nil list is an empty one to whoever wrote it, and is sent as one.
	var every []string
	if _, err := c.UpdateWebhook(ctx, "w1", WebhookChange{Events: &every}); err != nil || s.seen[1].body != `{"events":[]}`+"\n" || every != nil {
		t.Fatalf("%v %s", err, s.seen[1].body)
	}
}

func TestWebhooksAndDeliveries(t *testing.T) {
	s, c := serving(t, func(w http.ResponseWriter, n int) {
		switch n {
		case 1:
			w.WriteHeader(201)
			io.WriteString(w, `{"id":"w1","url":"https://example.com/hook","events":["email.bounced"],"enabled":true,"secret":"whsec_c2VjcmV0"}`)
		case 2:
			io.WriteString(w, `{"data":[{"id":"w1","url":"https://example.com/hook","events":[],"enabled":true}]}`)
		case 3:
			io.WriteString(w, `{"data":[{"id":"d2","email_id":"m","type":"email.bounced","state":"failed","attempts":8,"last_status":500,"last_failure":"status"}],"next":"d2"}`)
		case 4:
			io.WriteString(w, `{"data":[{"id":"d1","email_id":"m","type":"email.delivered","state":"failed","attempts":8,"last_failure":"timeout"}]}`)
		case 5:
			io.WriteString(w, `{"state":"pending"}`)
		case 6:
			io.WriteString(w, `{"delivered":false,"status":500,"failure":"status"}`)
		case 7:
			w.WriteHeader(204)
		}
	})
	made, err := c.CreateWebhook(ctx, NewWebhook{URL: "https://example.com/hook", Events: []string{EventBounced}})
	if err != nil || made.Secret != "whsec_c2VjcmV0" || s.seen[0].body != `{"url":"https://example.com/hook","events":["email.bounced"]}`+"\n" {
		t.Fatalf("%+v %v %s", made, err, s.seen[0].body)
	}
	if list, err := c.Webhooks(ctx); err != nil || len(list) != 1 || list[0].ID != "w1" {
		t.Fatalf("%+v %v", list, err)
	}
	var ids []string
	for d, err := range c.AllDeliveries(ctx, "w1", DeliveryFailed) {
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, d.ID)
	}
	if fmt.Sprint(ids) != "[d2 d1]" || s.seen[2].path != "/webhooks/w1/deliveries" || s.seen[2].query != "limit=500&state=failed" ||
		s.seen[3].query != "after=d2&limit=500&state=failed" {
		t.Fatalf("%v %+v", ids, s.seen[2:4])
	}
	if err := c.RetryDelivery(ctx, "w1", "d1"); err != nil || s.seen[4].path != "/webhooks/w1/deliveries/d1/retry" {
		t.Fatalf("%v %+v", err, s.seen[4])
	}
	if res, err := c.TestWebhook(ctx, "w1"); err != nil || res.Delivered || res.Status != 500 || res.Failure != "status" {
		t.Fatalf("%+v %v", res, err)
	}
	if err := c.DeleteWebhook(ctx, "w1"); err != nil || s.seen[6].method != "DELETE" {
		t.Fatalf("%v", err)
	}
}
