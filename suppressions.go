package cairnpost

import (
	"context"
	"iter"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// Why an address is suppressed.
const (
	ReasonBounce    = "bounce"    // a receiving server refused it for good
	ReasonComplaint = "complaint" // its owner marked an email as spam
	ReasonManual    = "manual"    // put there by hand
)

// Suppression is an address CairnPost will not send to from a domain.
type Suppression struct {
	// Domain is the sending domain it holds for.
	Domain  string `json:"domain"`
	Address string `json:"address"`
	// Reason is one of the Reason constants.
	Reason string `json:"reason"`
	// EmailID is the email whose bounce or complaint caused it; "" when
	// there is none, or its record has been deleted.
	EmailID   string    `json:"email_id,omitempty"`
	Note      string    `json:"note,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

// SuppressionQuery narrows and pages a list of suppressions.
type SuppressionQuery struct {
	// Domain narrows to one sending domain; "" for all.
	Domain string
	// Limit is the page's size; 0 for the server's own (100).
	Limit int
	// After is the Next of the page before; "" for the first page.
	After string
}

// SuppressionPage is one page of suppressions.
type SuppressionPage struct {
	Data []Suppression `json:"data"`
	// Next is what to pass as After for the page that follows; "" when
	// this is the last.
	Next string `json:"next"`
}

// Suppressions answers one page of the addresses not sent to, in order of
// domain and address. Admin key.
func (c *Client) Suppressions(ctx context.Context, q SuppressionQuery) (*SuppressionPage, error) {
	var page SuppressionPage
	query := paging(q.Limit, q.After)
	if q.Domain != "" {
		query.Set("domain", q.Domain)
	}
	if err := c.do(ctx, call{method: http.MethodGet, path: []string{"suppressions"}, query: query}, &page); err != nil {
		return nil, err
	}
	return &page, nil
}

// AllSuppressions walks every suppression of a sending domain ("" for
// all), a page at a time. It stops at the first error, which it yields.
// Admin key.
func (c *Client) AllSuppressions(ctx context.Context, domain string) iter.Seq2[Suppression, error] {
	return walk(func(after string) ([]Suppression, string, error) {
		// The largest page the server gives: fewer calls for a long list.
		page, err := c.Suppressions(ctx, SuppressionQuery{Domain: domain, Limit: 1000, After: after})
		if err != nil {
			return nil, "", err
		}
		return page.Data, page.Next, nil
	})
}

// Suppress stops sending to an address from a sending domain. The note is
// optional. It answers false when the address was suppressed already, in
// which case nothing changed. Admin key.
func (c *Client) Suppress(ctx context.Context, domain, address, note string) (added bool, err error) {
	body := struct {
		Domain  string `json:"domain"`
		Address string `json:"address"`
		Note    string `json:"note,omitempty"`
	}{domain, address, note}
	var out struct {
		Added bool `json:"added"`
	}
	err = c.do(ctx, call{method: http.MethodPost, path: []string{"suppressions"}, body: body}, &out)
	return out.Added, err
}

// Lift lets an address be sent to again from a sending domain. IsNotFound
// says it was not suppressed. Admin key.
func (c *Client) Lift(ctx context.Context, domain, address string) error {
	body := struct {
		Domain  string `json:"domain"`
		Address string `json:"address"`
	}{domain, address}
	return c.do(ctx, call{method: http.MethodPost, path: []string{"suppressions", "lift"}, body: body}, nil)
}

// LiftAll lifts every suppression of a sending domain, or those of one
// reason (a Reason constant; "" for all), and answers how many. Admin key.
func (c *Client) LiftAll(ctx context.Context, domain, reason string) (int, error) {
	body := struct {
		Domain string `json:"domain"`
		All    bool   `json:"all"`
		Reason string `json:"reason,omitempty"`
	}{domain, true, reason}
	var out struct {
		Lifted int `json:"lifted"`
	}
	err := c.do(ctx, call{method: http.MethodPost, path: []string{"suppressions", "lift"}, body: body}, &out)
	return out.Lifted, err
}

// paging is the query of a paged list.
func paging(limit int, after string) url.Values {
	q := url.Values{}
	if limit > 0 {
		q.Set("limit", strconv.Itoa(limit))
	}
	if after != "" {
		q.Set("after", after)
	}
	return q
}
