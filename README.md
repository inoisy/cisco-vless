# cvpn — Cisco AnyConnect вместе с VLESS, WireGuard или AmneziaWG

Корпоративный Cisco AnyConnect и личный VPN одновременно: рабочая сеть идёт
через Cisco, всё остальное — через VLESS, WireGuard или AmneziaWG.
macOS, Linux и Windows; командная строка и значок в трее.

Обычно так не получается. Cisco Secure Client, Hiddify, V2Box, Amnezia и
прочие клиенты перехватывают маршруты и DNS каждый под себя, и после второго
подключения отваливается либо работа, либо всё остальное. `cvpn` заменяет
их одной программой, в которой порядок включения не важен.

```
cvpn on 123456     # vless + cisco (123456 — одноразовый код, если нужен)
cvpn               # статус
cvpn off           # выключить всё
```

Или значок в трее: включить/выключить Cisco и выход, окно с настройками и
логами.

## Установка

Нужно: адрес сервера Cisco, логин и пароль; ссылка `vless://...` и/или
конфиг WireGuard / AmneziaWG.

**macOS** ([Homebrew](https://brew.sh)) и **Linux** (Debian/Ubuntu с systemd):

```
git clone https://github.com/inoisy/cisco-vless && cd cisco-vless
./install.sh
```

**Windows** — сначала поставь [OpenConnect-GUI](https://gui.openconnect-vpn.net/download/)
(из него нужен `openconnect.exe` с драйвером wintun), затем в PowerShell от
администратора:

```
git clone https://github.com/inoisy/cisco-vless; cd cisco-vless
powershell -ExecutionPolicy Bypass -File install.ps1
```

Если стоит Go — установщик соберёт `cvpn` из исходников, иначе скачает
релиз. Дальше он ставит службу `cvpn`, значок в трее (с автозапуском) и
спрашивает настройки: сервер Cisco, логин, группу, корп-домены, ссылку VLESS,
конфиги WireGuard / AmneziaWG. Отпечаток сертификата Cisco показывает на
подтверждение. Пароль Cisco на macOS хранится в Keychain, на Linux и
Windows — в файле, который читает только root / SYSTEM.

Настройки потом меняются в окне «Настройки» или `cvpn setup`.

Выходы, кроме VLESS, ставятся отдельно:

| | macOS | Linux | Windows |
|---|---|---|---|
| WireGuard | `CVPN_WG=1 ./install.sh` | `CVPN_WG=1 ./install.sh` | [клиент WireGuard](https://www.wireguard.com/install/) |
| AmneziaWG | `CVPN_AWG=1 ./install.sh` (сборка из исходников) | `CVPN_AWG=1 ./install.sh` (ppa:amnezia/ppa) | [клиент AmneziaWG](https://github.com/amnezia-vpn/amneziawg-windows-client/releases) |

Cisco Secure Client и другие VPN-клиенты перед работой выключи: `cvpn status`
и окно предупредят, если они подключены.

Обновление с bash-версии — тот же `./install.sh`: старая выключится,
настройки и пароль подхватятся, sudo для `cvpn` больше не нужен.

## Команды

| команда | что делает |
|---|---|
| `cvpn` | статус: что включено, внешний IP, резолвится ли корп-хост |
| `cvpn on [vless\|wg\|awg] [otp]` | выход (по умолчанию vless) + cisco |
| `cvpn off` | выключить всё |
| `cvpn cisco on [otp]` / `off` | только корп-сеть |
| `cvpn vless on` / `off` | весь остальной трафик через VLESS |
| `cvpn wg on` / `off` | весь остальной трафик через WireGuard |
| `cvpn awg on` / `off` | весь остальной трафик через AmneziaWG |
| `cvpn boot vless\|wg\|awg\|off` | какой выход поднимать при старте системы |
| `cvpn setup` | настройки |
| `cvpn password` | сменить пароль Cisco |
| `cvpn log` | логи cisco, vless и службы |
| `cvpn service install\|uninstall\|restart` | служба (sudo / администратор) |

cisco совместим с любым выходом; vless, wg и awg — взаимоисключающие:
включение одного выключает другой.

## Как это устроено

- **Служба** (`cvpn daemon`: launchd / systemd / служба Windows) работает с
  правами root/SYSTEM и держит openconnect и sing-box дочерними процессами.
  CLI и значок в трее — её клиенты: говорят с ней по unix-сокету
  (`/var/run/cvpn.sock`) или named pipe (`\\.\pipe\cvpn`), доступ — у root,
  администраторов и пользователя, который ставил. Поэтому sudo не нужен.
- **Cisco** — `openconnect` в режиме split-tunnel: в туннель идут только
  подсети, которые прислал сервер. Шлюз Cisco заранее получает маршрут через
  физическую сеть. Иначе при включённом VLESS TLS-сессия с шлюзом поднимется
  через прокси, а после маршрута от vpnc-script уйдёт другим путём с другого
  IP — и сессия рвётся (`SSL read error`, переподключение, обрыв).
- **DNS** — штатный vpnc-script перехватывает DNS всей системы, и публичные
  домены перестают резолвиться. `cvpn` отбирает у него DNS и делает
  split-DNS: в корп-DNS идут только корп-домены (из настроек и присланные
  сервером). macOS — `/etc/resolver/<домен>`, Linux — systemd-resolved,
  Windows — правила NRPT.
- **VLESS** — `sing-box` в tun-режиме: весь трафик без настройки программ.
  Адрес tun `198.18.0.1/30` не пересекается с корп-сетями.
- **WireGuard / AmneziaWG** — `wg-quick` / `awg-quick` на macOS и Linux,
  служба туннеля официального клиента на Windows (`/installtunnelservice`).
- **Linux-сервер** — ответы на входящие соединения (SSH) и всё, у чего есть
  точный маршрут (туннель Cisco, локальная сеть), идут мимо VLESS, поэтому
  включение vless по SSH не отрезает доступ к серверу. Публичные DNS-серверы
  провайдера тоже идут напрямую: многие не отвечают на запросы с чужого IP.

Файлы:

| | macOS / Linux | Windows |
|---|---|---|
| программа | `/usr/local/lib/cvpn` | `C:\Program Files\cvpn` |
| настройки | `/usr/local/etc/cvpn` (`config.json`, `singbox.json`, `cvpn-wg.conf`, `cvpn-awg.conf`) | `C:\ProgramData\cvpn` |
| логи | `/var/log/cvpn-*.log` | `C:\ProgramData\cvpn\log` |

## Ограничения

- VLESS: транспорт только `tcp`, `security=reality|tls|none`.
- Linux: split-DNS работает, только если `/etc/resolv.conf` смотрит в
  systemd-resolved (установщик предупредит).
- Windows: openconnect останавливается без disconnect-скрипта (у службы нет
  консоли для Ctrl+C); адаптер wintun и его маршруты уходят вместе с
  процессом, правила NRPT и маршрут к шлюзу снимает служба.
- Пароль Cisco + одноразовый код передаются формой «пароль, затем код» —
  как у AnyConnect с двухфакторкой. Если код не нужен — оставь поле пустым.

## Разработка

```
go test ./...
go build ./cmd/cvpn ./cmd/cvpn-gui
```

Без root, в песочнице: `CVPN_ROOT=/tmp/cvpn ./cvpn daemon` — все пути
(сокет, настройки, логи) под `CVPN_ROOT`, CLI и GUI с той же переменной
говорят с этой копией. Окно без webview, в обычном браузере:
`CVPN_ROOT=/tmp/cvpn ./cvpn-gui -serve 127.0.0.1:8765`.

GUI — [Wails v3](https://v3.wails.io) (Go + webview): на macOS нужен Xcode
Command Line Tools, на Linux — `libgtk-3-dev libwebkit2gtk-4.1-dev`, на
Windows собирается без cgo (`-ldflags -H=windowsgui`). Сборки под все ОС —
`.github/workflows/build.yml`, релиз — по тегу `v*`.

## Удаление

```
./uninstall.sh                                            # macOS, Linux
powershell -ExecutionPolicy Bypass -File uninstall.ps1    # Windows, от администратора
```
