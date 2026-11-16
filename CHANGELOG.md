# Changelog

Every change a user of this client can notice is written here. The format is
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and versions follow
[Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

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

[Unreleased]: https://github.com/mettjs/cairnpost-go/compare/v0.1.2...HEAD
[0.1.2]: https://github.com/mettjs/cairnpost-go/compare/v0.1.1...v0.1.2
[0.1.1]: https://github.com/mettjs/cairnpost-go/compare/v0.1.0...v0.1.1
[0.1.0]: https://github.com/mettjs/cairnpost-go/releases/tag/v0.1.0
