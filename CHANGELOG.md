# Changelog

Every change a user of this client can notice is written here. The format is
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and versions follow
[Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- `Send` and `Get`: send one email, with an `IdempotencyKey` that makes a
  repeated send one email, and read what became of it.
- `Error`, `TransportError`, `MayHaveSent` and `IsNotFound`: what the server
  refused, whether trying again is right, and whether an email may have gone.
- `NewVerifier` and `Verify`: check that a webhook is CairnPost's and read
  it as a typed `Event`.
- Suppressions: `Suppressions`, `AllSuppressions`, `Suppress`, `Lift`,
  `LiftAll`.
- Webhook endpoints: `CreateWebhook`, `Webhooks`, `Webhook`,
  `UpdateWebhook`, `DeleteWebhook`, `TestWebhook`, and their deliveries:
  `Deliveries`, `AllDeliveries`, `RetryDelivery`.
- `Limits`, `Health` and `Captured`.
- Options: `WithHTTPClient`, `WithUserAgent`, `WithMaxAnswerBytes`.
