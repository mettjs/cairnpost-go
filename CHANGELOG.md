# Changelog

Every change a user of this client can notice is written here. The format is
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and versions follow
[Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- `Send` and `Get`: send one email, with an `IdempotencyKey` that makes a
  repeated send one email, and read what became of it.
- Several addresses: `Email.To`, `Cc` and `Bcc` are lists. `Send` answers a
  `Sent` with the email's `ID` and, under `Suppressed`, the addresses it
  was not sent to, which `Message.Suppressed` says later; `Message.Recipients` says what became of the email at
  each address, and each `MessageEvent` names its address in `To`.
- Attachments: `Email.Attachments` sends files with an email, beside it or
  shown in its HTML by a `ContentID`. `Content.Attachments` and
  `CapturedEmail.Attachments` say what the files were.
- `SendBatch`: several emails in one request, each with its own result;
  with an `IdempotencyKey` the same batch sent again sends only what did
  not go. `Failed` counts the emails that did not go.
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
