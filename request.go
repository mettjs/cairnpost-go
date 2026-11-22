package cairnpost

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// around is what is dropped around an API key, a signing secret and the
// value of a signature header: what a file or a header puts there, and
// no more. A language's own blank space is wider, and differs from the
// next language's, and one value must read the same in every client.
const around = " \t\r\n"

// keyLike says a text could be an API key: not empty, and printable
// ASCII with no space in it.
func keyLike(v string) bool {
	for i := 0; i < len(v); i++ {
		if v[i] < 0x21 || v[i] > 0x7e {
			return false
		}
	}
	return v != ""
}

// headerSafe says a value can be sent as a header as it is: printable,
// with no space at either end.
func headerSafe(v string) bool {
	for _, r := range v {
		if r < 0x20 || r == 0x7f {
			return false
		}
	}
	return v == strings.TrimSpace(v)
}

// errTooLarge is an answer longer than the client reads.
var errTooLarge = errors.New("the answer is larger than the client reads; see WithMaxAnswerBytes")

// errNotCairnPost is an answer that is not what CairnPost says: something
// else is at that address, or stands in between.
var errNotCairnPost = errors.New("the answer was not the JSON a CairnPost server gives")

// mostOfARefusal is how much of an answer that is a refusal is read.
// It is not an option: CairnPost's refusals are a few hundred bytes, and
// nothing past its short fields is kept, so there is nothing to choose.
const mostOfARefusal = 64 << 10

// call is one request to make.
type call struct {
	method string
	// path is the route's segments; each is escaped, so an id is safe.
	path   []string
	query  url.Values
	header http.Header
	body   any // encoded as JSON; nil for none
	// plain marks a route that needs no key and answers its own body on
	// any status.
	plain bool
	// file marks a route whose answer is a file's bytes, not JSON: out is
	// a *content.
	file bool
}

// do makes a call and decodes a 2xx answer into out, which must then be
// whole. With a nil out the answer must be CairnPost's "done" that says
// nothing: a 204 without a body.
// Any other status is an *Error. A failure to reach the server, or to read
// its answer, is a *TransportError.
func (c *Client) do(ctx context.Context, k call, out answer) error {
	u := *c.base
	u.Path, u.RawPath = c.base.Path, c.base.EscapedPath()
	for _, seg := range k.path {
		if seg == "" {
			return errors.New("cairnpost: an id is required")
		}
		u.Path += "/" + seg
		u.RawPath += "/" + url.PathEscape(seg)
	}
	u.RawQuery = k.query.Encode()
	var (
		body io.Reader
		size int64
	)
	if k.body != nil {
		var raw bytes.Buffer
		enc := json.NewEncoder(&raw)
		enc.SetEscapeHTML(false)
		if err := enc.Encode(k.body); err != nil {
			return fmt.Errorf("cairnpost: encode the request: %w", err)
		}
		// Handed over as a plain reader, not as a buffer: net/http replays
		// a request it can rewind when it carries an Idempotency-Key, and
		// this client makes each call once.
		body, size = struct{ io.Reader }{&raw}, int64(raw.Len())
	}
	req, err := http.NewRequestWithContext(ctx, k.method, u.String(), body)
	if err != nil {
		return errors.New("cairnpost: the request could not be built")
	}
	req.ContentLength = size
	for name, values := range k.header {
		req.Header[name] = values
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")
	if k.file {
		req.Header.Set("Accept", "application/octet-stream, application/json")
	}
	req.Header.Set("User-Agent", c.agent)
	if !k.plain {
		req.Header.Set("Authorization", "Bearer "+c.key)
	}
	res, err := c.http.Do(req)
	if err != nil {
		return &TransportError{cause: err}
	}
	defer res.Body.Close()
	fine := res.StatusCode >= 200 && res.StatusCode <= 299
	most := c.most
	if !fine {
		// Of a refusal only its few short fields are kept: as much is
		// read as holds them, whatever the limit on answers is. A limit
		// smaller than CairnPost's own refusal would lose its name, and
		// with it whether the email may have gone.
		most = mostOfARefusal
	}
	raw, err := io.ReadAll(io.LimitReader(res.Body, most+1))
	if err != nil {
		return &TransportError{cause: err}
	}
	if int64(len(raw)) > most {
		if fine {
			return &TransportError{cause: errTooLarge}
		}
		// A refusal this long is not CairnPost's. Its status is still
		// what was answered, and says more than "too large" would.
		raw = nil
	}
	if !fine {
		// A plain call's 503 is an answer to read (the server is up and
		// says what is not); anything else it is refused with is an
		// error like any other, a path that is not there above all.
		if problem := problemOf(res, raw); !k.plain || problem.Name != "" || res.StatusCode != http.StatusServiceUnavailable {
			return problem
		}
	}
	if out == nil {
		// Anything else that answers 2xx here did not do what was asked.
		if res.StatusCode != http.StatusNoContent || len(raw) != 0 {
			return &TransportError{cause: errNotCairnPost}
		}
		return nil
	}
	if k.file {
		// Anything else that answers 200 here is not a CairnPost server.
		if res.Header.Get("Content-Type") != "application/octet-stream" {
			return &TransportError{cause: errNotCairnPost}
		}
		*out.(*content) = raw
		return nil
	}
	if err := json.Unmarshal(raw, out); err != nil || !out.whole() {
		// The answer's text is not quoted: it could hold an email's words.
		return &TransportError{cause: errNotCairnPost}
	}
	return nil
}
