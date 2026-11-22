# Changelog

Every change a user of this client can notice is written here. The format is
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and versions follow
[Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Fixed

- `NewVerifier` refuses a signing secret with a line break inside it, as
  the Node.js and Python clients do. It took one as the secret it would
  be without the break. A secret with space or a line break around it is
  still taken.

## [0.2.1] - 2026-11-17

### Fixed

- `Cancel` is done only on an answer that says the email is cancelled,
  and `Reschedule` on one that says the time asked for. Both took any
  answer that named the email, so the email's own record, which is at
  the path a move is asked at, read as "moved" while the email kept its
  old time.
- `Health` wants every field a server says of itself, and `Templates`
  wants to be told whether there are more. Something else that answered
  a status, a driver and a version passed for a server whose parts were
  all unnamed, and a list that left `more` out read as complete.

## [0.2.0] - 2026-11-17

### Changed

- `Verify` passes on a webhook whose type starts with `email.` and is
  not one this client knows, even when it names no email: its
  `Event.Email` is then nil. Before, it was refused with `ErrPayload`,
  so a newer server's new type could not reach an older client. Check
  `Event.Email` for nil before using it under a type you do not handle
  by name.

### Fixed

- A refusal with a body longer than the client reads (a proxy's long
  error page under `WithMaxAnswerBytes`) is the `*Error` with its status
  that it is. It was a `*TransportError` saying the answer was too
  large, which lost the status and read as "may have sent" even for a
  404. 64 KiB of a refusal are read, whatever `WithMaxAnswerBytes`
  says.
- `WithMaxAnswerBytes(math.MaxInt64)`, as "no limit", made every answer
  read as empty and so refused: a send that went came back as a
  `*TransportError` with its id lost.
- `Captured`, asked of a server without that path, answers the error
  that says why when `Health` fails too. It answered "nothing at this
  path" (a wrong base URL by `IsWrongAddress`) also when the server had
  just stopped answering or the time ran out; that is now the
  `*TransportError` it is, and a wrong address only when `Health` finds
  one.
- `Error.Retryable` is true of a 409 `changed_meanwhile`, which says to
  read the template and try again. It answered false, as for the 409s
  that must not be retried.
- `Verify` refuses (`ErrPayload`) a webhook that does not say when it
  happened, when its email was made or when its received email came,
  and an `email.bounced` whose bounce names no type. They were handed
  on with a zero time or an empty `Bounce.Type`.
- A 2xx that does not say what CairnPost always says is no longer taken
  for an answer. Something else at the address, or in between, can
  answer 200 to anything, and `{}`, `null` or a field said as `null`
  read as "done", "no" or "none": `Cancel`, `Reschedule`, `Lift`,
  `RetryDelivery` and the three deletes succeeded, `Suppress` answered
  "already suppressed", `LiftAll` answered 0, `TestWebhook` answered
  "not delivered", and every list was empty. Each is now a
  `*TransportError`. A delete is done only on a 204 without a body. A
  record that does not say which it is and when it was made is refused,
  alone or in a list. `Cancel` and `Reschedule` want the answer to name
  the email asked about, `Lift` that one address was lifted,
  `RetryDelivery` that the webhook is pending, `CreateWebhook` and
  `RotateWebhookSecret` the secret they alone say, and `Health` the
  driver and the version, so that another service's `{"status":"ok"}`
  no longer passes for a CairnPost server.

## [0.1.2] - 2026-11-16

### Added

- `ErrRequestIncomplete`, the name of the server's 408: a request that did
  not arrive whole in time.

### Fixed

- A 408 `request_incomplete` is now read as CairnPost's own answer:
  `Error.Name` is `ErrRequestIncomplete` and `Error.Message` the server's
  sentence. Before, the name was not known to the client, so both were
  empty; `Retryable` already said to try again.
- `MayHaveSent` is now true of a 408 that is not CairnPost's own. A proxy
  can answer one after it passed the request on, while the server is still
  sending: such an email may have gone, and is to be retried only with the
  same `IdempotencyKey`.

## [0.1.1] - 2026-11-15

### Added

- `ErrSenderDomainNotReady`, the name CairnPost 0.1.1 answers (422) when
  the mail provider refuses an email and the sending domain's DNS lacks a
  record the provider asks: the domain is likely not set up, and nothing
  from it will be sent until it is. An older client reads that answer as
  a 422 with an empty `Name`, so take this version before the server's.

## [0.1.0] - 2026-11-15

### Added

- `Send` and `Get`: send one email, with an `IdempotencyKey` that makes a
  repeated send one email, and read what became of it.
- Several addresses: `Email.To`, `Cc` and `Bcc` are lists. `Send` answers a
  `Sent` with the email's `ID` and, under `Suppressed`, the addresses it
  was not sent to, which `Message.Suppressed` says later; `Message.Recipients` says what became of the email at
  each address, and each `MessageEvent` names its address in `To`.
- Attachments: `Email.Attachments` sends files with an email, beside it or
  shown in its HTML by a `ContentID`. `Content.Attachments` and
  `CapturedEmail.Attachments` say what the files were. `FileContent` reads
  a file back from a server that keeps it, which `File.Kept` says.
- `SendBatch`: several emails in one request, each with its own result;
  with an `IdempotencyKey` the same batch sent again sends only what did
  not go. `Failed` counts the emails that did not go.
- Sending later: `Email.ScheduledAt` has an email sent at a time to come;
  `Sent.ScheduledAt` says it waits. `Reschedule` and `Cancel` move and
  cancel it. `StateScheduled`, `StateCancelled`, `Message.ScheduledAt` and
  `Message.Reason`; `ErrNotScheduled` and `ErrScheduleFull`.
- Templates: `Email.Template` makes an email from a template the server
  keeps, with a value for each of its variables. `CreateTemplate`,
  `Templates`, `Template`, `UpdateTemplate` and `DeleteTemplate` manage
  them; `ErrNameTaken` and `ErrChangedMeanwhile`.
- Receiving email: `EventReceived` and `Event.Received` tell of an email
  that came; `Received`, `ReceivedList`, `AllReceived`, `ReceivedRaw`,
  `ReceivedFile` and `DeleteReceived` read and delete what the server
  keeps. `Delivery.InboundID`; `Health.Inbound` and `Health.Scheduled`.
- `Error`, `TransportError`, `MayHaveSent`, `IsNotFound` and
  `IsWrongAddress`: what the server refused, whether trying again is right,
  whether an email may have gone, and whether the base URL is at fault
  instead of a record being missing.
- `NewVerifier` and `Verify`: check that a webhook is CairnPost's and read
  it as a typed `Event`.
- Suppressions: `Suppressions`, `AllSuppressions`, `Suppress`, `Lift`,
  `LiftAll`.
- Webhook endpoints: `CreateWebhook`, `Webhooks`, `Webhook`,
  `UpdateWebhook`, `DeleteWebhook`, `TestWebhook`, `RotateWebhookSecret`,
  `ResumeWebhook`,
  and their deliveries: `Deliveries`, `AllDeliveries`, `RetryDelivery`. A
  `Webhook` says whether it is failing or paused.
- `EventAddressSuppressed`: the webhook for an address suppressed because
  of an email the server has no record of, read into `Event.Suppression`.
- `Limits`, `Health` and `Captured`. `Health` fails as any call does on a
  wrong base URL; `Captured` answers `ErrNotCapturing` from a server that
  delivers for real.
- Options: `WithHTTPClient`, `WithUserAgent`, `WithMaxAnswerBytes`.

[Unreleased]: https://github.com/mettjs/cairnpost-go/compare/v0.2.1...HEAD
[0.2.1]: https://github.com/mettjs/cairnpost-go/compare/v0.2.0...v0.2.1
[0.2.0]: https://github.com/mettjs/cairnpost-go/compare/v0.1.2...v0.2.0
[0.1.2]: https://github.com/mettjs/cairnpost-go/compare/v0.1.1...v0.1.2
[0.1.1]: https://github.com/mettjs/cairnpost-go/compare/v0.1.0...v0.1.1
[0.1.0]: https://github.com/mettjs/cairnpost-go/releases/tag/v0.1.0
