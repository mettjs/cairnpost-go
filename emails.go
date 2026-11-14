package cairnpost

import (
	"context"
	"errors"
	"net/http"
	"time"
)

// Email is one email, to one address or several.
type Email struct {
	// From is the sender: "app@example.com" or "App <app@example.com>".
	From string
	// To is who the email is for: one address at least. Cc are copied in
	// the open, Bcc without the others seeing them. No address twice;
	// the server says how many an email may name in all (50 by default).
	To  []string
	Cc  []string
	Bcc []string
	// ReplyTo is optional.
	ReplyTo string
	Subject string
	// HTML and Text are the bodies; at least one is required.
	HTML string
	Text string
	// Headers are extra headers, optional.
	Headers map[string]string
	// Attachments are files to send with it, optional.
	Attachments []Attachment
	// IdempotencyKey makes a repeated send one email: the same key with
	// the same content answers the first one's id. Use one for anything
	// that may be retried. It is remembered for a day by default.
	IdempotencyKey string
}

// Attachment is a file sent with an email.
type Attachment struct {
	// Filename is the name its reader sees: a name, not a path.
	Filename string
	// Content is the file's bytes.
	Content []byte
	// ContentType is its media type ("application/pdf"); "" has the
	// server tell it from the filename.
	ContentType string
	// ContentID, when set, makes the file part of the HTML body, which
	// shows it as "cid:" and this name; "" is a file beside the email.
	ContentID string
}

// Sent is the server's answer to a send.
type Sent struct {
	// ID is the email's id: one, however many addresses it went to.
	ID string
	// Suppressed are the addresses it was not sent to, because they are
	// on the sending domain's suppression list. Empty when it went to
	// all. An email with nobody left in To or Cc is an error instead
	// (ErrRecipientSuppress): a Bcc alone is not sent.
	Suppressed []string
}

// wire is an email as the server takes it: only the fields it names,
// since it refuses any other.
type wire struct {
	From    string            `json:"from"`
	To      []string          `json:"to"`
	Cc      []string          `json:"cc,omitempty"`
	Bcc     []string          `json:"bcc,omitempty"`
	ReplyTo string            `json:"reply_to,omitempty"`
	Subject string            `json:"subject"`
	HTML    string            `json:"html,omitempty"`
	Text    string            `json:"text,omitempty"`
	Headers map[string]string `json:"headers,omitempty"`
	// encoding/json writes a file's bytes in base64, as the server
	// reads them.
	Attachments []attachment `json:"attachments,omitempty"`
}

func wireOf(e Email) wire {
	return wire{e.From, e.To, e.Cc, e.Bcc, e.ReplyTo, e.Subject, e.HTML, e.Text, e.Headers, attachments(e.Attachments)}
}

// Send sends an email and answers its id once the mail provider has taken
// it. On an error, see Error for what to do and MayHaveSent for whether
// the email may have gone; the error's EmailID, when set, can be read
// with Get.
func (c *Client) Send(ctx context.Context, e Email) (*Sent, error) {
	body := wireOf(e)
	k := call{method: http.MethodPost, path: []string{"emails"}, body: body}
	if e.IdempotencyKey != "" {
		// A key a header cannot carry as it is would be dropped or changed
		// on the way, and the send would quietly not be idempotent.
		if !headerSafe(e.IdempotencyKey) {
			return nil, errors.New("cairnpost: the IdempotencyKey must have no control characters and no space at either end; nothing was sent")
		}
		k.header = http.Header{"Idempotency-Key": {e.IdempotencyKey}}
	}
	var out struct {
		ID         string   `json:"id"`
		Suppressed []string `json:"suppressed"`
	}
	if err := c.do(ctx, k, &out); err != nil {
		return nil, err
	}
	if out.ID == "" {
		// A 2xx with no id is not CairnPost's answer: whatever is at that
		// address, the email is not known to have been sent.
		return nil, &TransportError{cause: errNotCairnPost}
	}
	return &Sent{ID: out.ID, Suppressed: out.Suppressed}, nil
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
	ID   string `json:"id"`
	From string `json:"from"`
	// To is the first address the email went to, and State what became
	// of it there. Recipients has every address with its own.
	To         string      `json:"to"`
	State      string      `json:"state"`
	Recipients []Recipient `json:"recipients"`
	// Suppressed are the addresses it was asked to go to and did not,
	// being on the suppression list when it was sent.
	Suppressed []string  `json:"suppressed,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
	// Events are what happened to it after it was sent, oldest first.
	Events []MessageEvent `json:"events"`
	// Content is what the email said: nil unless the server keeps content,
	// it is still within its time, and the key is an admin key.
	Content *Content `json:"content,omitempty"`
}

// Kinds of recipient.
const (
	KindTo  = "to"
	KindCc  = "cc"
	KindBcc = "bcc"
)

// Recipient is one address of an email and what became of it there.
type Recipient struct {
	Address string `json:"address"`
	// Kind is KindTo, KindCc or KindBcc.
	Kind      string    `json:"kind"`
	State     string    `json:"state"`
	UpdatedAt time.Time `json:"updated_at"`
}

// MessageEvent is one thing that happened to an email.
type MessageEvent struct {
	// Type is "delivered", "deferred", "bounced", "complained" or "failed".
	Type string    `json:"type"`
	At   time.Time `json:"at"`
	// To is the address it happened at.
	To string `json:"to,omitempty"`
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

// attachment is a file as the server takes it.
type attachment struct {
	Filename    string `json:"filename"`
	Content     []byte `json:"content"`
	ContentType string `json:"content_type,omitempty"`
	ContentID   string `json:"content_id,omitempty"`
}

func attachments(files []Attachment) []attachment {
	var out []attachment
	for _, f := range files {
		out = append(out, attachment(f))
	}
	return out
}

// File is what an attachment was: its bytes are kept by no server.
type File struct {
	Filename    string `json:"filename"`
	ContentType string `json:"content_type"`
	ContentID   string `json:"content_id,omitempty"`
	Size        int    `json:"size"`
}

// Content is what an email said, where the server is set to keep that.
type Content struct {
	FromName string            `json:"from_name,omitempty"`
	ReplyTo  string            `json:"reply_to,omitempty"`
	Subject  string            `json:"subject"`
	HTML     string            `json:"html,omitempty"`
	Text     string            `json:"text,omitempty"`
	Headers  map[string]string `json:"headers,omitempty"`
	// Attachments say what files went with it.
	Attachments []File    `json:"attachments,omitempty"`
	KeptUntil   time.Time `json:"kept_until"`
}

// Get answers what became of an email. A send key sees the emails it
// sent; an admin key sees any, or its sending domain's if it is bound to
// one. IsNotFound says there is none: the id is wrong, another key's, or
// its record was deleted after the server's retention.
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
