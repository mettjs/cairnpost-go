package cairnpost

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"
)

// answering is every call that reads an answer, each made against a
// server that says something CairnPost never does.
var answering = map[string]func(context.Context, *Client) error{
	"Send": func(ctx context.Context, c *Client) error {
		_, err := c.Send(ctx, Email{From: "app@example.com", To: []string{"ana@example.org"}, Text: "t"})
		return err
	},
	"SendBatch": func(ctx context.Context, c *Client) error {
		_, err := c.SendBatch(ctx, Batch{Emails: []Email{{From: "app@example.com", To: []string{"ana@example.org"}, Text: "t"}}})
		return err
	},
	"Get":        func(ctx context.Context, c *Client) error { _, err := c.Get(ctx, "m"); return err },
	"Cancel":     func(ctx context.Context, c *Client) error { return c.Cancel(ctx, "m") },
	"Reschedule": func(ctx context.Context, c *Client) error { return c.Reschedule(ctx, "m", time.Unix(1900000000, 0)) },
	"Limits":     func(ctx context.Context, c *Client) error { _, err := c.Limits(ctx); return err },
	"Health":     func(ctx context.Context, c *Client) error { _, err := c.Health(ctx); return err },
	"Captured":   func(ctx context.Context, c *Client) error { _, err := c.Captured(ctx); return err },
	"Suppressions": func(ctx context.Context, c *Client) error {
		_, err := c.Suppressions(ctx, SuppressionQuery{})
		return err
	},
	"Suppress": func(ctx context.Context, c *Client) error {
		_, err := c.Suppress(ctx, "example.com", "a@example.org", "")
		return err
	},
	"Lift":          func(ctx context.Context, c *Client) error { return c.Lift(ctx, "example.com", "a@example.org") },
	"LiftAll":       func(ctx context.Context, c *Client) error { _, err := c.LiftAll(ctx, "example.com", ""); return err },
	"CreateWebhook": func(ctx context.Context, c *Client) error { _, err := c.CreateWebhook(ctx, NewWebhook{}); return err },
	"Webhooks":      func(ctx context.Context, c *Client) error { _, err := c.Webhooks(ctx); return err },
	"Webhook":       func(ctx context.Context, c *Client) error { _, err := c.Webhook(ctx, "w"); return err },
	"UpdateWebhook": func(ctx context.Context, c *Client) error {
		_, err := c.UpdateWebhook(ctx, "w", WebhookChange{})
		return err
	},
	"DeleteWebhook":       func(ctx context.Context, c *Client) error { return c.DeleteWebhook(ctx, "w") },
	"RotateWebhookSecret": func(ctx context.Context, c *Client) error { _, err := c.RotateWebhookSecret(ctx, "w"); return err },
	"ResumeWebhook":       func(ctx context.Context, c *Client) error { _, err := c.ResumeWebhook(ctx, "w"); return err },
	"TestWebhook":         func(ctx context.Context, c *Client) error { _, err := c.TestWebhook(ctx, "w"); return err },
	"Deliveries": func(ctx context.Context, c *Client) error {
		_, err := c.Deliveries(ctx, "w", DeliveryQuery{})
		return err
	},
	"RetryDelivery":  func(ctx context.Context, c *Client) error { return c.RetryDelivery(ctx, "w", "d") },
	"CreateTemplate": func(ctx context.Context, c *Client) error { _, err := c.CreateTemplate(ctx, NewTemplate{}); return err },
	"Templates":      func(ctx context.Context, c *Client) error { _, err := c.Templates(ctx, ""); return err },
	"Template":       func(ctx context.Context, c *Client) error { _, err := c.Template(ctx, "t"); return err },
	"UpdateTemplate": func(ctx context.Context, c *Client) error {
		_, err := c.UpdateTemplate(ctx, "t", TemplateChange{})
		return err
	},
	"DeleteTemplate": func(ctx context.Context, c *Client) error { return c.DeleteTemplate(ctx, "t") },
	"ReceivedList":   func(ctx context.Context, c *Client) error { _, err := c.ReceivedList(ctx, ReceivedQuery{}); return err },
	"Received":       func(ctx context.Context, c *Client) error { _, err := c.Received(ctx, "r"); return err },
	"ReceivedRaw":    func(ctx context.Context, c *Client) error { _, err := c.ReceivedRaw(ctx, "r"); return err },
	"ReceivedFile":   func(ctx context.Context, c *Client) error { _, err := c.ReceivedFile(ctx, "r", 1); return err },
	"FileContent":    func(ctx context.Context, c *Client) error { _, err := c.FileContent(ctx, "m", 1); return err },
	"DeleteReceived": func(ctx context.Context, c *Client) error { return c.DeleteReceived(ctx, "r") },
}

// Something else at the address, or between, can answer 200 to anything.
// None of it may read as "done", "no" or "none".
func TestNoCallTakesAnAnswerThatIsNotCairnPosts(t *testing.T) {
	bodies := map[string]string{
		"an empty object": `{}`,
		"null":            `null`,
		"a page":          `<html>maintenance</html>`,
		"nothing":         ``,
		"every field null": `{"id":null,"state":null,"data":null,"added":null,"lifted":null,"delivered":null,"driver":null,
			"status":null,"created_at":null,"updated_at":null,"received_at":null,"kept_until":null}`,
	}
	for what, body := range bodies {
		for name, ask := range answering {
			_, c := serve(t, 200, body)
			if err := ask(ctx, c); !errors.Is(err, errNotCairnPost) {
				t.Errorf("%s answered %s: %v", name, what, err)
			}
		}
	}
}

// A record says when it was made: one that does not was not written by
// CairnPost, alone or in a list.
func TestARecordWithoutItsTimesIsRefused(t *testing.T) {
	const body = `{"id":"x","data":[{"id":"x"}]}`
	for _, name := range []string{"Get", "Captured", "Suppressions", "CreateWebhook", "Webhooks", "Webhook", "UpdateWebhook",
		"RotateWebhookSecret", "ResumeWebhook", "Deliveries", "CreateTemplate", "Templates", "Template", "UpdateTemplate",
		"ReceivedList", "Received"} {
		_, c := serve(t, 200, body)
		if err := answering[name](ctx, c); !errors.Is(err, errNotCairnPost) {
			t.Errorf("%s: %v", name, err)
		}
	}
}

// An answer can be a whole object of CairnPost's kind and still not be
// the answer to what was asked.
func TestAnAnswerAboutSomethingElseIsRefused(t *testing.T) {
	const at = `"created_at":"2030-01-01T00:00:00Z","updated_at":"2030-01-01T00:00:00Z","received_at":"2030-01-01T00:00:00Z","kept_until":"2030-01-01T00:00:00Z"`
	for name, body := range map[string]string{
		"Cancel":              `{"id":"req-123","state":"cancelled"}`,
		"Reschedule":          `{"id":"req-123","scheduled_at":"2030-01-31T08:00:00Z"}`,
		"Lift":                `{"lifted":0}`,
		"LiftAll":             `{"lifted":-1}`,
		"RetryDelivery":       `{"state":"error"}`,
		"CreateWebhook":       `{"id":"w1",` + at + `}`,
		"RotateWebhookSecret": `{"id":"w1",` + at + `}`,
		"Health":              `{"status":"ok"}`,
		"Webhooks":            `{"data":[{` + at + `}]}`,
		"Templates":           `{"data":[{` + at + `}]}`,
		"ReceivedList":        `{"data":[{` + at + `}]}`,
		"Deliveries":          `{"data":[{` + at + `}]}`,
		"Suppressions":        `{"data":[{` + at + `}]}`,
		"Captured":            `{"data":[{"at":"2030-01-01T00:00:00Z"}]}`,
	} {
		_, c := serve(t, 200, body)
		if err := answering[name](ctx, c); !errors.Is(err, errNotCairnPost) {
			t.Errorf("%s took %s: %v", name, body, err)
		}
	}
	// A value never comes with an error.
	_, c := serve(t, 200, `{"data":[{"id":"c1"}],"added":"yes","lifted":"7"}`)
	if held, err := c.Captured(ctx); err == nil || held != nil {
		t.Errorf("%+v %v", held, err)
	}
	if added, err := c.Suppress(ctx, "example.com", "a@example.org", ""); err == nil || added {
		t.Errorf("%v %v", added, err)
	}
	if n, err := c.LiftAll(ctx, "example.com", ""); err == nil || n != 0 {
		t.Errorf("%v %v", n, err)
	}
}

// The table above is held to every call there is: a new call is put in
// front of the same answers, or this fails.
func TestEveryCallIsInTheTable(t *testing.T) {
	// These read no answer of their own: they walk a call that is there.
	walks := map[string]bool{"AllSuppressions": true, "AllDeliveries": true, "AllReceived": true}
	of := reflect.TypeOf(&Client{})
	for i := range of.NumMethod() {
		name := of.Method(i).Name
		if _, ok := answering[name]; !ok && !walks[name] {
			t.Errorf("%s is not in the table of calls", name)
		}
	}
}

// A call that answers nothing is done only on CairnPost's own 204.
func TestACallThatAnswersNothingWantsA204(t *testing.T) {
	for _, name := range []string{"DeleteWebhook", "DeleteTemplate", "DeleteReceived"} {
		for _, status := range []int{200, 202} {
			_, c := serve(t, status, ``)
			if err := answering[name](ctx, c); !errors.Is(err, errNotCairnPost) {
				t.Errorf("%s took a %d: %v", name, status, err)
			}
		}
		_, c := serve(t, 204, ``)
		if err := answering[name](ctx, c); err != nil {
			t.Errorf("%s refused a 204: %v", name, err)
		}
	}
}

// What CairnPost says of nothing is still read as nothing.
func TestAnEmptyListAndANoAreStillRead(t *testing.T) {
	_, c := serve(t, 200, `{"data":[],"added":false,"lifted":0,"delivered":false,"more":false}`)
	if page, err := c.Suppressions(ctx, SuppressionQuery{}); err != nil || len(page.Data) != 0 || page.Next != "" {
		t.Fatalf("%+v %v", page, err)
	}
	if list, err := c.Webhooks(ctx); err != nil || len(list) != 0 {
		t.Fatalf("%+v %v", list, err)
	}
	if list, err := c.Templates(ctx, ""); err != nil || len(list.Data) != 0 || list.More {
		t.Fatalf("%+v %v", list, err)
	}
	if list, err := c.Captured(ctx); err != nil || len(list) != 0 {
		t.Fatalf("%+v %v", list, err)
	}
	if added, err := c.Suppress(ctx, "example.com", "a@example.org", ""); err != nil || added {
		t.Fatalf("%v %v", added, err)
	}
	if n, err := c.LiftAll(ctx, "example.com", ""); err != nil || n != 0 {
		t.Fatalf("%v %v", n, err)
	}
	if got, err := c.TestWebhook(ctx, "w"); err != nil || got.Delivered {
		t.Fatalf("%+v %v", got, err)
	}
}
