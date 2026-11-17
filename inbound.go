package cairnpost

import (
	"context"
	"errors"
	"iter"
	"net/http"
	"strconv"
	"time"
)

// Received is an email that came for an address of a sending domain and
// is kept by the server for a time.
type Received struct {
	ID string `json:"id"`
	// To is the address it came for; From who it came from, as the
	// sending server said: "" for a mail server's own notice, such as a
	// bounce. Neither is what the email says of itself: see Content.
	To         string    `json:"to"`
	From       string    `json:"from,omitempty"`
	ReceivedAt time.Time `json:"received_at"`
	// KeptUntil is when the server stops showing it, and deletes it
	// soon after.
	KeptUntil time.Time `json:"kept_until"`
	// Size is how long it is as it came, in bytes.
	Size int `json:"size"`
	// Content is the email as it reads; nil in a list.
	Content *ReceivedContent `json:"content,omitempty"`
}

// ReceivedHeader is one header of a received email.
type ReceivedHeader struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// ReceivedContent is what a received email says. A stranger wrote it:
// treat every part as untrusted, and never show HTML as a page.
type ReceivedContent struct {
	Subject   string           `json:"subject"`
	From      string           `json:"from"`
	To        []string         `json:"to,omitempty"`
	Cc        []string         `json:"cc,omitempty"`
	ReplyTo   string           `json:"reply_to,omitempty"`
	MessageID string           `json:"message_id,omitempty"`
	Date      time.Time        `json:"date"`
	Text      string           `json:"text,omitempty"`
	HTML      string           `json:"html,omitempty"`
	Headers   []ReceivedHeader `json:"headers,omitempty"`
	// Attachments are its files in the order they came: a file's place,
	// for ReceivedFile, counts from 1.
	Attachments []File `json:"attachments,omitempty"`
	// Partial says some of it could not be read as it was written (a
	// charset the server does not know, a broken part): what is here
	// may be incomplete. ReceivedRaw has all of it.
	Partial bool `json:"partial,omitempty"`
}

// ReceivedQuery asks for a page of received emails.
type ReceivedQuery struct {
	// Limit is the page's size; 0 for the server's own (50). 200 at most.
	Limit int
	// After is the Next of the page before; "" for the first page.
	After string
}

// ReceivedPage is one page of received emails, newest first, each
// without its Content.
type ReceivedPage struct {
	Data []Received `json:"data"`
	// Next is what to pass as After for the page that follows; "" when
	// this is the last.
	Next string `json:"next"`
}

// ReceivedList answers a page of the emails that came and are kept:
// all, or its sending domain's for a key bound to one. Admin key.
func (c *Client) ReceivedList(ctx context.Context, q ReceivedQuery) (*ReceivedPage, error) {
	var page ReceivedPage
	if err := c.do(ctx, call{method: http.MethodGet, path: []string{"inbound"}, query: paging(q.Limit, q.After)}, &page); err != nil {
		return nil, err
	}
	return &page, nil
}

// AllReceived walks every email that came and is kept, newest first, a
// page at a time. It stops at the first error, which it yields. Admin
// key.
func (c *Client) AllReceived(ctx context.Context) iter.Seq2[Received, error] {
	return walk(func(after string) ([]Received, string, error) {
		page, err := c.ReceivedList(ctx, ReceivedQuery{Limit: largestPage, After: after})
		if err != nil {
			return nil, "", err
		}
		return page.Data, page.Next, nil
	})
}

// Received answers one email that came, with what it says. IsNotFound
// says there is none: the id is wrong, it came for another sending
// domain, or its time is up. Admin key.
func (c *Client) Received(ctx context.Context, id string) (*Received, error) {
	var r Received
	if err := c.do(ctx, call{method: http.MethodGet, path: []string{"inbound", id}}, &r); err != nil {
		return nil, err
	}
	return &r, nil
}

// ReceivedRaw answers a received email as it came, every byte: what a
// mail program opens. Admin key.
func (c *Client) ReceivedRaw(ctx context.Context, id string) ([]byte, error) {
	var raw content
	if err := c.do(ctx, call{method: http.MethodGet, path: []string{"inbound", id, "raw"}, file: true}, &raw); err != nil {
		return nil, err
	}
	return raw, nil
}

// ReceivedFile reads what one of a received email's files holds. place
// counts its files from 1, in the order of ReceivedContent.Attachments.
// Admin key.
func (c *Client) ReceivedFile(ctx context.Context, id string, place int) ([]byte, error) {
	if place < 1 {
		return nil, errors.New("cairnpost: a file's place counts from 1")
	}
	var held content
	k := call{method: http.MethodGet, path: []string{"inbound", id, "attachments", strconv.Itoa(place)}, file: true}
	if err := c.do(ctx, k, &held); err != nil {
		return nil, err
	}
	return held, nil
}

// DeleteReceived deletes a received email before its time. Admin key.
func (c *Client) DeleteReceived(ctx context.Context, id string) error {
	return c.do(ctx, call{method: http.MethodDelete, path: []string{"inbound", id}}, nil)
}
