package cairnpost

import (
	"encoding/json"
	"errors"
	"testing"
)

// answerCase is an answer put in front of some calls, or of all of them.
type answerCase struct {
	Name   string          `json:"name"`
	Calls  json.RawMessage `json:"calls"`
	Answer said            `json:"answer"`
}

// calls answers the names of the calls a case is about.
func (c answerCase) calls(t *testing.T) []string {
	t.Helper()
	var names []string
	if string(c.Calls) == `"all"` {
		for _, a := range asking {
			names = append(names, a.name)
		}
	} else if err := json.Unmarshal(c.Calls, &names); err != nil || len(names) == 0 {
		t.Fatalf("%s: its calls are %s", c.Name, c.Calls)
	}
	return names
}

// Something else at the address, or between, can answer 200 to anything,
// and an answer can be of CairnPost's kind and still not be the answer to
// what was asked. None of it may read as "done", "no" or "none"; and what
// CairnPost does say is taken.
func TestAnAnswerIsTakenOnlyWhenItIsCairnPosts(t *testing.T) {
	var file struct {
		Refused []answerCase `json:"refused"`
		Taken   []answerCase `json:"taken"`
	}
	cases(t, "answers.json", &file)
	if len(file.Refused) == 0 || len(file.Taken) == 0 {
		t.Fatalf("answers.json holds %d answers to refuse and %d to take", len(file.Refused), len(file.Taken))
	}
	for _, c := range file.Refused {
		_, client := saying(t, c.Answer)
		for _, name := range c.calls(t) {
			if err := ask(t, name)(ctx, client); !errors.Is(err, errNotCairnPost) {
				t.Errorf("%s took %s: %v", name, c.Name, err)
			}
		}
	}
	for _, c := range file.Taken {
		_, client := saying(t, c.Answer)
		for _, name := range c.calls(t) {
			if err := ask(t, name)(ctx, client); err != nil {
				t.Errorf("%s refused %s: %v", name, c.Name, err)
			}
		}
	}
}
