package cairnpost

import (
	"errors"
	"testing"
)

func TestAnEmailIsSentFromATemplate(t *testing.T) {
	s, c := serve(t, 200, `{"id":"msg-1"}`)
	_, err := c.Send(ctx, Email{From: "app@example.com", To: []string{"ana@example.org"},
		Template: &UseTemplate{Name: "welcome", Variables: map[string]string{"code": "4821"}}})
	if err != nil || s.seen[0].body != `{"from":"app@example.com","to":["ana@example.org"],"subject":"","template":{"name":"welcome","variables":{"code":"4821"}}}`+"\n" {
		t.Fatalf("%v %s", err, s.seen[0].body)
	}
	// And each email of a batch.
	s, c = serve(t, 200, `{"data":[{"id":"msg-1"}]}`)
	if _, err := c.SendBatch(ctx, Batch{Emails: []Email{{From: "app@example.com", To: []string{"ana@example.org"}, Template: &UseTemplate{Name: "welcome"}}}}); err != nil ||
		s.seen[0].body != `[{"from":"app@example.com","to":["ana@example.org"],"subject":"","template":{"name":"welcome"}}]`+"\n" {
		t.Fatalf("%v %s", err, s.seen[0].body)
	}
}

func TestTemplatesAreKeptReadChangedAndDeleted(t *testing.T) {
	one := `{"id":"t-1","name":"welcome","subject":"Hello {{name}}","html":"<p>{{code}}</p>",
		"variables":[{"name":"name","default":"there","required":false},{"name":"code","required":true}],
		"created_at":"2030-01-01T00:00:00Z","updated_at":"2030-01-02T00:00:00Z"}`
	s, c := serve(t, 201, one)
	made, err := c.CreateTemplate(ctx, NewTemplate{Name: "welcome", Subject: "Hello {{name}}", HTML: "<p>{{code}}</p>", Defaults: map[string]string{"name": "there"}})
	if err != nil || made.ID != "t-1" || len(made.Variables) != 2 || *made.Variables[0].Default != "there" || made.Variables[1].Default != nil ||
		!made.Variables[1].Required || made.UpdatedAt.Day() != 2 {
		t.Fatalf("%+v %v", made, err)
	}
	if s.seen[0].method != "POST" || s.seen[0].path != "/templates" ||
		s.seen[0].body != `{"name":"welcome","subject":"Hello {{name}}","html":"<p>{{code}}</p>","defaults":{"name":"there"}}`+"\n" {
		t.Fatalf("%+v", s.seen[0])
	}
	s, c = serve(t, 200, one)
	if got, err := c.Template(ctx, "a/b"); err != nil || got.Name != "welcome" || s.seen[0].rawPath != "/templates/a%2Fb" {
		t.Fatalf("%+v %v", got, err)
	}
	subject := "Hi"
	var none map[string]string
	if _, err := c.UpdateTemplate(ctx, "t-1", TemplateChange{Subject: &subject, Defaults: &none}); err != nil || s.seen[1].method != "PATCH" ||
		s.seen[1].body != `{"subject":"Hi","defaults":{}}`+"\n" {
		t.Fatalf("%v %+v", err, s.seen[1])
	}
	if _, err := c.Template(ctx, ""); err == nil || len(s.seen) != 2 {
		t.Fatalf("an empty id was asked of the server: %v", err)
	}
	s, c = serve(t, 200, `{"data":[`+one+`],"more":true}`)
	if list, err := c.Templates(ctx, ""); err != nil || len(list.Data) != 1 || list.Data[0].ID != "t-1" || !list.More || s.seen[0].query != "" {
		t.Fatalf("%+v %v", list, err)
	}
	if _, err := c.Templates(ctx, "shop.example.com"); err != nil || s.seen[1].query != "domain=shop.example.com" {
		t.Fatalf("%v %+v", err, s.seen[1])
	}
	s, c = serve(t, 204, "")
	if err := c.DeleteTemplate(ctx, "t-1"); err != nil || s.seen[0].method != "DELETE" || s.seen[0].path != "/templates/t-1" {
		t.Fatalf("%v %+v", err, s.seen[0])
	}
	_, c = serve(t, 409, `{"statusCode":409,"name":"name_taken","message":"A template of this name is already there."}`)
	var e *Error
	if _, err := c.CreateTemplate(ctx, NewTemplate{Name: "welcome", Subject: "s", Text: "t"}); !errors.As(err, &e) || e.Name != ErrNameTaken {
		t.Fatalf("%v", err)
	}
	// An answer that is no template is not trusted.
	_, c = serve(t, 200, `{}`)
	if _, err := c.Template(ctx, "t-1"); err == nil {
		t.Fatal("an empty answer was taken for a template")
	}
}
