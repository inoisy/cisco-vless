# Security

## Reporting a vulnerability

Please don't open a public issue. Use
[Report a vulnerability](https://github.com/inoisy/cisco-vless/security/advisories/new)
(GitHub private reporting). Include the version (`cvpn version`), the OS and
steps to reproduce; leave out real gateway addresses, logins, links and keys.

This is a volunteer project with no guaranteed response time. Only the latest
release gets fixes.

## What to know

- The service runs as root / SYSTEM. It answers root, administrators and the
  user who installed it. **Treat that user as equivalent to root**: the settings
  they can change include the paths of programs the service runs.
- Secrets: the Cisco password is in the Keychain on macOS and in a root-only
  file on Linux and Windows. Links, subscriptions and WireGuard / AmneziaWG
  configs are in files readable only by root / SYSTEM. One-time codes are not
  stored.
- The Cisco server certificate is pinned: you confirm its fingerprint once and
  openconnect refuses a different one.
- The installers download sing-box, and cvpn itself when Go is not installed,
  from GitHub over HTTPS and don't verify checksums or signatures. Each release
  has `SHA256SUMS`, and you can build from source.
- With `ru_direct` on, cvpn downloads a geoip list from GitHub, and DNS queries
  for Russian zones go to Yandex unencrypted, outside the tunnel.
- Removing your corporate VPN client may break your employer's rules. cvpn
  doesn't run posture checks and doesn't try to pass them.

## Scope

openconnect, sing-box, WireGuard and AmneziaWG are separate programs: report
their vulnerabilities to their authors. Anything in how cvpn launches or
configures them belongs here.
