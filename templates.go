package cairnpost

import (
	"context"
	"net/http"
	"time"
)

// UseTemplate names a template to make an email from, and gives its
// variables their values. A variable the template has a default for may
// be left out; one it has none for must be given, and a value for a
// variable it does not name is refused. A value is written as it is in
// the subject and the text, and escaped in the HTML.
type UseTemplate struct {
	Name      string            `json:"name"`
	Variables map[string]string `json:"variables,omitempty"`
}

// TemplateVariable is a variable a template names.
type TemplateVariable struct {
	Name string `json:"name"`
	// Default is what it is filled with when a send gives it no value;
	// nil for one every send must give, and then Required is true.
	Default  *string `json:"default,omitempty"`
	Required bool    `json:"required"`
}

// Template is an email written once to be sent many times: a subject and
// bodies that name variables as {{name}}.
type Template struct {
	ID string `json:"id"`
	// Name is how a send asks for it.
	Name string `json:"name"`
	// Domain is the sending domain it belongs to; "" for one that serves
	// every domain.
	Domain  string `json:"domain,omitempty"`
	Subject string `json:"subject"`
	// HTML and Text are empty in a list: Template reads them.
	HTML      string             `json:"html,omitempty"`
	Text      string             `json:"text,omitempty"`
	Variables []TemplateVariable `json:"variables"`
	CreatedAt time.Time          `json:"created_at"`
	UpdatedAt time.Time          `json:"updated_at"`
}

// NewTemplate is a template to keep. Name, Subject, and HTML or Text are
// required. Domain is the sending domain it is for; empty, it is the
// key's own domain, or serves every domain when the key is bound to none.
// Defaults are what a variable is filled with when a send gives it no
// value.
type NewTemplate struct {
	Name     string            `json:"name"`
	Domain   string            `json:"domain,omitempty"`
	Subject  string            `json:"subject"`
	HTML     string            `json:"html,omitempty"`
	Text     string            `json:"text,omitempty"`
	Defaults map[string]string `json:"defaults,omitempty"`
}

// CreateTemplate keeps a template. An *Error named ErrNameTaken says its
// sending domain already has one of that name. Admin key.
func (c *Client) CreateTemplate(ctx context.Context, n NewTemplate) (*Template, error) {
	return c.template(ctx, call{method: http.MethodPost, path: []string{"templates"}, body: n})
}

// template makes a call that answers one template.
func (c *Client) template(ctx context.Context, k call) (*Template, error) {
	var t Template
	if err := c.do(ctx, k, &t); err != nil {
		return nil, err
	}
	if t.ID == "" {
		return nil, &TransportError{cause: errNotCairnPost}
	}
	return &t, nil
}

// Templates answers the templates the key manages, each without its
// bodies: every one, or its sending domain's if it is bound to one.
// Admin key.
func (c *Client) Templates(ctx context.Context) ([]Template, error) {
	var out struct {
		Data []Template `json:"data"`
	}
	err := c.do(ctx, call{method: http.MethodGet, path: []string{"templates"}}, &out)
	return out.Data, err
}

// Template answers one template whole. IsNotFound says there is none,
// or that it is another sending domain's. Admin key.
func (c *Client) Template(ctx context.Context, id string) (*Template, error) {
	return c.template(ctx, call{method: http.MethodGet, path: []string{"templates", id}})
}

// TemplateChange is what to change about a template. A nil field is left
// as it is; a field that is set is changed to exactly that, so Defaults
// pointing at an empty map leaves no variable a default.
type TemplateChange struct {
	Name     *string            `json:"name,omitempty"`
	Subject  *string            `json:"subject,omitempty"`
	HTML     *string            `json:"html,omitempty"`
	Text     *string            `json:"text,omitempty"`
	Defaults *map[string]string `json:"defaults,omitempty"`
}

// UpdateTemplate changes a template and answers it as it now is. Emails
// asked for from then on are made from it; one that already waits to be
// sent keeps what it was filled with. Admin key.
func (c *Client) UpdateTemplate(ctx context.Context, id string, change TemplateChange) (*Template, error) {
	if change.Defaults != nil && *change.Defaults == nil {
		// A nil map is written as null, which the server reads as "leave
		// it": an empty one says what was meant.
		change.Defaults = &map[string]string{}
	}
	return c.template(ctx, call{method: http.MethodPatch, path: []string{"templates", id}, body: change})
}

// DeleteTemplate deletes a template: a send that names it is refused
// from then on. Admin key.
func (c *Client) DeleteTemplate(ctx context.Context, id string) error {
	return c.do(ctx, call{method: http.MethodDelete, path: []string{"templates", id}}, nil)
}
