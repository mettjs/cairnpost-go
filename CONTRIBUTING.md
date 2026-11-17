# Contributing to cairnpost-go

This is the Go client for [CairnPost](https://github.com/mettjs/CairnPost),
a small gateway for transactional email. It does what the server's API
does, typed, and verifies the webhooks the server sends. It is small on
purpose.

## What it is not

- It does not retry by itself. It says what happened and whether trying
  again is right; the application decides.
- It has no dependencies beyond Go's standard library, and takes none.
- It never puts an email's subject or body, or the API key, in an error.

## Tests

```sh
gofmt -l .        # must print nothing
go vet ./...
go test -race ./...
```

The integration tests run against a real CairnPost server and are behind a
build tag. Start a server with the capture driver, so nothing is sent to
anybody, with `CAIRNPOST_WEBHOOK_ALLOW_PRIVATE=true`, so its webhooks
can reach your machine, and with a `CAIRNPOST_INBOUND_SECRET`, so the
tests can hand it an email as a mail provider would
(`CAIRNPOST_WEBHOOK_POLL=1s` keeps them short); make a send key and an
admin key; then:

```sh
export CAIRNPOST_URL=http://localhost:8025
export CAIRNPOST_SEND_KEY=...
export CAIRNPOST_ADMIN_KEY=...
export CAIRNPOST_INBOUND_SECRET=...   # the one the server was started with
go test -tags=integration -count=1 ./...
```

With the tag and without those variables they fail rather than skip.

## Conventions

- **One file per part of the API**, small, with its tests beside it.
- **The standard library only.**
- **The latest stable Go**, as `go.mod` names it.
- **Errors say what to do**, and never echo the server's raw answer.
- **Documentation is true now.** A change to what the client does updates
  the README and the changelog with it.
- **Commits** follow [Conventional Commits](https://www.conventionalcommits.org/en/v1.0.0/),
  one concern each, and say why. Versions follow
  [Semantic Versioning](https://semver.org/spec/v2.0.0.html), and the
  changelog follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).
