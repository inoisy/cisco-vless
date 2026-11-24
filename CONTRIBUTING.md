# Contributing

Issues and pull requests are welcome in English or Russian. Code comments and
the interface are currently in Russian; please keep new interface strings in
Russian so one screen doesn't mix languages.

## Build

Go 1.26 or newer.

```
go build ./cmd/cvpn            # CLI and service, no cgo
go build ./cmd/cvpn-gui        # tray icon and window
```

The GUI uses [Wails v3](https://v3.wails.io):

| | needed to build the GUI |
|---|---|
| macOS | Xcode Command Line Tools |
| Debian / Ubuntu | `build-essential pkg-config libgtk-4-dev libwebkitgtk-6.0-dev` |
| Windows | nothing extra, no cgo; add `-ldflags -H=windowsgui` |

## Test

```
gofmt -l .
go vet ./...
go test ./...
```

Without the GUI libraries on Linux, use `./cmd/cvpn ./internal/...` in place of
`./...`. CI runs vet, tests and both builds on macOS (arm64, amd64), Linux and
Windows (amd64, arm64 each). Platform code lives in `*_darwin.go`,
`*_linux.go` and `*_windows.go`, so at least cross-check the others:

```
GOOS=linux   CGO_ENABLED=0 go vet ./cmd/cvpn ./internal/...
GOOS=windows CGO_ENABLED=0 go vet ./cmd/cvpn ./internal/...
```

## Checking without a Cisco gateway

You don't need a corporate network for most changes.

**Service, CLI and settings window.** `CVPN_ROOT` puts the socket, settings and
logs in one directory, and no root is needed. Keep the path short: a unix
socket path is limited to about 100 characters.

```
CVPN_ROOT=/tmp/cvpn ./cvpn daemon
CVPN_ROOT=/tmp/cvpn ./cvpn
CVPN_ROOT=/tmp/cvpn ./cvpn-gui -serve 127.0.0.1:8765    # window in a browser
```

Settings, validation, status and logs work there. Bringing up a tunnel does
not: that needs root. `-serve` has no authentication, so keep it on localhost.

**sing-box config.** Print it without the service and let sing-box check it:

```
./cvpn _singbox-config 'vless://11111111-2222-3333-4444-555555555555@192.0.2.10:443?security=tls&sni=example.com' > /tmp/singbox.json
sing-box check -c /tmp/singbox.json
```

**Routes and DNS from the gateway.** openconnect passes them to
`cvpn _vpnc-script` in environment variables; `internal/vpncscript` has tests
that call it the same way.

**A real tunnel.** [ocserv](https://ocserv.gitlab.io/www/) is an open-source
server that speaks the AnyConnect protocol. Nothing in this repository sets it
up, and CI doesn't use it. If your change touches the connection itself, say in
the pull request what you tested against (gateway or ocserv, OS), or that you
couldn't.

## Pull requests

- One change per pull request, with a test where the code can be tested.
- [Conventional Commits](https://www.conventionalcommits.org) for commit
  titles: `fix(cisco): ...`, `feat(singbox): ...`. The `T-12345` suffix in the
  history is the maintainer's; you don't need it.
- No real servers, keys, logins or company names in code, tests or logs. Use
  `example.com` and the RFC 5737 addresses (`192.0.2.0/24`, `198.51.100.0/24`,
  `203.0.113.0/24`).
- An upgrade must not change routes for someone who already has a
  `config.json`: new settings need a default that keeps the old behavior.
- If you change something in `README.md`, change `README.ru.md` too, or say
  that it needs translating.

## Releases

The maintainer pushes a `v*` tag; CI builds every platform and publishes a
release with `SHA256SUMS`.
