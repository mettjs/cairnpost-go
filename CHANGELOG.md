# Changelog

Every change a user of this client can notice is written here. The format is
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and versions follow
[Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- `Send` and `Get`: send one email, with an `IdempotencyKey` that makes a
  repeated send one email, and read what became of it.
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
