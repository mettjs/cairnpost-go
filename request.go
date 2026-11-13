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
}

// do makes a call and decodes a 2xx answer into out (nil to ignore it).
// Any other status is an *Error. A failure to reach the server, or to read
// its answer, is a *TransportError.
func (c *Client) do(ctx context.Context, k call, out any) error {
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
	req.Header.Set("User-Agent", c.agent)
	if !k.plain {
		req.Header.Set("Authorization", "Bearer "+c.key)
	}
	res, err := c.http.Do(req)
	if err != nil {
		return &TransportError{cause: err}
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(res.Body, c.most+1))
	if err != nil {
		return &TransportError{cause: err}
	}
	if int64(len(raw)) > c.most {
		return &TransportError{cause: errTooLarge}
	}
	if !k.plain && (res.StatusCode < 200 || res.StatusCode > 299) {
		return problemOf(res, raw)
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(raw, out); err != nil {
		// The answer's text is not quoted: it could hold an email's words.
		return &TransportError{cause: errNotCairnPost}
	}
	return nil
}
