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
	var out cancelledAnswer
	if err := c.do(ctx, call{method: http.MethodPost, path: []string{"emails", id, "cancel"}}, &out); err != nil {
		return err
	}
	return about(out.ID == id)
}

// cancelledAnswer is the answer to a cancel: the email, and that it is
// cancelled.
type cancelledAnswer struct {
	ID    string `json:"id"`
	State string `json:"state"`
}

func (a *cancelledAnswer) whole() bool { return a.ID != "" && a.State == StateCancelled }

// movedAnswer is the answer to a move: the email, and its new time.
type movedAnswer struct {
	ID          string    `json:"id"`
	ScheduledAt time.Time `json:"scheduled_at"`
}

func (a *movedAnswer) whole() bool { return a.ID != "" && !a.ScheduledAt.IsZero() }

// about says whether a whole answer is the answer to what was asked.
// Only CairnPost's own word that it changed this email counts: an
// answer about anything else is not one.
func about(asked bool) error {
	if !asked {
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
	to := at.UTC().Round(0)
	body := struct {
		ScheduledAt time.Time `json:"scheduled_at"`
	}{to}
	var out movedAnswer
	if err := c.do(ctx, call{method: http.MethodPatch, path: []string{"emails", id}, body: body}, &out); err != nil {
		return err
	}
	// The email's own record is at this path, and says the time it had:
	// only the time asked for says it was moved.
	return about(out.ID == id && out.ScheduledAt.Equal(to))
}
