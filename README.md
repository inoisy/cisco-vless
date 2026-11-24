# cvpn — Cisco AnyConnect alongside VLESS, WireGuard or AmneziaWG

[Русский](README.ru.md)

Corporate Cisco AnyConnect and a personal VPN at the same time: the work
network goes through Cisco, everything else through VLESS, WireGuard or
AmneziaWG. macOS, Linux and Windows; command line and a tray icon.

Normally the two don't mix. Cisco Secure Client, Hiddify, V2Box, Amnezia and
the like each take over routes and DNS, so after the second connection either
work or everything else stops working. `cvpn` replaces them with one program
where the order of connecting doesn't matter.

```
cvpn on 123456     # vless + cisco (123456 is the one-time code, if you need one)
cvpn               # status
cvpn off           # turn everything off
```

Or use the tray icon: switch Cisco and the exit on and off, open the settings
and logs window.

> The interface (CLI, window, installers) is in Russian only for now.

## Install

You need: the Cisco server address, login and password; a `vless://...` link
(or an `https://...` subscription URL) and/or a WireGuard / AmneziaWG config.

**macOS** ([Homebrew](https://brew.sh)) and **Linux** (Debian/Ubuntu with
systemd):

```
git clone https://github.com/inoisy/cisco-vless && cd cisco-vless
./install.sh
```

**Windows**: first install
[OpenConnect-GUI](https://gui.openconnect-vpn.net/download/) (it provides
`openconnect.exe` and the wintun driver), then in PowerShell as administrator:

```
git clone https://github.com/inoisy/cisco-vless; cd cisco-vless
powershell -ExecutionPolicy Bypass -File install.ps1
```

If Go is installed, the installer builds `cvpn` from source, otherwise it
downloads a release. It then installs the `cvpn` service and the tray icon
(started at login) and asks for settings: Cisco server, login, group,
corporate domains, VLESS link, WireGuard / AmneziaWG configs. The Cisco
certificate fingerprint is shown for confirmation. The Cisco password is kept
in the Keychain on macOS, and in a file readable only by root / SYSTEM on
Linux and Windows.

Change settings later in the settings window or with `cvpn setup`.

With a subscription, cvpn takes every server from it (VLESS over TCP and
Hysteria2; sing-box has no XHTTP) and puts them in a `urltest` group: sing-box
checks each one every minute and keeps the live one with the lowest latency.
The subscription is downloaded when vless is switched on; if that fails, the
last copy is used.

Exits other than VLESS are installed separately:

| | macOS | Linux | Windows |
|---|---|---|---|
| WireGuard | `CVPN_WG=1 ./install.sh` | `CVPN_WG=1 ./install.sh` | [WireGuard client](https://www.wireguard.com/install/) |
| AmneziaWG | `CVPN_AWG=1 ./install.sh` (built from source) | `CVPN_AWG=1 ./install.sh` (ppa:amnezia/ppa) | [AmneziaWG client](https://github.com/amnezia-vpn/amneziawg-windows-client/releases) |

Turn off Cisco Secure Client and other VPN clients first: `cvpn status` and
the window warn you if they are connected.

### What bypasses VLESS

By default everything except the corporate network and private addresses goes
through VLESS. Exceptions are set in `cvpn setup`, in the settings window or in
`config.json`:

```json
"routing": {
  "ru_direct": true,
  "proxy_domains": ["api.example.ru"]
}
```

- `ru_direct`: Russian sites go directly, from your provider's address, since
  marketplaces and banks there block or challenge foreign IPs. Covers the
  `.ru`, `.su`, `.рф`, `.рус`, `.moscow` and `.москва` zones plus geoip RU; DNS
  for these zones goes to Yandex (77.88.8.8) outside the tunnel.
- `proxy_domains`: exact host names that go through VLESS even if `ru_direct`
  matches them, for services with an IP allowlist.

Takes effect on the next `cvpn vless on`. A `config.json` written before this
setting existed keeps the old rules: `ru_direct` on, `api.reg.ru` through
VLESS.

## Commands

| command | what it does |
|---|---|
| `cvpn` | status: what is on, external IP, whether the corporate host resolves |
| `cvpn on [vless\|wg\|awg] [otp]` | exit (vless by default) + cisco |
| `cvpn off` | turn everything off |
| `cvpn cisco on [otp]` / `off` | corporate network only |
| `cvpn vless on` / `off` | all other traffic through VLESS |
| `cvpn wg on` / `off` | all other traffic through WireGuard |
| `cvpn awg on` / `off` | all other traffic through AmneziaWG |
| `cvpn boot vless\|wg\|awg\|off` | which exit to bring up at system start |
| `cvpn setup` | settings |
| `cvpn password` | change the Cisco password |
| `cvpn log` | cisco, vless and service logs |
| `cvpn service install\|uninstall\|restart` | the service (sudo / administrator) |

cisco works with any exit; vless, wg and awg are mutually exclusive: turning
one on turns the other off.

## How it works

- **Service** (`cvpn daemon`: launchd / systemd / Windows service) runs as
  root/SYSTEM and keeps openconnect and sing-box as child processes. The CLI
  and the tray icon are its clients over a unix socket (`/var/run/cvpn.sock`)
  or a named pipe (`\\.\pipe\cvpn`), open to root, administrators and the user
  who installed it. That is why sudo isn't needed.
- **Cisco** is `openconnect` in split-tunnel mode: only the subnets sent by the
  server go into the tunnel. The Cisco gateway gets a route through the
  physical network up front. Otherwise, with VLESS on, the TLS session to the
  gateway starts through the proxy and then, once vpnc-script adds its route,
  continues from a different IP, and the session breaks (`SSL read error`,
  reconnect, drop).
- **DNS**: the stock vpnc-script takes over DNS for the whole system and public
  domains stop resolving. `cvpn` takes DNS away from it and does split DNS:
  only corporate domains (from the settings and those sent by the server) go
  to corporate DNS. macOS uses `/etc/resolver/<domain>`, Linux
  systemd-resolved, Windows NRPT rules.
- **VLESS** is `sing-box` in tun mode: all traffic, no per-application setup.
  The tun address `198.18.0.1/30` doesn't overlap with corporate networks.
- **WireGuard / AmneziaWG** use `wg-quick` / `awg-quick` on macOS and Linux and
  the official client's tunnel service on Windows (`/installtunnelservice`).
- **Linux servers**: replies to inbound connections (SSH) and anything with a
  specific route (Cisco tunnel, local network) bypass VLESS, so turning vless
  on over SSH doesn't cut you off. The provider's public DNS servers are also
  reached directly: many don't answer queries from another IP.

Files:

| | macOS / Linux | Windows |
|---|---|---|
| program | `/usr/local/lib/cvpn` | `C:\Program Files\cvpn` |
| settings | `/usr/local/etc/cvpn` (`config.json`, `singbox.json`, `cvpn-wg.conf`, `cvpn-awg.conf`) | `C:\ProgramData\cvpn` |
| logs | `/var/log/cvpn-*.log` | `C:\ProgramData\cvpn\log` |

`server/` holds the older bash version for servers where sing-box runs as its
own systemd unit and `singbox.json` is edited by hand. See
[README.ru.md](README.ru.md) (Russian) for details. On such a server
`install.sh` refuses to run.

## Limitations

- VLESS: `tcp` transport only, `security=reality|tls|none`.
- Linux: split DNS works only if `/etc/resolv.conf` points at
  systemd-resolved (the installer warns you). Debian/Ubuntu only.
- Linux tray icon: GTK 4 and WebKitGTK 6.0. Prebuilt binaries are built on the
  latest Ubuntu LTS and may not start on older systems; build from source
  there, or use the CLI with `CVPN_GUI=0`.
- Windows: openconnect is stopped without its disconnect script (a service has
  no console for Ctrl+C); the wintun adapter and its routes disappear with the
  process, the service removes the NRPT rules and the gateway route.
- The Cisco password and one-time code are sent as a "password, then code"
  form, as AnyConnect does with two-factor authentication. If you don't need a
  code, leave the field empty.
- Binaries are not signed or notarized.

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md), including how to test without a Cisco
gateway. Report vulnerabilities as described in [SECURITY.md](SECURITY.md).

## Uninstall

```
./uninstall.sh                                            # macOS, Linux
powershell -ExecutionPolicy Bypass -File uninstall.ps1    # Windows, as administrator
```

## Trademarks

This project is not affiliated with or endorsed by Cisco Systems. Cisco and
AnyConnect are trademarks of Cisco Systems, Inc.; here they only name the
protocol and gateway the program works with. The project contains no Cisco
code: it connects through [OpenConnect](https://www.infradead.org/openconnect/).
