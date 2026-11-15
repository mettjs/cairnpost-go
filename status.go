package cairnpost

import (
	"context"
	"errors"
	"net/http"
	"time"
)

// Quota is the mail provider's sending allowance and how much is used.
type Quota struct {
	Limit int `json:"limit"`
	// Per is the span the limit counts over, such as "day".
	Per  string `json:"per"`
	Used int    `json:"used"`
	// Over says the allowance is used up.
	Over     bool       `json:"over"`
	ResetsAt *time.Time `json:"resets_at,omitempty"`
}

// Limits is what the server knows of how much more it may send.
type Limits struct {
	// Driver is the mail provider in use.
	Driver string `json:"driver"`
	// Quota is nil for a provider that has no allowance to tell.
	Quota *Quota `json:"quota"`
}

// Limits answers the mail provider's sending allowance, so an application
// knows before it runs out.
func (c *Client) Limits(ctx context.Context) (*Limits, error) {
	var l Limits
	if err := c.do(ctx, call{method: http.MethodGet, path: []string{"limits"}}, &l); err != nil {
		return nil, err
	}
	return &l, nil
}

// Health is whether the server can do its work.
type Health struct {
	// Status is "ok", or "unavailable" when its database does not answer.
	Status  string `json:"status"`
	Driver  string `json:"driver"`
	Version string `json:"version"`
	// Events is whether delivery events are being read: "reading",
	// "failing" or "off". Webhooks, whether they are being sent:
	// "sending", "failing" or "off". Pruning, whether old records are
	// being deleted: "running", "failing" or "off". Mail still goes
	// while any of them is failing.
	Events   string `json:"events"`
	Webhooks string `json:"webhooks"`
	Pruning  string `json:"pruning"`
}

// OK says the server is up and its database answers.
func (h *Health) OK() bool { return h.Status == "ok" }

// Health asks the server how it is. It needs no key. A server that is up
// but cannot reach its database is answered, not an error: see OK. With a
// base URL that is wrong it fails as any call does, and IsWrongAddress
// says so: it is the call to check a configuration with.
func (c *Client) Health(ctx context.Context) (*Health, error) {
	var h Health
	if err := c.do(ctx, call{method: http.MethodGet, path: []string{"health"}, plain: true}, &h); err != nil {
		return nil, err
	}
	if h.Status == "" {
		return nil, &TransportError{cause: errNotCairnPost}
	}
	return &h, nil
}

// CapturedEmail is an email a development server kept instead of sending.
type CapturedEmail struct {
	ID       string            `json:"id"`
	At       time.Time         `json:"at"`
	From     string            `json:"from"`
	FromName string            `json:"from_name,omitempty"`
	To       []string          `json:"to"`
	Cc       []string          `json:"cc,omitempty"`
	Bcc      []string          `json:"bcc,omitempty"`
	ReplyTo  string            `json:"reply_to,omitempty"`
	Subject  string            `json:"subject"`
	HTML     string            `json:"html,omitempty"`
	Text     string            `json:"text,omitempty"`
	Headers  map[string]string `json:"headers,omitempty"`
	// Attachments say what files it had; a captured email holds no file's
	// bytes.
	Attachments []File `json:"attachments,omitempty"`
}

// ErrNotCapturing is Captured asked of a server that delivers for real:
// it keeps no emails to show.
var ErrNotCapturing = errors.New("cairnpost: the server delivers for real and keeps no captured emails")

// Captured answers the emails a server running the capture driver holds,
// newest first: what would have been sent. For development and tests. A
// server that delivers for real has no such list: the error is then
// ErrNotCapturing (ask with errors.Is), unless Health fails too, in which
// case the base URL is what is wrong. Admin key.
func (c *Client) Captured(ctx context.Context) ([]CapturedEmail, error) {
	var out struct {
		Data []CapturedEmail `json:"data"`
	}
	err := c.do(ctx, call{method: http.MethodGet, path: []string{"capture", "emails"}}, &out)
	var problem *Error
	if errors.As(err, &problem) && problem.Name == ErrRouteNotFound {
		// The path is not there. If the server is, it is one that does
		// not capture; if it is not, that is the error to give.
		if _, down := c.Health(ctx); down != nil {
			return nil, err
		}
		return nil, ErrNotCapturing
	}
	return out.Data, err
}
