// Package cairnpost is the Go client for CairnPost, a self-hostable gateway
// for transactional email. It sends email, reads what became of it, manages
// suppressions and webhook endpoints, and verifies the webhooks CairnPost
// sends. It uses the standard library only.
//
// The client never retries by itself. An error says what happened and
// whether trying again is right (see Error); the application decides.
package cairnpost

import (
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Version is this client's version, said in its User-Agent.
const Version = "0.1.0-dev"

// DefaultTimeout is how long a call may take with the client's own HTTP
// client. It is longer than the server gives its slowest answer (a
// batch, which by the server's own settings may take a minute and a
// half): a client that gave up sooner would not know whether its emails
// went. A server set to give a batch longer wants a client given longer.
const DefaultTimeout = 2 * time.Minute

// DefaultMaxAnswerBytes is how much of an answer is read unless
// WithMaxAnswerBytes says otherwise. Only a development server's list of
// captured emails comes near it.
const DefaultMaxAnswerBytes = 64 << 20

// Client calls one CairnPost server with one API key. It is safe for use
// by many goroutines.
type Client struct {
	base  *url.URL
	key   string
	http  *http.Client
	agent string
	most  int64
}

// Option adjusts a Client.
type Option func(*Client)

// WithHTTPClient makes the client call through h; a nil h is ignored. Its
// timeout should be longer than the server's send timeout, as
// DefaultTimeout is. The client's own does not follow redirects: one
// would turn a send into a request with no email in it.
func WithHTTPClient(h *http.Client) Option {
	return func(c *Client) {
		if h != nil {
			c.http = h
		}
	}
}

// WithMaxAnswerBytes sets how much of an answer is read. A larger answer
// is an error that says so.
func WithMaxAnswerBytes(n int64) Option {
	return func(c *Client) {
		if n > 0 {
			c.most = n
		}
	}
}

// WithUserAgent puts the application's name before the client's own in the
// User-Agent header. A name a header cannot carry is ignored.
func WithUserAgent(name string) Option {
	return func(c *Client) {
		if name != "" && headerSafe(name) {
			c.agent = name + " " + c.agent
		}
	}
}

// New returns a client for the server at baseURL ("https://mail.example.com")
// using apiKey. A send key may send and read its own emails; the calls
// marked "admin key" need an admin key.
func New(baseURL, apiKey string, opts ...Option) (*Client, error) {
	u, err := url.Parse(strings.TrimRight(baseURL, "/"))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, errors.New("cairnpost: the base URL must be http:// or https:// and name a host")
	}
	// A key read from a file often ends in a newline; one with anything
	// else a header cannot carry is refused here, not at the first call.
	apiKey = strings.TrimSpace(apiKey)
	if apiKey == "" || !headerSafe(apiKey) {
		return nil, errors.New("cairnpost: an API key is required, and holds no spaces or control characters")
	}
	own := &http.Client{Timeout: DefaultTimeout,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	c := &Client{base: u, key: apiKey, http: own, agent: "cairnpost-go/" + Version, most: DefaultMaxAnswerBytes}
	for _, opt := range opts {
		opt(c)
	}
	return c, nil
}
