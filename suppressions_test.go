package cairnpost

import (
	"fmt"
	"io"
	"net/http"
	"testing"
)

func TestSuppressionsArePagedAndWalked(t *testing.T) {
	s, c := serving(t, func(w http.ResponseWriter, n int) {
		switch n {
		case 1:
			io.WriteString(w, `{"data":[{"domain":"example.com","address":"a@example.org","reason":"bounce","email_id":"m1","created_at":"2030-01-01T00:00:00Z"}],"next":"cursor 1"}`)
		default:
			io.WriteString(w, `{"data":[{"domain":"example.com","address":"b@example.org","reason":"manual","note":"asked","created_at":"2030-01-01T00:00:00Z"}]}`)
		}
	})
	var got []string
	for sup, err := range c.AllSuppressions(ctx, "example.com") {
		if err != nil {
			t.Fatal(err)
		}
		got = append(got, sup.Address+":"+sup.Reason)
	}
	if fmt.Sprint(got) != "[a@example.org:bounce b@example.org:manual]" || len(s.seen) != 2 ||
		s.seen[0].query != "domain=example.com&limit=200" || s.seen[1].query != "after=cursor+1&domain=example.com&limit=200" {
		t.Fatalf("%v %+v", got, s.seen)
	}
	page, err := c.Suppressions(ctx, SuppressionQuery{Limit: 5})
	if err != nil || len(page.Data) != 1 || page.Next != "" || s.seen[2].query != "limit=5" {
		t.Fatalf("%+v %v %q", page, err, s.seen[2].query)
	}
	// A walk stops at the first error and says it.
	_, c = serve(t, 403, `{"name":"admin_key_required","message":"Admin."}`)
	n := 0
	for _, err := range c.AllSuppressions(ctx, "") {
		if n++; err == nil {
			t.Fatal("no error")
		}
	}
	if n != 1 {
		t.Fatalf("yielded %d times", n)
	}
}

func TestSuppressAndLift(t *testing.T) {
	s, c := serving(t, func(w http.ResponseWriter, n int) {
		switch n {
		case 1:
			w.WriteHeader(201)
			io.WriteString(w, `{"domain":"example.com","address":"a@example.org","added":true}`)
		case 2:
			io.WriteString(w, `{"domain":"example.com","address":"a@example.org","added":false}`)
		case 3:
			io.WriteString(w, `{"lifted":1}`)
		default:
			io.WriteString(w, `{"lifted":3}`)
		}
	})
	if added, err := c.Suppress(ctx, "example.com", "a@example.org", "asked"); err != nil || !added {
		t.Fatalf("%v %v", added, err)
	}
	if added, err := c.Suppress(ctx, "example.com", "a@example.org", ""); err != nil || added {
		t.Fatalf("already there: %v %v", added, err)
	}
	if err := c.Lift(ctx, "example.com", "a@example.org"); err != nil {
		t.Fatal(err)
	}
	if n, err := c.LiftAll(ctx, "example.com", ReasonBounce); err != nil || n != 3 {
		t.Fatalf("%d %v", n, err)
	}
	want := []string{
		`{"domain":"example.com","address":"a@example.org","note":"asked"}`,
		`{"domain":"example.com","address":"a@example.org"}`,
		`{"domain":"example.com","address":"a@example.org"}`,
		`{"domain":"example.com","all":true,"reason":"bounce"}`,
	}
	for i, body := range want {
		if got := s.seen[i].body; got != body+"\n" {
			t.Errorf("call %d sent %s", i, got)
		}
	}
	if s.seen[2].path != "/suppressions/lift" || s.seen[3].path != "/suppressions/lift" {
		t.Fatalf("%+v", s.seen)
	}
}

// A cursor that does not move ends the walk, not the program.
func TestAWalkStopsAtACursorThatDoesNotMove(t *testing.T) {
	s, c := serve(t, 200, `{"data":[{"domain":"example.com","address":"a@example.org","reason":"manual","created_at":"2030-01-01T00:00:00Z"}],"next":"same"}`)
	n := 0
	for _, err := range c.AllSuppressions(ctx, "") {
		if n++; err != nil || n > 5 {
			t.Fatalf("%d %v", n, err)
		}
	}
	if n != 2 || len(s.seen) != 2 {
		t.Fatalf("%d items over %d calls", n, len(s.seen))
	}
}
