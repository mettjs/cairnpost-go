package cairnpost

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

// An error says what to do: whether to try again, whether the email may
// have gone, whether a record is missing or the address is wrong. Only
// under a name of CairnPost's is anything but the status believed: what
// any other answer says is not repeated.
func TestAnErrorSaysWhatToDo(t *testing.T) {
	var file struct {
		Cases []struct {
			Name   string `json:"name"`
			Call   string `json:"call"`
			Answer said   `json:"answer"`
			Want   struct {
				Status       int     `json:"status"`
				Code         *string `json:"code"`
				Detail       *string `json:"detail"`
				EmailID      *string `json:"email_id"`
				RetryAfter   *int    `json:"retry_after"`
				Retryable    bool    `json:"retryable"`
				MayHaveSent  bool    `json:"may_have_sent"`
				NotFound     bool    `json:"not_found"`
				WrongAddress bool    `json:"wrong_address"`
			} `json:"want"`
		} `json:"cases"`
	}
	cases(t, "errors.json", &file)
	if len(file.Cases) == 0 {
		t.Fatal("errors.json holds no case")
	}
	or := func(s *string) string {
		if s == nil {
			return ""
		}
		return *s
	}
	for _, c := range file.Cases {
		_, client := saying(t, c.Answer)
		err := ask(t, c.Call)(ctx, client)
		var problem *Error
		if !errors.As(err, &problem) {
			t.Errorf("%s: %v", c.Name, err)
			continue
		}
		var wait time.Duration
		if c.Want.RetryAfter != nil {
			wait = time.Duration(*c.Want.RetryAfter) * time.Second
		}
		w := c.Want
		if problem.Status != w.Status || problem.Name != or(w.Code) || problem.Message != or(w.Detail) ||
			problem.EmailID != or(w.EmailID) || problem.RetryAfter != wait || problem.Retryable() != w.Retryable ||
			MayHaveSent(err) != w.MayHaveSent || IsNotFound(err) != w.NotFound || IsWrongAddress(err) != w.WrongAddress {
			t.Errorf("%s: %+v, retryable %v, may have sent %v, not found %v, wrong address %v", c.Name, problem,
				problem.Retryable(), MayHaveSent(err), IsNotFound(err), IsWrongAddress(err))
		}
		var answer struct {
			Message string `json:"message"`
		}
		_ = json.Unmarshal(c.Answer.JSON, &answer)
		if w.Code == nil && answer.Message != "" && strings.Contains(err.Error(), answer.Message) {
			t.Errorf("%s: the error repeats what was answered: %v", c.Name, err)
		}
	}
}
