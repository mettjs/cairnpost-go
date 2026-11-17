package cairnpost

import (
	"context"
	"errors"
	"net/http"
	"time"
)

// Cancel ends the wait of an email that was asked for with
// Email.ScheduledAt: it will not be sent, and what it said is deleted
// from the server. The key that asked for the email may, and an admin
// key that may read it.
//
// An email that no longer waits (it is being sent, was sent or was
// cancelled) answers an *Error named ErrNotScheduled.
func (c *Client) Cancel(ctx context.Context, id string) error {
	return c.moved(ctx, id, call{method: http.MethodPost, path: []string{"emails", id, "cancel"}})
}

// movedAnswer names the email a call changed.
type movedAnswer struct {
	ID string `json:"id"`
}

func (m *movedAnswer) whole() bool { return m.ID != "" }

// moved makes a call that changes an email and answers its id. Only
// CairnPost's own word that it did counts: an answer about anything
// else is not one.
func (c *Client) moved(ctx context.Context, id string, k call) error {
	var out movedAnswer
	if err := c.do(ctx, k, &out); err != nil {
		return err
	}
	if out.ID != id {
		return &TransportError{cause: errNotCairnPost}
	}
	return nil
}

// Reschedule gives an email that waits another time to be sent at. Its
// errors are Cancel's, and ErrValidation for a time it cannot be sent
// at.
func (c *Client) Reschedule(ctx context.Context, id string, at time.Time) error {
	if at.IsZero() {
		return errors.New("cairnpost: a time to send the email at is required")
	}
	body := struct {
		ScheduledAt time.Time `json:"scheduled_at"`
	}{at.UTC().Round(0)}
	return c.moved(ctx, id, call{method: http.MethodPatch, path: []string{"emails", id}, body: body})
}
