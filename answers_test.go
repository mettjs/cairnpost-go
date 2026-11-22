package cairnpost

import "testing"

// The answers every client must take or refuse are the shared cases
// (cases_answers_test.go). What follows is this client's own: what a
// call hands back beside its error, and what it reads out of an answer
// that says "none".

// A value never comes with an error.
func TestAValueNeverComesWithAnError(t *testing.T) {
	_, c := serve(t, 200, `{"data":[{"id":"c1"}],"added":"yes","lifted":"7"}`)
	if held, err := c.Captured(ctx); err == nil || held != nil {
		t.Errorf("%+v %v", held, err)
	}
	if added, err := c.Suppress(ctx, "example.com", "a@example.org", ""); err == nil || added {
		t.Errorf("%v %v", added, err)
	}
	if n, err := c.LiftAll(ctx, "example.com", ""); err == nil || n != 0 {
		t.Errorf("%v %v", n, err)
	}
}

// What CairnPost says of nothing is still read as nothing.
func TestAnEmptyListAndANoAreStillRead(t *testing.T) {
	_, c := serve(t, 200, `{"data":[],"added":false,"lifted":0,"delivered":false,"more":false}`)
	if page, err := c.Suppressions(ctx, SuppressionQuery{}); err != nil || len(page.Data) != 0 || page.Next != "" {
		t.Fatalf("%+v %v", page, err)
	}
	if list, err := c.Webhooks(ctx); err != nil || len(list) != 0 {
		t.Fatalf("%+v %v", list, err)
	}
	if list, err := c.Templates(ctx, ""); err != nil || len(list.Data) != 0 || list.More {
		t.Fatalf("%+v %v", list, err)
	}
	if list, err := c.Captured(ctx); err != nil || len(list) != 0 {
		t.Fatalf("%+v %v", list, err)
	}
	if added, err := c.Suppress(ctx, "example.com", "a@example.org", ""); err != nil || added {
		t.Fatalf("%v %v", added, err)
	}
	if n, err := c.LiftAll(ctx, "example.com", ""); err != nil || n != 0 {
		t.Fatalf("%v %v", n, err)
	}
	if got, err := c.TestWebhook(ctx, "w"); err != nil || got.Delivered {
		t.Fatalf("%+v %v", got, err)
	}
}
