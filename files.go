package cairnpost

import (
	"context"
	"errors"
	"net/http"
	"strconv"
)

// FileContent reads what one of an email's files held, as it was sent.
// place counts the email's files from 1, in the order of
// Content.Attachments. An admin key only. IsNotFound says there is none:
// no email has this id, it has no file at that place, or the server does
// not keep it (File.Kept says whether it does).
func (c *Client) FileContent(ctx context.Context, id string, place int) ([]byte, error) {
	if place < 1 {
		return nil, errors.New("cairnpost: a file's place counts from 1")
	}
	var content []byte
	k := call{method: http.MethodGet, path: []string{"emails", id, "attachments", strconv.Itoa(place)}, file: true}
	if err := c.do(ctx, k, &content); err != nil {
		return nil, err
	}
	return content, nil
}
