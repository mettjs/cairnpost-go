package cairnpost

import (
	"context"
	"errors"
	"net/http"
	"time"
)

// Email is one email to one recipient.
type Email struct {
	// From is the sender: "app@example.com" or "App <app@example.com>".
	From string
	// To is the one recipient's address.
	To string
	// ReplyTo is optional.
	ReplyTo string
	Subject string
	// HTML and Text are the bodies; at least one is required.
	HTML string
	Text string
	// Headers are extra headers, optional.
	Headers map[string]string
	// IdempotencyKey makes a repeated send one email: the same key with
	// the same content answers the first one's id. Use one for anything
	// that may be retried. It is remembered for a day by default.
	IdempotencyKey string
}

// Send sends an email and answers its id once the mail provider has taken
// it. On an error, see Error for what to do and MayHaveSent for whether
// the email may have gone; the error's EmailID, when set, can be read
// with Get.
func (c *Client) Send(ctx context.Context, e Email) (string, error) {
	// Only the fields the server names: it refuses any other.
	body := struct {
		From    string            `json:"from"`
		To      string            `json:"to"`
		ReplyTo string            `json:"reply_to,omitempty"`
		Subject string            `json:"subject"`
		HTML    string            `json:"html,omitempty"`
		Text    string            `json:"text,omitempty"`
		Headers map[string]string `json:"headers,omitempty"`
	}{e.From, e.To, e.ReplyTo, e.Subject, e.HTML, e.Text, e.Headers}
	k := call{method: http.MethodPost, path: []string{"emails"}, body: body}
	if e.IdempotencyKey != "" {
		// A key a header cannot carry as it is would be dropped or changed
		// on the way, and the send would quietly not be idempotent.
		if !headerSafe(e.IdempotencyKey) {
			return "", errors.New("cairnpost: the IdempotencyKey must have no control characters and no space at either end; nothing was sent")
		}
		k.header = http.Header{"Idempotency-Key": {e.IdempotencyKey}}
	}
	var out struct {
		ID string `json:"id"`
	}
	if err := c.do(ctx, k, &out); err != nil {
		return "", err
	}
	if out.ID == "" {
		// A 2xx with no id is not CairnPost's answer: whatever is at that
		// address, the email is not known to have been sent.
		return "", &TransportError{cause: errNotCairnPost}
	}
	return out.ID, nil
}

// States of an email.
const (
	StateSending    = "sending"    // handed to the provider, no answer yet
	StateSent       = "sent"       // the provider took it
	StateDeferred   = "deferred"   // the receiving server said "not now"
	StateDelivered  = "delivered"  // the receiving server took it
	StateBounced    = "bounced"    // the receiving server refused it
	StateComplained = "complained" // its recipient marked it as spam
	StateFailed     = "failed"     // it was not sent
	StateUnknown    = "unknown"    // the provider's answer was lost
)

// Message is what became of an email.
type Message struct {
	ID        string    `json:"id"`
	From      string    `json:"from"`
	To        string    `json:"to"`
	State     string    `json:"state"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	// Events are what happened to it after it was sent, oldest first.
	Events []MessageEvent `json:"events"`
	// Content is what the email said: nil unless the server keeps content,
	// it is still within its time, and the key is an admin key.
	Content *Content `json:"content,omitempty"`
}

// MessageEvent is one thing that happened to an email.
type MessageEvent struct {
	// Type is "delivered", "deferred", "bounced", "complained" or "failed".
	Type string    `json:"type"`
	At   time.Time `json:"at"`
	// Permanent says, for a bounce, that the address will never take mail.
	Permanent bool `json:"permanent,omitempty"`
	// Code is the receiving server's status ("5.1.1"); Reason the mail
	// provider's short name for what happened.
	Code   string `json:"code,omitempty"`
	Reason string `json:"reason,omitempty"`
	// Detail is the receiving server's own sentence, under the same rule
	// as Content.
	Detail string `json:"detail,omitempty"`
}

// Content is what an email said, where the server is set to keep that.
type Content struct {
	FromName  string            `json:"from_name,omitempty"`
	ReplyTo   string            `json:"reply_to,omitempty"`
	Subject   string            `json:"subject"`
	HTML      string            `json:"html,omitempty"`
	Text      string            `json:"text,omitempty"`
	Headers   map[string]string `json:"headers,omitempty"`
	KeptUntil time.Time         `json:"kept_until"`
}

// Get answers what became of an email. A send key sees the emails it
// sent; an admin key sees any. IsNotFound says there is none: the id is
// wrong, another key's, or its record was deleted after the server's
// retention.
func (c *Client) Get(ctx context.Context, id string) (*Message, error) {
	var m Message
	if err := c.do(ctx, call{method: http.MethodGet, path: []string{"emails", id}}, &m); err != nil {
		return nil, err
	}
	if m.ID == "" {
		return nil, &TransportError{cause: errNotCairnPost}
	}
	return &m, nil
}
