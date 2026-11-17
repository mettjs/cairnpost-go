<p align="center">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset=".github/logo-dark.svg">
    <img src=".github/logo-light.svg" alt="CairnPost" width="330" height="72">
  </picture>
</p>

# cairnpost-go

The Go client for [CairnPost](https://github.com/mettjs/CairnPost), a
self-hostable gateway for transactional email. It sends email, reads what
became of it, manages suppressions and webhook endpoints, and verifies the
webhooks CairnPost sends.

It uses the standard library only, and needs the Go version `go.mod` names.

**Status: early.** It follows the server release by release: this is 0.1.2,
for CairnPost 0.1.1. The [changelog](CHANGELOG.md) says what exists.

```sh
go get github.com/mettjs/cairnpost-go
```

## Send an email

```go
client, err := cairnpost.New("https://mail.example.com", os.Getenv("CAIRNPOST_API_KEY"))
if err != nil {
	log.Fatal(err)
}

sent, err := client.Send(ctx, cairnpost.Email{
	From:           "Example <hello@example.com>",
	To:             []string{"ana@example.org"},
	Subject:        "Welcome",
	Text:           "Glad you are here.",
	IdempotencyKey: "welcome-user-42",
})
```

`sent.ID` is the email's id. `HTML`, `Text` or both; `Cc`, `Bcc`,
`ReplyTo` and `Headers` are optional.

Files go with an email as `Attachments`:

```go
cairnpost.Email{
	// ...
	Attachments: []cairnpost.Attachment{
		{Filename: "invoice.pdf", Content: pdf},
		// Shown in the HTML body as <img src="cid:logo">.
		{Filename: "logo.png", Content: logo, ContentID: "logo"},
	},
}
```

`ContentType` is told from the filename when left empty. The server sets
how many files an email may carry and how large they may be in all (10
files and 10 MiB by default). It keeps a file only where it is set to
keep what emails say, for as long, and while an email waits to be sent;
`FileContent` reads one back. Whether a file with a `ContentID` can be sent depends on the server's
mail provider.

An email may name several addresses in `To`, `Cc` and `Bcc` (50 in all,
unless the server is set otherwise). It is still one email with one id.
An address on the sending domain's suppression list is left out and the
email goes to the others: `sent.Suppressed` names who was left out, as
`Message.Suppressed` does when the email is read later. The send is an
error when nobody in `To` or `Cc` is left: a `Bcc` alone is not sent.

**Use an `IdempotencyKey` for anything you may retry.** The same key with
the same content, sent again, is the same email: the retry answers the
first one's id and sends nothing. An empty key is no key at all, and the
send is then not idempotent: make sure yours is not empty when it is put
together from other values.

Several emails go in one request with `SendBatch`:

```go
results, err := client.SendBatch(ctx, cairnpost.Batch{
	Emails:         emails,
	IdempotencyKey: "welcome-run-42",
})
if err != nil {
	// The batch was refused whole (one email could never be sent, say):
	// nothing was sent.
}
for i, r := range results {
	if r.Err != nil {
		// Email i did not go: the same *cairnpost.Error Send would give.
		continue
	}
	// r.Sent.ID
}
```

A nil error says the batch was taken, not that every email went: read
each result, or count with `cairnpost.Failed(results)`. With an
`IdempotencyKey` on the batch, the same batch sent again (the same emails
in the same order) sends only what did not go. Each failed email's
`RetryAfter` is its own. The server sets how many
emails a batch may hold (100 by default) and how long it may take; a
server given more than its default minute wants a client with a longer
timeout than `DefaultTimeout`.

An email can be asked for now and sent later:

```go
sent, err := client.Send(ctx, cairnpost.Email{
	// ...
	ScheduledAt: time.Now().Add(24 * time.Hour),
})
// sent.ScheduledAt is when it goes; nothing was sent yet.

err = client.Reschedule(ctx, sent.ID, time.Now().Add(48*time.Hour))
err = client.Cancel(ctx, sent.ID)
```

Until its time the email's `State` is `StateScheduled`; from then on it
is an email like any other, under the same id. `Cancel` and `Reschedule`
answer an `*Error` named `ErrNotScheduled` once the email is being sent,
was sent or was cancelled. The server keeps what a waiting email says,
and its files, until it is sent or cancelled. An address suppressed by
the time the email goes is left out; with nobody left in `To` or `Cc`,
the email is cancelled and `Message.Reason` says `"suppressed"`.
Revoking an API key cancels the emails it asked for that still wait. The
emails of a `Batch` may each have a `ScheduledAt`. An `IdempotencyKey` is
remembered for a day from the asking, not until the email goes.

## Templates

A template is an email the server keeps, written once with variables as
`{{name}}`. An admin key keeps one, and any key sends from it:

```go
_, err := admin.CreateTemplate(ctx, cairnpost.NewTemplate{
	Name:     "welcome",
	Subject:  "Welcome, {{name}}",
	HTML:     "<p>Your code is <b>{{code}}</b></p>",
	Text:     "Your code is {{code}}",
	Defaults: map[string]string{"name": "there"},
})

sent, err := client.Send(ctx, cairnpost.Email{
	From: "app@example.com",
	To:   []string{"ana@example.org"},
	Template: &cairnpost.UseTemplate{
		Name:      "welcome",
		Variables: map[string]string{"code": "4821"},
	},
})
```

Leave `HTML` and `Text` empty with a `Template`; a `Subject` given with
it is used instead of the template's. A value is written as it is in the
subject and the text and escaped in the HTML, so it is never markup
there: keep attributes in quotes (`href="{{link}}"`). A
variable with no default must be given, and a value for a variable the
template does not name is refused: both are an `*Error` with status 422,
and nothing is sent. The email is filled when it is asked for, so one
that waits (`ScheduledAt`) keeps what it was filled with whatever becomes
of the template. A template belongs to a sending domain or, with an empty
`Domain` and a key bound to none, serves every domain; a send gets its
own domain's template of that name first.

## When a send fails

The client never retries by itself. The error says what happened:

```go
sent, err := client.Send(ctx, email)

var problem *cairnpost.Error
switch {
case err == nil:
	// Sent. Keep sent.ID.
case errors.As(err, &problem) && problem.Name == cairnpost.ErrRecipientSuppress:
	// The address bounced for good, complained, or was blocked by hand
	// (everyone in To and Cc, if the email named several).
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
| `*Error`, 409 `concurrent_idempotent_requests`, 408 `request_incomplete`, 429, 503 | Nothing was sent. Try again, after `RetryAfter` when set. The 408 is a request that did not arrive whole in time, a 503 can be a server busy with other large emails. |
| `*Error`, 502 `upstream_unknown` | The mail provider did not answer. The email may have gone. |
| `*Error`, any other 5xx but the 503s above, or a 408 that is not CairnPost's own | The server failed after it began, or something in between answered for it (a proxy's 502 or 408). The email may have gone. |
| `*TransportError` | The server was not reached, its answer never came, or what answered 2xx was not CairnPost. For a send, the email may have gone. |

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

A 2xx is believed only when it says what CairnPost always says there.
Something else at the address, or in between, can answer 200 to anything:
an empty object, a `null`, a page, its own `{"status":"ok"}`. That is a
`*TransportError`, never "cancelled", "deleted", "not suppressed", an
empty list or a healthy server.

The client does not follow redirects, which would turn a send into a
request with no email in it: give `New` the server's own address. An API
key is sent without the spaces or newline around it.

The client's own HTTP timeout is two and a half minutes, longer than the
server takes for its slowest answer: a batch, which may take 30 seconds
to arrive, starts emails for a minute and then waits for the mail
provider on the last of them, two minutes in all. If you pass
your own `http.Client` with `WithHTTPClient`, keep its timeout above that
(and higher still if the server's send or batch timeout was raised): a
client that gives up first cannot know whether its emails went.

## What became of an email

```go
m, err := client.Get(ctx, id)
fmt.Println(m.State) // sending, sent, deferred, delivered, bounced, complained, failed, unknown
for _, r := range m.Recipients {
	fmt.Println(r.Address, r.Kind, r.State) // each address has its own outcome
}
for _, e := range m.Events {
	fmt.Println(e.To, e.Type, e.At, e.Code, e.Reason)
}
```

`m.To` and `m.State` are the first address and what became of the email
there; for an email to several, read `m.Recipients`: `failed` at one
address does not say the email went to nobody.

A send key sees the emails it sent; an admin key sees any, or its
sending domain's if it is bound to one.
`cairnpost.IsNotFound(err)` says there is no such email: the id is wrong,
another key's, or older than the server keeps records.
`cairnpost.IsWrongAddress(err)` says something else: the request reached
no route of CairnPost's, so the base URL given to `New` is most likely
wrong, and no record is known to be missing. `Health` fails the same way
on a wrong base URL, so it is the call to check a configuration with.
(Both need the server's own error answers to arrive: a proxy that puts its
own page in place of every 404 makes a missing record read as a wrong
address.) `m.Content` is what
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
- `event.Email` is set for `email.delivered`, `email.delivery_delayed`,
  `email.bounced`, `email.complained` and `email.failed`, and
  `event.Email.Bounce` for `email.bounced`; a webhook of those types that
  lacks them is refused (`ErrPayload`). `event.Email` is nil for
  `webhook.test`, the webhook sent to try an endpoint out.
- A type this client does not know (a newer server's) is passed on, with
  an `event.Email` only if it names one: check it for nil in a `default`
  case.
- `event.Email.To` holds the one address the event is about, also when
  the email went to several; `event.Email.ID` is the email's one id.
- `email.received` (`cairnpost.EventReceived`) tells of an email that
  came for one of your addresses, where the server takes them. It is
  about no email you sent: `event.Email` is nil and `event.Received`
  says who it is from and for, and the id to read it by (see "Receiving
  email"). An endpoint that hears about every event gets these too.
- `address.suppressed` (`cairnpost.EventAddressSuppressed`) tells of an
  address suppressed because of a bounce or a complaint about an email
  the server has no record of. It names no email: `event.Suppression`
  holds the sending domain, the address and the reason (`ReasonBounce` or
  `ReasonComplaint`). Treat it as you treat an `email.bounced` or an
  `email.complained` with `Suppressed` set.
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

## Receiving email

A server set up to take the emails that come for its domains' addresses
keeps each for a time (7 days by default) and tells your endpoint with an
`email.received` webhook. Read it with an admin key:

```go
if event.Type == cairnpost.EventReceived {
	got, err := admin.Received(ctx, event.Received.ID)
	// got.Content.Subject, .Text, .HTML, .Attachments ...
	pdf, err := admin.ReceivedFile(ctx, got.ID, 1) // its first file
	raw, err := admin.ReceivedRaw(ctx, got.ID)     // every byte, as it came
}
```

A stranger wrote it: treat every part as untrusted, and never show its
HTML as a page. `got.From` is what the sending server said and
`got.Content.From` what the email says of itself; either can be made up.
`Content.Partial` says some of it could not be read as it was written
(a charset the server does not know); `ReceivedRaw` always has all of it.
`ReceivedList` and `AllReceived` list what is kept, and `DeleteReceived`
deletes one before its time. `IsNotFound` says an email's time is up.

## With an admin key

```go
// Webhook endpoints. The secret is said only here: keep it.
w, err := admin.CreateWebhook(ctx, cairnpost.NewWebhook{
	URL:    "https://app.example.com/webhooks/cairnpost",
	Events: []string{cairnpost.EventBounced, cairnpost.EventComplained},
})
fmt.Println(w.Secret)

res, err := admin.TestWebhook(ctx, w.ID) // sends a webhook.test now

// A new signing secret, said only here. Until w.PreviousSecretUntil the
// server signs with the old one too: change the application's secret
// within that time. If it is nil, the old one has stopped already.
w, err = admin.RotateWebhookSecret(ctx, w.ID)

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
| `RotateWebhookSecret` | Give an endpoint a new signing secret; the old one still signs for a day, unless the server is set otherwise. |
| `ResumeWebhook` | End an endpoint's pause, and have what it is owed sent. |
| `Deliveries`, `AllDeliveries`, `RetryDelivery` | What an endpoint was sent and how each went; try one again now. |
| `Suppressions`, `AllSuppressions` | The addresses not sent to, a page at a time or all of them. |
| `Suppress`, `Lift`, `LiftAll` | Block an address for a sending domain, or let it be sent to again. |
| `Received`, `ReceivedList`, `AllReceived`, `ReceivedRaw`, `ReceivedFile`, `DeleteReceived` | The emails that came for your addresses, while the server keeps them. |
| `CreateTemplate`, `Templates`, `Template`, `UpdateTemplate`, `DeleteTemplate` | Manage the templates emails are sent from. `ErrNameTaken` for a name its domain has; `ErrChangedMeanwhile` (retryable) when others changed it at the same time. |
| `FileContent` | What one of an email's files held, where the server keeps it (`File.Kept`). An admin key. |
| `Captured` | The emails a development server (capture driver) kept instead of sending; `ErrNotCapturing` from a server that delivers for real. |

In a `WebhookChange` a nil field is left as it is, and a field that is set
is changed to exactly that: `Enabled` pointing at `false` disables the
endpoint, `Events` pointing at an empty list makes it hear about every type.

An endpoint that fails every try for days is paused by the server:
`w.PausedAt` is set, nothing is sent to it, and it is still owed everything
that happens. `admin.ResumeWebhook(ctx, w.ID)` resumes it, and what it is
owed is sent then; a `WebhookChange` never does. `w.FailingSince` says
since when every try has failed.

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
