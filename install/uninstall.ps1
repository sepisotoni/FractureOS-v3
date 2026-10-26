$ErrorActionPreference = 'Stop'
$root = Join-Path $env:LOCALAPPDATA 'FractureOS-v3'
if ($PSScriptRoot -ne $root -or !(Test-Path (Join-Path $root '.fracture-owned'))) { throw 'Managed installation not found' }
Get-Process -Name fracture -ErrorAction SilentlyContinue | ForEach-Object {
    if ($_.Path -eq (Join-Path $root 'fracture.exe')) { Stop-Process -Id $_.Id -ErrorAction Stop }
}
$shortcut = Join-Path ([Environment]::GetFolderPath('Programs')) 'FractureOS.lnk'
if (Test-Path $shortcut) { Remove-Item -LiteralPath $shortcut }
$reg = 'HKCU:\Software\Microsoft\Windows\CurrentVersion\Uninstall\FractureOS-v3'
if (Test-Path $reg) { Remove-Item -LiteralPath $reg -Recurse }
# The PowerShell process is outside the app folder and can remove its script.
Remove-Item -LiteralPath $root -Recurse -Force
Add-Type -AssemblyName System.Windows.Forms
[Windows.Forms.MessageBox]::Show('FractureOS removed. User settings and hub pairing credentials were retained.','FractureOS') | Out-Null
