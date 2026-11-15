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
	return c.do(ctx, call{method: http.MethodPost, path: []string{"emails", id, "cancel"}}, nil)
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
	return c.do(ctx, call{method: http.MethodPatch, path: []string{"emails", id}, body: body}, nil)
}
