package cairnpost

import (
	"context"
	"iter"
	"net/http"
	"time"
)

// States of a webhook delivery.
const (
	DeliveryPending   = "pending"   // owed; it will be tried, or tried again
	DeliveryDelivered = "delivered" // the endpoint took it
	DeliveryFailed    = "failed"    // every try was used up
)

// Delivery is one webhook owed to one endpoint, and how it went.
type Delivery struct {
	// ID is also the webhook's id as its receiver sees it (Event.ID).
	ID string `json:"id"`
	// EmailID is the email the webhook is about; "" for one about no
	// email (EventAddressSuppressed).
	EmailID string `json:"email_id"`
	// InboundID is the received email the webhook is about, for an
	// EventReceived.
	InboundID string `json:"inbound_id"`
	// Type is the webhook's type (the Event constants).
	Type string `json:"type"`
	// State is one of the Delivery constants.
	State    string `json:"state"`
	Attempts int    `json:"attempts"`
	// LastStatus is the endpoint's status at the last try, 0 for none;
	// LastFailure why that try failed, "" if it did not.
	LastStatus    int        `json:"last_status,omitempty"`
	LastFailure   string     `json:"last_failure,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
	AttemptedAt   *time.Time `json:"attempted_at,omitempty"`
	DeliveredAt   *time.Time `json:"delivered_at,omitempty"`
	NextAttemptAt *time.Time `json:"next_attempt_at,omitempty"`
}

// DeliveryQuery narrows and pages an endpoint's deliveries.
type DeliveryQuery struct {
	// State narrows to one state (a Delivery constant); "" for all.
	State string
	// Limit is the page's size; 0 for the server's own (50). 200 at most.
	Limit int
	// After is the Next of the page before; "" for the first page.
	After string
}

// DeliveryPage is one page of deliveries.
type DeliveryPage struct {
	Data []Delivery `json:"data"`
	// Next is what to pass as After for the page that follows; "" when
	// this is the last.
	Next string `json:"next"`
}

// Deliveries answers one page of what an endpoint was sent, newest first.
// Admin key.
func (c *Client) Deliveries(ctx context.Context, webhookID string, q DeliveryQuery) (*DeliveryPage, error) {
	var page DeliveryPage
	query := paging(q.Limit, q.After)
	if q.State != "" {
		query.Set("state", q.State)
	}
	k := call{method: http.MethodGet, path: []string{"webhooks", webhookID, "deliveries"}, query: query}
	if err := c.do(ctx, k, &page); err != nil {
		return nil, err
	}
	return &page, nil
}

// AllDeliveries walks an endpoint's deliveries of one state ("" for all),
// newest first, a page at a time. It stops at the first error, which it
// yields. Admin key.
func (c *Client) AllDeliveries(ctx context.Context, webhookID, state string) iter.Seq2[Delivery, error] {
	return walk(func(after string) ([]Delivery, string, error) {
		page, err := c.Deliveries(ctx, webhookID, DeliveryQuery{State: state, Limit: largestPage, After: after})
		if err != nil {
			return nil, "", err
		}
		return page.Data, page.Next, nil
	})
}

// RetryDelivery has a webhook tried now: once more if it was given up, or
// without waiting for its next try if it is pending. Admin key.
func (c *Client) RetryDelivery(ctx context.Context, webhookID, deliveryID string) error {
	var out retriedAnswer
	return c.do(ctx, call{method: http.MethodPost, path: []string{"webhooks", webhookID, "deliveries", deliveryID, "retry"}}, &out)
}

// retriedAnswer is the state of a webhook that will be tried now. Only
// CairnPost's own word that it will counts.
type retriedAnswer struct {
	State string `json:"state"`
}

func (r *retriedAnswer) whole() bool {
	return r.State == DeliveryPending
}
