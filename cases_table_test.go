package cairnpost

import (
	"context"
	"reflect"
	"testing"
	"time"
)

const (
	anEmail    = "01a20155-8f2b-76c9-bc68-b2e7bdcd4c56"
	anEndpoint = "01a20155-8f42-7fcf-bf8a-120e8aa42bf6"
	aDelivery  = "01a20155-8f59-7f5a-ba31-4cfbc0f1e864"
	aTemplate  = "01a20155-9b2b-7b2a-8f83-ef332546a80b"
	aReceived  = "01a20155-8f58-7e44-b3a0-1fe938616579"
)

func some[T any](v T) *T { return &v }

// asking is every call the client has: the name the shared cases know it
// by, its method here, and the call that sends what calls.json says it
// sends.
var asking = []struct {
	name, method string
	ask          func(context.Context, *Client) error
}{
	{"health", "Health", func(ctx context.Context, c *Client) error { _, err := c.Health(ctx); return err }},
	{"limits", "Limits", func(ctx context.Context, c *Client) error { _, err := c.Limits(ctx); return err }},
	{"captured", "Captured", func(ctx context.Context, c *Client) error { _, err := c.Captured(ctx); return err }},
	{"emails.send", "Send", func(ctx context.Context, c *Client) error {
		_, err := c.Send(ctx, Email{From: "App <app@example.com>", To: []string{"ana@example.org"}, Cc: []string{"ben@example.org"},
			ReplyTo: "help@example.com", Subject: "Welcome", Text: "Hello.", HTML: "<p>Hello.</p>",
			Headers:        map[string]string{"X-Entity-Ref": "42"},
			Attachments:    []Attachment{{Filename: "a.txt", Content: []byte("hello"), ContentType: "text/plain"}},
			IdempotencyKey: "welcome-42"})
		return err
	}},
	{"emails.send_batch", "SendBatch", func(ctx context.Context, c *Client) error {
		_, err := c.SendBatch(ctx, Batch{IdempotencyKey: "batch-7", Emails: []Email{
			{From: "app@example.com", To: []string{"ana@example.org"}, Subject: "Welcome", Text: "Hello."},
			{From: "app@example.com", To: []string{"ben@example.org"}, Subject: "Welcome", Text: "Hello."}}})
		return err
	}},
	{"emails.get", "Get", func(ctx context.Context, c *Client) error { _, err := c.Get(ctx, anEmail); return err }},
	{"emails.cancel", "Cancel", func(ctx context.Context, c *Client) error { return c.Cancel(ctx, anEmail) }},
	{"emails.reschedule", "Reschedule", func(ctx context.Context, c *Client) error {
		return c.Reschedule(ctx, anEmail, time.Date(2030, 1, 31, 8, 0, 0, 0, time.UTC))
	}},
	{"emails.file", "FileContent", func(ctx context.Context, c *Client) error { _, err := c.FileContent(ctx, anEmail, 1); return err }},
	{"suppressions.list", "Suppressions", func(ctx context.Context, c *Client) error {
		_, err := c.Suppressions(ctx, SuppressionQuery{Domain: "example.com", Limit: 2})
		return err
	}},
	{"suppressions.add", "Suppress", func(ctx context.Context, c *Client) error {
		_, err := c.Suppress(ctx, "example.com", "gone@example.org", "asked")
		return err
	}},
	{"suppressions.lift", "Lift", func(ctx context.Context, c *Client) error { return c.Lift(ctx, "example.com", "gone@example.org") }},
	{"suppressions.lift_all", "LiftAll", func(ctx context.Context, c *Client) error {
		_, err := c.LiftAll(ctx, "example.com", ReasonBounce)
		return err
	}},
	{"webhooks.create", "CreateWebhook", func(ctx context.Context, c *Client) error {
		_, err := c.CreateWebhook(ctx, NewWebhook{URL: "https://app.example.com/hooks/cairnpost",
			Events: []string{EventBounced, EventComplained}, Description: "Bounces"})
		return err
	}},
	{"webhooks.list", "Webhooks", func(ctx context.Context, c *Client) error { _, err := c.Webhooks(ctx); return err }},
	{"webhooks.get", "Webhook", func(ctx context.Context, c *Client) error { _, err := c.Webhook(ctx, anEndpoint); return err }},
	{"webhooks.update", "UpdateWebhook", func(ctx context.Context, c *Client) error {
		_, err := c.UpdateWebhook(ctx, anEndpoint, WebhookChange{Description: some("Bounces and complaints"), Enabled: some(false)})
		return err
	}},
	{"webhooks.delete", "DeleteWebhook", func(ctx context.Context, c *Client) error { return c.DeleteWebhook(ctx, anEndpoint) }},
	{"webhooks.rotate_secret", "RotateWebhookSecret", func(ctx context.Context, c *Client) error {
		_, err := c.RotateWebhookSecret(ctx, anEndpoint)
		return err
	}},
	{"webhooks.resume", "ResumeWebhook", func(ctx context.Context, c *Client) error { _, err := c.ResumeWebhook(ctx, anEndpoint); return err }},
	{"webhooks.test", "TestWebhook", func(ctx context.Context, c *Client) error { _, err := c.TestWebhook(ctx, anEndpoint); return err }},
	{"webhooks.deliveries", "Deliveries", func(ctx context.Context, c *Client) error {
		_, err := c.Deliveries(ctx, anEndpoint, DeliveryQuery{State: "pending", Limit: 2})
		return err
	}},
	{"webhooks.retry_delivery", "RetryDelivery", func(ctx context.Context, c *Client) error {
		return c.RetryDelivery(ctx, anEndpoint, aDelivery)
	}},
	{"templates.create", "CreateTemplate", func(ctx context.Context, c *Client) error {
		_, err := c.CreateTemplate(ctx, NewTemplate{Name: "welcome", Subject: "Hello {{name}}", Text: "Hi {{name}}.",
			Defaults: map[string]string{"name": "there"}})
		return err
	}},
	{"templates.list", "Templates", func(ctx context.Context, c *Client) error { _, err := c.Templates(ctx, "example.com"); return err }},
	{"templates.get", "Template", func(ctx context.Context, c *Client) error { _, err := c.Template(ctx, aTemplate); return err }},
	{"templates.update", "UpdateTemplate", func(ctx context.Context, c *Client) error {
		_, err := c.UpdateTemplate(ctx, aTemplate, TemplateChange{Subject: some("Welcome {{name}}")})
		return err
	}},
	{"templates.delete", "DeleteTemplate", func(ctx context.Context, c *Client) error { return c.DeleteTemplate(ctx, aTemplate) }},
	{"received.list", "ReceivedList", func(ctx context.Context, c *Client) error {
		_, err := c.ReceivedList(ctx, ReceivedQuery{Limit: 2})
		return err
	}},
	{"received.get", "Received", func(ctx context.Context, c *Client) error { _, err := c.Received(ctx, aReceived); return err }},
	{"received.raw", "ReceivedRaw", func(ctx context.Context, c *Client) error { _, err := c.ReceivedRaw(ctx, aReceived); return err }},
	{"received.file", "ReceivedFile", func(ctx context.Context, c *Client) error { _, err := c.ReceivedFile(ctx, aReceived, 1); return err }},
	{"received.delete", "DeleteReceived", func(ctx context.Context, c *Client) error { return c.DeleteReceived(ctx, aReceived) }},
}

// ask answers the call the shared cases know by a name.
func ask(t *testing.T, name string) func(context.Context, *Client) error {
	t.Helper()
	for _, a := range asking {
		if a.name == name {
			return a.ask
		}
	}
	t.Fatalf("the cases name %q, which is no call of this client", name)
	return nil
}

// The table is held to every call there is, so that a new call is put in
// front of the shared cases, or this fails.
func TestEveryCallIsInTheTable(t *testing.T) {
	// These read no answer of their own: they walk a call that is there.
	walks := map[string]bool{"AllSuppressions": true, "AllDeliveries": true, "AllReceived": true}
	listed := map[string]bool{}
	for _, a := range asking {
		listed[a.method] = true
	}
	of := reflect.TypeOf(&Client{})
	for i := range of.NumMethod() {
		if name := of.Method(i).Name; !listed[name] && !walks[name] {
			t.Errorf("%s is not in the table of calls", name)
		}
	}
	if len(listed) != len(asking) || len(listed)+len(walks) != of.NumMethod() {
		t.Errorf("%d rows for %d methods, of %d the client has", len(asking), len(listed), of.NumMethod())
	}
}
