# cairnpost-go

The Go client for [CairnPost](https://github.com/mettjs/CairnPost), a
self-hostable gateway for transactional email. It sends email, reads what
became of it, manages suppressions and webhook endpoints, and verifies the
webhooks CairnPost sends.

It uses the standard library only, and needs the Go version `go.mod` names.

**Status: early.** It follows the server, which has no release yet. The
[changelog](CHANGELOG.md) says what exists.

```sh
go get github.com/mettjs/cairnpost-go
```

## Send an email

```go
client, err := cairnpost.New("https://mail.example.com", os.Getenv("CAIRNPOST_API_KEY"))
if err != nil {
	log.Fatal(err)
}

id, err := client.Send(ctx, cairnpost.Email{
	From:           "Example <hello@example.com>",
	To:             "ana@example.org",
	Subject:        "Welcome",
	Text:           "Glad you are here.",
	IdempotencyKey: "welcome-user-42",
})
```

An email goes to one recipient. `HTML`, `Text` or both; `ReplyTo` and
`Headers` are optional.

**Use an `IdempotencyKey` for anything you may retry.** The same key with
the same content, sent again, is the same email: the retry answers the
first one's id and sends nothing.

## When a send fails

The client never retries by itself. The error says what happened:

```go
id, err := client.Send(ctx, email)

var problem *cairnpost.Error
switch {
case err == nil:
	// Sent. Keep the id.
case errors.As(err, &problem) && problem.Name == cairnpost.ErrRecipientSuppress:
	// The address bounced for good, complained, or was blocked by hand.
	// Nothing was sent now, and nothing will be: stop sending to it, and
	// do not retry. If problem.EmailID is set, an earlier try under the
	// same key got no answer and may have gone: Get(problem.EmailID)
	// says what became of it.
case cairnpost.MayHaveSent(err):
	// It is not known whether the email went. Try again later with the
	// same IdempotencyKey, and only with it. Ask this before the cases
	// below: they mean "nothing was sent" only after it.
case errors.As(err, &problem) && problem.Retryable():
	// Nothing was sent. Try again, after problem.RetryAfter if it is set.
default:
	// The request is at fault, or the key is. Trying again will not help.
}
```

| Error | Meaning |
|---|---|
| `*Error`, status 422 | The request is at fault and will never succeed. `recipient_suppressed` is one of them. |
| `*Error`, 409 `invalid_idempotent_request` | The key was used for a different email, which was sent. |
| `*Error`, 409 `concurrent_idempotent_requests`, 429, 503 | Nothing was sent. Try again, after `RetryAfter` when set. |
| `*Error`, 502 `upstream_unknown` | The mail provider did not answer. The email may have gone. |
| `*Error`, any other 5xx but the 503s above | The server failed after it began, or something in between answered for it (a proxy's 502). The email may have gone. |
| `*TransportError` | The server was not reached, or its answer never came. For a send, the email may have gone. |

`MayHaveSent` is true for the last three, and for a `recipient_suppressed`
that carries an `EmailID`: nothing was sent now, but an earlier try under
the same key got no answer and may have gone. That one is not retried:
the address is suppressed. For the others, the retry that
`MayHaveSent` asks for is not a promise of one email: when the mail
provider's answer was lost, the server lets a retry through after a short
wait, because an email that arrives twice is better than one that never
does.

`Error.EmailID`, when set, is the email's id on the server: `Get` says what
became of it. No error holds the email's subject or body, or the API key;
an answer that is not CairnPost's own is never quoted.

The client does not follow redirects, which would turn a send into a
request with no email in it: give `New` the server's own address. An API
key is sent without the spaces or newline around it.

The client's own HTTP timeout is 40 seconds, longer than the server waits
for its mail provider. If you pass your own `http.Client` with
`WithHTTPClient`, keep its timeout above the server's send timeout: a
client that gives up first cannot know whether its email went.

## What became of an email

```go
m, err := client.Get(ctx, id)
fmt.Println(m.State) // sending, sent, deferred, delivered, bounced, complained, failed, unknown
for _, e := range m.Events {
	fmt.Println(e.Type, e.At, e.Code, e.Reason)
}
```

A send key sees the emails it sent; an admin key sees any.
`cairnpost.IsNotFound(err)` says there is no such email: the id is wrong,
another key's, or older than the server keeps records. (A base URL with a
wrong path answers the same for everything; `Health` tells the two apart.) `m.Content` is what
the email said, set only when the server keeps content and the key is an
admin key.

## Webhooks

CairnPost tells your application what becomes of its emails by posting to a
URL of yours. Each webhook is signed; verify it before you trust it:

```go
verifier, err := cairnpost.NewVerifier(os.Getenv("CAIRNPOST_WEBHOOK_SECRET"))

http.HandleFunc("/webhooks/cairnpost", func(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
	if err != nil {
		http.Error(w, "too large", http.StatusRequestEntityTooLarge)
		return
	}
	event, err := verifier.Verify(r.Header, body) // the body exactly as it arrived
	if err != nil {
		http.Error(w, "not a CairnPost webhook", http.StatusUnauthorized)
		return
	}
	switch event.Type {
	case cairnpost.EventBounced:
		if event.Email.Bounce.Type == cairnpost.BouncePermanent {
			// The address will never take mail.
		}
	case cairnpost.EventComplained:
		// Its owner marked the email as spam.
	}
	w.WriteHeader(http.StatusNoContent)
})
```

- `Verify` refuses a wrong signature (`ErrSignature`), missing headers
  (`ErrUnsigned`) and a timestamp more than five minutes from now
  (`ErrStale`), which is what stops a captured webhook being replayed.
- `event.ID` is the same on every try of a webhook. CairnPost retries one
  that was not answered with a 2xx, and may send one twice: act once per id.
- `event.Email` is set for every type that starts with `email.`, and
  `event.Email.Bounce` for `email.bounced`; a webhook that lacks them is
  refused (`ErrPayload`). `event.Email` is nil for `webhook.test`, the
  webhook sent to try an endpoint out.
- Make the verifier with `NewVerifier`. The zero `Verifier` has no secret
  and refuses everything.
- An `Event` can be passed on as JSON (to a queue, say) and read back
  whole. That form is the client's own, not the one the webhook came in.
- Of the three bounce types only `BouncePermanent` says the address is at
  fault. `BouncePolicy` is a refusal over the sender (a failed signature, a
  blocklist); the address may be perfectly good.
- `event.Email.Suppressed` says CairnPost will now refuse to send to that
  address from that sending domain.

The signature is the Standard Webhooks scheme, which Svix also uses, so any
verifier for either works too.

## With an admin key

```go
// Webhook endpoints. The secret is said only here: keep it.
w, err := admin.CreateWebhook(ctx, cairnpost.NewWebhook{
	URL:    "https://app.example.com/webhooks/cairnpost",
	Events: []string{cairnpost.EventBounced, cairnpost.EventComplained},
})
fmt.Println(w.Secret)

res, err := admin.TestWebhook(ctx, w.ID) // sends a webhook.test now

off := false
admin.UpdateWebhook(ctx, w.ID, cairnpost.WebhookChange{Enabled: &off})

for d, err := range admin.AllDeliveries(ctx, w.ID, cairnpost.DeliveryFailed) {
	if err != nil {
		break
	}
	admin.RetryDelivery(ctx, w.ID, d.ID)
}

// The suppression list.
for s, err := range admin.AllSuppressions(ctx, "example.com") {
	if err != nil {
		break
	}
	fmt.Println(s.Address, s.Reason)
}
admin.Suppress(ctx, "example.com", "ana@example.org", "asked to stop")
admin.Lift(ctx, "example.com", "ana@example.org")
admin.LiftAll(ctx, "example.com", cairnpost.ReasonBounce)
```

| Call | Does |
|---|---|
| `CreateWebhook`, `Webhooks`, `Webhook`, `UpdateWebhook`, `DeleteWebhook` | Manage the URLs told what becomes of emails. |
| `TestWebhook` | Send an endpoint a signed `webhook.test` and say how it answered. |
| `Deliveries`, `AllDeliveries`, `RetryDelivery` | What an endpoint was sent and how each went; try one again now. |
| `Suppressions`, `AllSuppressions` | The addresses not sent to, a page at a time or all of them. |
| `Suppress`, `Lift`, `LiftAll` | Block an address for a sending domain, or let it be sent to again. |
| `Captured` | The emails a development server (capture driver) kept instead of sending. |

In a `WebhookChange` a nil field is left as it is, and a field that is set
is changed to exactly that: `Enabled` pointing at `false` disables the
endpoint, `Events` pointing at an empty list makes it hear about every type.

## The server itself

```go
h, err := client.Health(ctx) // needs no key
fmt.Println(h.OK(), h.Events, h.Webhooks, h.Pruning)

l, err := client.Limits(ctx) // l.Quota is nil for a provider with no allowance
```

## Options

| Option | Does |
|---|---|
| `WithHTTPClient(h)` | Call through your own `http.Client`. Keep its timeout above the server's send timeout, and do not let it follow redirects. |
| `WithUserAgent(name)` | Put your application's name before the client's in `User-Agent`. |
| `WithMaxAnswerBytes(n)` | How much of an answer is read (64 MiB by default). A larger one is an error that says so. |

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md).

## License

[MIT](LICENSE)
