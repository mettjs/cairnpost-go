package cairnpost

import (
	"errors"
	"io"
	"net/http"
	"testing"
)

func TestAKeptFileIsReadBack(t *testing.T) {
	s, c := serving(t, func(w http.ResponseWriter, _ int) {
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Write([]byte{0, 1, 2, 0xff})
	})
	got, err := c.FileContent(ctx, "a/b", 2)
	if err != nil || string(got) != "\x00\x01\x02\xff" || s.seen[0].method != "GET" || s.seen[0].rawPath != "/emails/a%2Fb/attachments/2" {
		t.Fatalf("%q %v %+v", got, err, s.seen[0])
	}
	// A place before the first, or no id, is not asked of the server.
	if _, err := c.FileContent(ctx, "msg-1", 0); err == nil || len(s.seen) != 1 {
		t.Fatalf("place 0: %v", err)
	}
	if _, err := c.FileContent(ctx, "", 1); err == nil || len(s.seen) != 1 {
		t.Fatalf("no id: %v", err)
	}
}

func TestAFileThatIsNotKeptIsNotFound(t *testing.T) {
	_, c := serve(t, 404, `{"statusCode":404,"name":"not_found","message":"There is no such file."}`)
	if _, err := c.FileContent(ctx, "msg-1", 1); !IsNotFound(err) {
		t.Fatalf("%v", err)
	}
	// A 200 that is not a file is not a CairnPost server's answer.
	_, c = serving(t, func(w http.ResponseWriter, _ int) { io.WriteString(w, "<html>") })
	var te *TransportError
	if _, err := c.FileContent(ctx, "msg-1", 1); !errors.As(err, &te) {
		t.Fatalf("a page was taken for a file: %v", err)
	}
}

func TestContentSaysWhichFilesAreKept(t *testing.T) {
	_, c := serve(t, 200, `{"id":"msg-1","state":"sent","created_at":"2030-01-01T00:00:00Z","updated_at":"2030-01-01T00:00:00Z","content":{"subject":"s","kept_until":"2030-01-01T00:00:00Z","attachments":[
		{"filename":"a.pdf","content_type":"application/pdf","size":3,"kept":true},
		{"filename":"b.pdf","content_type":"application/pdf","size":4}]}}`)
	m, err := c.Get(ctx, "msg-1")
	if err != nil || len(m.Content.Attachments) != 2 || !m.Content.Attachments[0].Kept || m.Content.Attachments[1].Kept {
		t.Fatalf("%+v %v", m, err)
	}
}
