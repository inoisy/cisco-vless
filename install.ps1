# Установка cvpn на Windows: Cisco AnyConnect (openconnect) + VLESS /
# WireGuard / AmneziaWG. Запускать в PowerShell от администратора:
#
#   powershell -ExecutionPolicy Bypass -File install.ps1
#
# Нужен openconnect для Windows: ставится вместе с OpenConnect-GUI
# (https://gui.openconnect-vpn.net/download/) — в нём openconnect.exe,
# драйвер wintun и vpnc-script-win.js. WireGuard и AmneziaWG — их
# официальные клиенты (нужны, только если пользуешься этими выходами).
#
# Параметры:
#   -Version vX.Y   версия релиза cvpn, если собирать нечем (по умолчанию latest)
#   -NoGui          без значка в трее
#   -NoSetup        не запускать cvpn setup

param(
	[string]$Version = "",
	[switch]$NoGui,
	[switch]$NoSetup
)

$ErrorActionPreference = "Stop"
$Repo = "inoisy/cisco-vless"
$SingBoxVersion = "1.14.1"
$Lib = Join-Path $env:ProgramFiles "cvpn"

function Say($msg) { Write-Host "`n== $msg" }
function Die($msg) { Write-Host "Ошибка: $msg" -ForegroundColor Red; exit 1 }

$admin = ([Security.Principal.WindowsPrincipal][Security.Principal.WindowsIdentity]::GetCurrent()).
	IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)
if (-not $admin) { Die "запусти PowerShell от администратора" }

$arch = if ($env:PROCESSOR_ARCHITECTURE -eq "ARM64") { "arm64" } else { "amd64" }
Set-Location $PSScriptRoot

# --- openconnect -----------------------------------------------------------

$oc = @("$env:ProgramFiles\OpenConnect-GUI\openconnect.exe",
	"${env:ProgramFiles(x86)}\OpenConnect-GUI\openconnect.exe",
	"$env:ProgramFiles\OpenConnect\openconnect.exe") | Where-Object { Test-Path $_ } | Select-Object -First 1
if (-not $oc) {
	Write-Host "Не найден openconnect.exe. Поставь OpenConnect-GUI:" -ForegroundColor Yellow
	Write-Host "  https://gui.openconnect-vpn.net/download/"
	Write-Host "или укажи путь к openconnect.exe позже в настройках cvpn."
} else {
	Write-Host "openconnect: $oc"
}

# --- cvpn: собрать или скачать ----------------------------------------------

$tmp = Join-Path $env:TEMP ("cvpn-" + [guid]::NewGuid())
New-Item -ItemType Directory $tmp | Out-Null

if ((Get-Command go -ErrorAction SilentlyContinue) -and (Test-Path go.mod)) {
	Say "Сборка cvpn"
	$env:CGO_ENABLED = "0"
	go build -trimpath -ldflags "-s -w" -o "$tmp\cvpn.exe" ./cmd/cvpn
	if ($LASTEXITCODE) { Die "сборка cvpn" }
	if (-not $NoGui) {
		go build -trimpath -ldflags "-s -w -H=windowsgui" -o "$tmp\cvpn-gui.exe" ./cmd/cvpn-gui
		if ($LASTEXITCODE) { Die "сборка cvpn-gui" }
	}
} else {
	$rel = if ($Version) { "download/$Version" } else { "latest/download" }
	Say "Загрузка cvpn ($rel, windows_$arch)"
	Invoke-WebRequest "https://github.com/$Repo/releases/$rel/cvpn_windows_$arch.exe" -OutFile "$tmp\cvpn.exe"
	if (-not $NoGui) {
		Invoke-WebRequest "https://github.com/$Repo/releases/$rel/cvpn-gui_windows_$arch.exe" -OutFile "$tmp\cvpn-gui.exe"
	}
}

# --- sing-box ----------------------------------------------------------------

if (-not (Test-Path "$Lib\sing-box.exe")) {
	Say "sing-box $SingBoxVersion"
	$zip = "$tmp\sing-box.zip"
	Invoke-WebRequest "https://github.com/SagerNet/sing-box/releases/download/v$SingBoxVersion/sing-box-$SingBoxVersion-windows-$arch.zip" -OutFile $zip
	Expand-Archive $zip "$tmp\sb"
	New-Item -ItemType Directory -Force $Lib | Out-Null
	Copy-Item (Get-ChildItem "$tmp\sb" -Recurse -Filter sing-box.exe | Select-Object -First 1).FullName "$Lib\sing-box.exe"
}

# --- установка ---------------------------------------------------------------

Say "Файлы: $Lib"
if (Get-Service cvpn -ErrorAction SilentlyContinue) {
	& "$Lib\cvpn.exe" service stop 2>$null
}
Get-Process cvpn-gui -ErrorAction SilentlyContinue | Stop-Process -Force
New-Item -ItemType Directory -Force $Lib | Out-Null
Copy-Item "$tmp\cvpn*.exe" $Lib -Force

$path = [Environment]::GetEnvironmentVariable("Path", "Machine")
if (($path -split ";") -notcontains $Lib) {
	[Environment]::SetEnvironmentVariable("Path", "$path;$Lib", "Machine")
	$env:Path += ";$Lib"
}

Say "Служба cvpn"
# Владелец — кто ставит: ему служба отвечает по named pipe без прав админа.
$env:CVPN_OWNER = [Security.Principal.WindowsIdentity]::GetCurrent().Name
& "$Lib\cvpn.exe" service install
if ($LASTEXITCODE) { Die "служба не установилась" }

if (-not $NoGui) {
	Say "Значок в трее"
	$shell = New-Object -ComObject WScript.Shell
	foreach ($dir in @([Environment]::GetFolderPath("Programs"), [Environment]::GetFolderPath("Startup"))) {
		$lnk = $shell.CreateShortcut((Join-Path $dir "cvpn.lnk"))
		$lnk.TargetPath = "$Lib\cvpn-gui.exe"
		$lnk.Description = "Cisco + VLESS / WireGuard / AmneziaWG"
		$lnk.Save()
	}
	Start-Process "$Lib\cvpn-gui.exe"
}

Remove-Item -Recurse -Force $tmp

if (-not $NoSetup) {
	Say "Настройки"
	& "$Lib\cvpn.exe" setup
}

Say "Готово"
Write-Host "  cvpn on [otp]          vless + cisco"
Write-Host "  cvpn off               выключить всё"
Write-Host "  cvpn                   статус"
Write-Host "  cvpn cisco|vless|wg|awg on|off"
Write-Host "Новый PATH подхватится в новых окнах терминала."
