package cairnpost

import "time"

// Types of webhook CairnPost sends.
const (
	EventDelivered      = "email.delivered"
	EventDeliveryDelays = "email.delivery_delayed"
	EventBounced        = "email.bounced"
	EventComplained     = "email.complained"
	EventFailed         = "email.failed"
	// EventAddressSuppressed tells of an address put on the suppression
	// list because of a bounce or a complaint about an email the server
	// has no record of. It is about no email: its Email is nil and its
	// Suppression says which address. An email on record says the same
	// in its own webhook (EventEmail.Suppressed).
	EventAddressSuppressed = "address.suppressed"
	// EventReceived tells of an email that came for an address of a
	// sending domain. It is about no email that was sent: its Email is
	// nil and its Received says who from, who for, and the id to read it
	// by (Client.Received). It never says what the email says.
	EventReceived = "email.received"
	// EventTest is sent when an endpoint is tried out. It is about no
	// email: its Email is nil.
	EventTest = "webhook.test"
)

// Types of bounce, in an email.bounced webhook.
const (
	// BouncePermanent: the address will never take mail. It is the only
	// one that says the address is at fault.
	BouncePermanent = "Permanent"
	// BouncePolicy: the receiving server refused for good, for a reason
	// that is the sender's (a failed signature, a policy, a blocklist).
	// The address may be perfectly good.
	BouncePolicy = "Policy"
	// BounceTransient: refused for now.
	BounceTransient = "Transient"
)

// Event is a webhook from CairnPost, verified. Its JSON form is this
// client's own, for an application that passes one on (to a queue, say);
// it is not the form the webhook arrived in.
type Event struct {
	// ID names the webhook. It is the same on every try of it, so a
	// receiver that may be sent one twice tells by this.
	ID string `json:"id"`
	// Type is one of the Event constants; a type this client does not
	// know is passed through as it is, with an Email only if it names
	// one.
	Type string `json:"type"`
	// At is when the event happened.
	At time.Time `json:"created_at"`
	// Email is the email the event is about; nil for webhook.test,
	// address.suppressed and email.received.
	Email *EventEmail `json:"email,omitempty"`
	// Suppression is set for address.suppressed.
	Suppression *EventSuppression `json:"suppression,omitempty"`
	// Received is set for email.received.
	Received *EventReceivedEmail `json:"received,omitempty"`
}

// EventReceivedEmail is the email an email.received webhook is about:
// read what it says with Client.Received and an admin key.
type EventReceivedEmail struct {
	// ID is the received email's id.
	ID string `json:"inbound_id"`
	// From is who it came from, as the sending server said: "" for a
	// mail server's own notice, such as a bounce. To is the address it
	// came for.
	From       string    `json:"from"`
	To         []string  `json:"to"`
	ReceivedAt time.Time `json:"received_at"`
}

// EventSuppression is the address an address.suppressed webhook is
// about: the server will refuse to send to it from that sending domain
// until the suppression is lifted.
type EventSuppression struct {
	Domain  string `json:"domain"`
	Address string `json:"address"`
	// Reason is one of the Reason constants, never empty: ReasonBounce
	// (the address refused an email for good) or ReasonComplaint (its
	// owner marked one as spam).
	Reason string `json:"reason"`
}

// EventEmail is the email a webhook is about. Never its subject or body.
type EventEmail struct {
	ID        string    `json:"email_id"`
	From      string    `json:"from"`
	To        []string  `json:"to"`
	CreatedAt time.Time `json:"created_at"`
	// Code is the receiving server's status ("5.1.1"); Reason the mail
	// provider's short name for what happened.
	Code   string `json:"code,omitempty"`
	Reason string `json:"reason,omitempty"`
	// Bounce is set for email.bounced.
	Bounce *Bounce `json:"bounce,omitempty"`
	// Suppressed says that, with this event, the address is on the
	// sending domain's suppression list: CairnPost will refuse to send to
	// it from there until the suppression is lifted.
	Suppressed bool `json:"suppressed"`
}

// Bounce says what kind of bounce it was.
type Bounce struct {
	// Type is one of the Bounce constants.
	Type string `json:"type"`
}
