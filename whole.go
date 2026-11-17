package cairnpost

// answer is what a call reads a 2xx into. whole says it holds what
// CairnPost always says there. Something else at the address, or in
// between, can answer 200 to anything, and encoding/json reads a missing
// field, a null and "{}" all as the zero value: an answer that is not
// whole is not CairnPost's, and must not read as "no", "none" or "done".
//
// Every call's answer has to say what makes it whole: a type without the
// method cannot be read into.
type answer interface{ whole() bool }

// content is the bytes of a file, which are whatever they are.
type content []byte

func (*content) whole() bool { return true }

// listed says a list was said (it is neither missing nor null, which an
// empty one is not) and that each of its items is whole.
func listed[T any, P interface {
	*T
	answer
}](items []T) bool {
	if items == nil {
		return false
	}
	for i := range items {
		if !P(&items[i]).whole() {
			return false
		}
	}
	return true
}

// A record says which it is and when it was made: one that does not was
// not written by CairnPost, alone or in a list.

func (m *Message) whole() bool {
	if m.ID == "" || m.CreatedAt.IsZero() || m.UpdatedAt.IsZero() {
		return false
	}
	for _, r := range m.Recipients {
		if r.UpdatedAt.IsZero() {
			return false
		}
	}
	for _, e := range m.Events {
		if e.At.IsZero() {
			return false
		}
	}
	return m.Content == nil || !m.Content.KeptUntil.IsZero()
}

func (w *Webhook) whole() bool       { return w.ID != "" && !w.CreatedAt.IsZero() }
func (d *Delivery) whole() bool      { return d.ID != "" && !d.CreatedAt.IsZero() }
func (e *CapturedEmail) whole() bool { return e.ID != "" && !e.At.IsZero() }
func (t *Template) whole() bool {
	return t.ID != "" && !t.CreatedAt.IsZero() && !t.UpdatedAt.IsZero()
}
func (r *Received) whole() bool {
	return r.ID != "" && !r.ReceivedAt.IsZero() && !r.KeptUntil.IsZero()
}
func (s *Suppression) whole() bool {
	return s.Domain != "" && s.Address != "" && !s.CreatedAt.IsZero()
}

func (p *SuppressionPage) whole() bool { return listed(p.Data) }
func (p *DeliveryPage) whole() bool    { return listed(p.Data) }
func (p *ReceivedPage) whole() bool    { return listed(p.Data) }
func (l *TemplateList) whole() bool    { return listed(l.Data) }

// A server says what it sends with and which version it is; anything
// else that answers "ok" at /health does not.
func (h *Health) whole() bool { return h.Status != "" && h.Driver != "" && h.Version != "" }
func (l *Limits) whole() bool { return l.Driver != "" }
