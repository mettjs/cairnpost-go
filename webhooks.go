package cairnpost

import (
	"context"
	"net/http"
	"time"
)

// Webhook is an endpoint: a URL CairnPost tells what becomes of emails.
type Webhook struct {
	ID  string `json:"id"`
	URL string `json:"url"`
	// Domain is the sending domain it hears about; "" for every domain.
	Domain string `json:"domain,omitempty"`
	// Events are the types it hears about (the Event constants); empty
	// for every type.
	Events      []string  `json:"events"`
	Description string    `json:"description,omitempty"`
	Enabled     bool      `json:"enabled"`
	CreatedAt   time.Time `json:"created_at"`
	// FailingSince is set while every try since then has failed.
	FailingSince *time.Time `json:"failing_since,omitempty"`
	// PausedAt is set while the endpoint is paused: it failed every try
	// for longer than the server allows. It is still owed everything that
	// happens, and nothing is sent to it until ResumeWebhook.
	PausedAt *time.Time `json:"paused_at,omitempty"`
	// PreviousSecretUntil is set after RotateWebhookSecret: until then
	// the server signs with the secret before as well as the new one. It
	// is nil when no older secret signs: none was changed lately, or the
	// server is set to end an old secret at once.
	PreviousSecretUntil *time.Time `json:"previous_secret_until,omitempty"`
	// Secret is the signing secret, for NewVerifier. It is set only on
	// the answers to CreateWebhook and RotateWebhookSecret and is never
	// said again: keep it.
	Secret string `json:"secret,omitempty"`
}

// NewWebhook is an endpoint to create. Only URL is required.
type NewWebhook struct {
	URL         string   `json:"url"`
	Domain      string   `json:"domain,omitempty"`
	Events      []string `json:"events,omitempty"`
	Description string   `json:"description,omitempty"`
}

// CreateWebhook has a URL told what becomes of emails. The answer carries
// the endpoint's signing Secret, the only time it is said. Admin key.
func (c *Client) CreateWebhook(ctx context.Context, n NewWebhook) (*Webhook, error) {
	return c.endpoint(ctx, call{method: http.MethodPost, path: []string{"webhooks"}, body: n})
}

// endpoint makes a call that answers one endpoint.
func (c *Client) endpoint(ctx context.Context, k call) (*Webhook, error) {
	var w Webhook
	if err := c.do(ctx, k, &w); err != nil {
		return nil, err
	}
	if w.ID == "" || !w.whole() {
		return nil, &TransportError{cause: errNotCairnPost}
	}
	return &w, nil
}

// Webhooks answers every endpoint. Admin key.
func (c *Client) Webhooks(ctx context.Context) ([]Webhook, error) {
	var out struct {
		Data []Webhook `json:"data"`
	}
	if err := c.do(ctx, call{method: http.MethodGet, path: []string{"webhooks"}, must: []string{"data"}}, &out); err != nil {
		return nil, err
	}
	if !each(out.Data, (*Webhook).whole) {
		return nil, &TransportError{cause: errNotCairnPost}
	}
	return out.Data, nil
}

// Webhook answers one endpoint. Admin key.
func (c *Client) Webhook(ctx context.Context, id string) (*Webhook, error) {
	return c.endpoint(ctx, call{method: http.MethodGet, path: []string{"webhooks", id}})
}

// WebhookChange is what to change about an endpoint. A nil field is left
// as it is; a field that is set is changed to exactly that, so Enabled
// pointing at false disables it and Events pointing at an empty list
// makes it hear about every type.
type WebhookChange struct {
	URL         *string   `json:"url,omitempty"`
	Events      *[]string `json:"events,omitempty"`
	Description *string   `json:"description,omitempty"`
	Enabled     *bool     `json:"enabled,omitempty"`
}

// UpdateWebhook changes an endpoint and answers it as it now is. A
// disabled endpoint is owed nothing new, and what it was owed waits. It
// does not resume a paused endpoint: ResumeWebhook does.
// Admin key.
func (c *Client) UpdateWebhook(ctx context.Context, id string, change WebhookChange) (*Webhook, error) {
	if change.Events != nil && *change.Events == nil {
		// A nil list is an empty one to whoever wrote it, but would be
		// sent as null, which the server reads as "not named".
		change.Events = &[]string{}
	}
	return c.endpoint(ctx, call{method: http.MethodPatch, path: []string{"webhooks", id}, body: change})
}

// DeleteWebhook removes an endpoint and all it was owed. Admin key.
func (c *Client) DeleteWebhook(ctx context.Context, id string) error {
	return c.do(ctx, call{method: http.MethodDelete, path: []string{"webhooks", id}}, nil)
}

// WebhookTest is how an endpoint answered a webhook sent to try it out.
type WebhookTest struct {
	// Delivered says the endpoint took it.
	Delivered bool `json:"delivered"`
	// Status is the endpoint's HTTP status; 0 when it gave none.
	Status int `json:"status,omitempty"`
	// Failure is why it was not delivered: "timeout", "refused", "dns",
	// "tls", "blocked", "status" or "other".
	Failure string `json:"failure,omitempty"`
}

// RotateWebhookSecret gives the endpoint a new signing secret and
// answers the endpoint with it, the only time it is said. Until the
// PreviousSecretUntil it answers (a day later, unless the server is set
// otherwise) the server signs each webhook with the old secret as well,
// so a Verifier made with either accepts it: change the application's
// secret within that time. A nil PreviousSecretUntil means the old secret
// has already stopped signing: put the new one in place at once.
//
// If the call fails with no answer (a TransportError), the secret may
// have been changed and its new value lost. Call it again: that gives
// another new secret, and the one the application holds goes on signing
// for its own time meanwhile. Admin key.
func (c *Client) RotateWebhookSecret(ctx context.Context, id string) (*Webhook, error) {
	return c.endpoint(ctx, call{method: http.MethodPost, path: []string{"webhooks", id, "secret"}})
}

// ResumeWebhook ends an endpoint's pause and answers the endpoint: what
// it is owed is sent then, with what was given up while it was failing.
// One that is not paused is answered as it is. Admin key.
func (c *Client) ResumeWebhook(ctx context.Context, id string) (*Webhook, error) {
	return c.endpoint(ctx, call{method: http.MethodPost, path: []string{"webhooks", id, "resume"}})
}

// TestWebhook sends the endpoint a signed webhook of type webhook.test
// now, and answers how it went. Admin key.
func (c *Client) TestWebhook(ctx context.Context, id string) (*WebhookTest, error) {
	var t WebhookTest
	k := call{method: http.MethodPost, path: []string{"webhooks", id, "test"}, must: []string{"delivered"}}
	if err := c.do(ctx, k, &t); err != nil {
		return nil, err
	}
	return &t, nil
}
