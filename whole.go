package cairnpost

import "encoding/json"

// names says the answer is an object that says each of these fields, and
// none of them as null. An answer that lacks what CairnPost always says
// ("{}" above all) is not CairnPost's, and must not read as "no", "none"
// or "done".
func names(raw []byte, keys []string) bool {
	if len(keys) == 0 {
		return true
	}
	var said map[string]json.RawMessage
	if json.Unmarshal(raw, &said) != nil {
		return false
	}
	for _, key := range keys {
		if v, ok := said[key]; !ok || string(v) == "null" {
			return false
		}
	}
	return true
}

// each says every item of a list is whole.
func each[T any](items []T, whole func(*T) bool) bool {
	for i := range items {
		if !whole(&items[i]) {
			return false
		}
	}
	return true
}

// The times below are ones CairnPost always says. A record without one
// was not written by CairnPost.

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

func (w *Webhook) whole() bool       { return !w.CreatedAt.IsZero() }
func (d *Delivery) whole() bool      { return !d.CreatedAt.IsZero() }
func (s *Suppression) whole() bool   { return !s.CreatedAt.IsZero() }
func (t *Template) whole() bool      { return !t.CreatedAt.IsZero() && !t.UpdatedAt.IsZero() }
func (r *Received) whole() bool      { return !r.ReceivedAt.IsZero() && !r.KeptUntil.IsZero() }
func (e *CapturedEmail) whole() bool { return !e.At.IsZero() }
