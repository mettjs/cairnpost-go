package cairnpost

import (
	"encoding/json"
	"errors"
	"net/http"
	"testing"
	"time"
)

// plain is a verified event in the form the shared cases write one: every
// field said, a time as its instant in UTC, "" for what is not there.
func plain(e *Event) map[string]any {
	stamp := func(at time.Time) string {
		if at.IsZero() {
			return ""
		}
		return at.UTC().Format("2006-01-02T15:04:05Z")
	}
	out := map[string]any{"id": e.ID, "type": e.Type, "at": stamp(e.At)}
	if m := e.Email; m != nil {
		bounce := ""
		if m.Bounce != nil {
			bounce = m.Bounce.Type
		}
		out["email"] = map[string]any{"id": m.ID, "from": m.From, "to": m.To, "created_at": stamp(m.CreatedAt),
			"code": m.Code, "reason": m.Reason, "bounce": bounce, "suppressed": m.Suppressed}
	}
	if s := e.Suppression; s != nil {
		out["suppression"] = map[string]any{"domain": s.Domain, "address": s.Address, "reason": s.Reason}
	}
	if r := e.Received; r != nil {
		out["received"] = map[string]any{"id": r.ID, "from": r.From, "to": r.To, "received_at": stamp(r.ReceivedAt)}
	}
	return out
}

// A webhook is read when the endpoint's secret signed it and it says
// what it is about, and refused otherwise, each for its own reason.
func TestAWebhookIsReadOnlyWhenItIsTheEndpoints(t *testing.T) {
	var file struct {
		Secret  string `json:"secret"`
		Now     int64  `json:"now"`
		Secrets struct {
			Taken   []string `json:"taken"`
			Refused []string `json:"refused"`
		} `json:"secrets"`
		Cases []struct {
			Name      string            `json:"name"`
			Headers   map[string]string `json:"headers"`
			Body      string            `json:"body"`
			Tolerance int               `json:"tolerance"`
			Want      struct {
				Refused string          `json:"refused"`
				Event   json.RawMessage `json:"event"`
			} `json:"want"`
		} `json:"cases"`
	}
	cases(t, "webhooks.json", &file)
	if len(file.Cases) == 0 || len(file.Secrets.Taken) == 0 || len(file.Secrets.Refused) == 0 {
		t.Fatal("webhooks.json holds no case, or no secret to take or to refuse")
	}
	for _, secret := range file.Secrets.Taken {
		if _, err := NewVerifier(secret); err != nil {
			t.Errorf("%q was refused as a secret: %v", secret, err)
		}
	}
	for _, secret := range file.Secrets.Refused {
		if _, err := NewVerifier(secret); !errors.Is(err, ErrSecret) {
			t.Errorf("%q was taken as a secret: %v", secret, err)
		}
	}
	refusals := map[string]error{"unsigned": ErrUnsigned, "signature": ErrSignature, "stale": ErrStale, "payload": ErrPayload}
	for _, c := range file.Cases {
		v, err := NewVerifier(file.Secret)
		if err != nil {
			t.Fatal(err)
		}
		v.Now = func() time.Time { return time.Unix(file.Now, 0) }
		v.Tolerance = time.Duration(c.Tolerance) * time.Second
		// Under the names as the case writes them, not as Set would: a
		// header is not always one net/http made.
		header := http.Header{}
		for name, value := range c.Headers {
			header[name] = []string{value}
		}
		e, err := v.Verify(header, []byte(c.Body))
		want, refused := refusals[c.Want.Refused]
		switch {
		case refused == (c.Want.Event != nil):
			t.Fatalf("%s: a case wants an event or one of the refusals, and not both: %+v", c.Name, c.Want)
		case refused:
			if !errors.Is(err, want) || e != nil {
				t.Errorf("%s: %+v %v, not %v", c.Name, e, err, want)
			}
		case err != nil:
			t.Errorf("%s was refused: %v", c.Name, err)
		default:
			if got, _ := json.Marshal(plain(e)); !sameJSON(t, got, c.Want.Event) {
				t.Errorf("%s was read as %s, not %s", c.Name, got, c.Want.Event)
			}
		}
	}
}
