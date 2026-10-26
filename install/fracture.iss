; Build with Inno Setup 6.3+ on Windows. No configuration wizard: use app Settings.
#ifndef AppVersion
#define AppVersion "0.1.9"
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
[Icons]
Name: "{group}\FractureOS Dashboard"; Filename: "{app}\fracture.exe"; Parameters: "dashboard"
Name: "{group}\FractureOS Tray"; Filename: "{app}\fracture.exe"; Parameters: "tray"
Name: "{group}\Uninstall FractureOS"; Filename: "{uninstallexe}"
[Tasks]
Name: "startup"; Description: "Start the tray app when I sign in"; Flags: unchecked
[Registry]
Root: HKCU; Subkey: "Software\Microsoft\Windows\CurrentVersion\Run"; ValueType: string; ValueName: "FractureOS"; ValueData: """{app}\fracture.exe"" tray"; Tasks: startup; Flags: uninsdeletevalue
[Run]
Filename: "{app}\fracture.exe"; Parameters: "tray"; Description: "Start FractureOS tray"; Flags: nowait postinstall skipifsilent
Filename: "{app}\fracture.exe"; Parameters: "dashboard"; Description: "Open dashboard and settings"; Flags: nowait postinstall skipifsilent
; Config stays in %APPDATA%\fracture-v3 across upgrades/uninstallation.
