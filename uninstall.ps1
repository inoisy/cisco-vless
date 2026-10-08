# Удаление cvpn на Windows (от администратора). OpenConnect-GUI, WireGuard
# и AmneziaWG не трогает.

$ErrorActionPreference = "Continue"
$Lib = Join-Path $env:ProgramFiles "cvpn"
$Etc = Join-Path $env:ProgramData "cvpn"

if (Test-Path "$Lib\cvpn.exe") {
	& "$Lib\cvpn.exe" off
	& "$Lib\cvpn.exe" service uninstall
}
Get-Process cvpn-gui -ErrorAction SilentlyContinue | Stop-Process -Force

# Правила split-DNS, если openconnect не успел их снять.
Get-DnsClientNrptRule | Where-Object { $_.Comment -eq "cvpn" } |
	ForEach-Object { Remove-DnsClientNrptRule -Name $_.Name -Force }

foreach ($dir in @([Environment]::GetFolderPath("Programs"), [Environment]::GetFolderPath("Startup"))) {
	Remove-Item (Join-Path $dir "cvpn.lnk") -ErrorAction SilentlyContinue
}

$path = [Environment]::GetEnvironmentVariable("Path", "Machine")
[Environment]::SetEnvironmentVariable("Path", (($path -split ";") | Where-Object { $_ -ne $Lib }) -join ";", "Machine")

Remove-Item -Recurse -Force $Lib, $Etc -ErrorAction SilentlyContinue
Write-Host "cvpn удалён"
