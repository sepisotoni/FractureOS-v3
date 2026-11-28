; Build with Inno Setup 6.3+ on Windows. No configuration wizard: use app Settings.
#ifndef AppVersion
#define AppVersion "0.1.18.1"
#endif
#ifndef Arch
#define Arch "amd64"
#endif
[Setup]
AppId={{FB5074DB-A706-41A1-9C2C-E1981BD3A6B9}
AppName=FractureOS
AppVersion={#AppVersion}
AppPublisher=sepisotoni
AppPublisherURL=https://github.com/sepisotoni/FractureOS-v3
DefaultDirName={localappdata}\Programs\FractureOS
DefaultGroupName=FractureOS
PrivilegesRequired=lowest
DisableProgramGroupPage=yes
WizardStyle=modern
SetupIconFile=..\assets\branding\fracture.ico
UninstallDisplayIcon={app}\fracture.exe
OutputDir=..\build\inno
OutputBaseFilename=fracture-v{#AppVersion}-windows-{#Arch}-setup
Compression=lzma2
SolidCompression=yes
MinVersion=10.0.17763
CloseApplications=yes
CloseApplicationsFilter=fracture.exe
RestartApplications=no
#if Arch == "arm64"
ArchitecturesAllowed=arm64
ArchitecturesInstallIn64BitMode=arm64
#else
ArchitecturesAllowed=x64compatible
ArchitecturesInstallIn64BitMode=x64compatible
#endif
[Files]
Source: "..\build\windows-{#Arch}\fracture.exe"; DestDir: "{app}"; Flags: ignoreversion
Source: "..\build\windows-{#Arch}\fracture-preview.dll"; DestDir: "{app}"; Flags: ignoreversion
Source: "..\assets\wallpaper-packs\*"; DestDir: "{app}\assets\wallpaper-packs"; Flags: ignoreversion recursesubdirs createallsubdirs
[InstallDelete]
Type: files; Name: "{group}\FractureOS Tray.lnk"
Type: files; Name: "{group}\FractureOS Dashboard.lnk"
Type: files; Name: "{group}\FractureOS Settings.lnk"
Type: files; Name: "{group}\Uninstall FractureOS.lnk"
Type: files; Name: "{userprograms}\FractureOS Tray.lnk"
Type: files; Name: "{userprograms}\FractureOS Dashboard.lnk"
Type: files; Name: "{userprograms}\FractureOS Settings.lnk"
Type: files; Name: "{userprograms}\Uninstall FractureOS.lnk"
Type: files; Name: "{userdesktop}\FractureOS Tray.lnk"
Type: files; Name: "{userdesktop}\FractureOS Dashboard.lnk"
[Icons]
Name: "{group}\FractureOS"; Filename: "{app}\fracture.exe"; Parameters: "dashboard"
[Tasks]
Name: "startup"; Description: "Start the tray app when I sign in"; Flags: unchecked
[Registry]
Root: HKCU; Subkey: "Software\Microsoft\Windows\CurrentVersion\Run"; ValueType: string; ValueName: "FractureOS"; ValueData: """{app}\fracture.exe"" tray"; Tasks: startup; Flags: uninsdeletevalue
[Run]
Filename: "{app}\fracture.exe"; Parameters: "tray"; Description: "Start FractureOS tray"; Flags: nowait postinstall skipifsilent
Filename: "{app}\fracture.exe"; Parameters: "dashboard"; Description: "Open dashboard and settings"; Flags: nowait postinstall skipifsilent
; Config stays in %APPDATA%\fracture-v3 across upgrades/uninstallation.

[Code]
// Setup's Restart Manager cannot always close every fracture.exe (tray, dashboard,
// web and helper processes), which caused "DeleteFile failed; code 5". Stop only
// the copies that run from this install folder, then wait for them to exit.
function StopFractureProcesses(const AppDir: String): Boolean;
var
  ResultCode: Integer;
  Command, AppExe: String;
begin
  AppExe := AddBackslash(AppDir) + 'fracture.exe';
  StringChangeEx(AppExe, '''', '''''', True);
  Command := '-NoProfile -NonInteractive -ExecutionPolicy Bypass -Command "' +
    '$ErrorActionPreference = ''Stop''; ' +
    '$running = @(Get-Process -Name fracture -ErrorAction SilentlyContinue | ' +
    'Where-Object { [String]::Equals($_.Path, ''' + AppExe + ''', [StringComparison]::OrdinalIgnoreCase) }); ' +
    'foreach ($process in $running) { if (!$process.HasExited) { try { $process.Kill() } catch { if (!$process.HasExited) { throw } } }; ' +
    'if (!$process.WaitForExit(5000)) { exit 1 } }"';
  Result := Exec('powershell.exe', Command, '', SW_HIDE, ewWaitUntilTerminated, ResultCode);
  if Result then Result := ResultCode = 0;
end;

function PrepareToInstall(var NeedsRestart: Boolean): String;
begin
  if StopFractureProcesses(ExpandConstant('{app}')) then Result := ''
  else Result := 'Close FractureOS from the system tray, then retry Setup.';
end;

function InitializeUninstall(): Boolean;
begin
  Result := StopFractureProcesses(ExpandConstant('{app}'));
  if not Result then MsgBox('Close FractureOS from the system tray, then run uninstall again.', mbError, MB_OK);
end;
