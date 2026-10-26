# FractureOS per-user Windows installer; run from Windows PowerShell 5.1+.
$ErrorActionPreference = 'Stop'
$version = if ($env:FRACTURE_VERSION) { $env:FRACTURE_VERSION } else { 'v0.1.5' }
if ($version -notmatch '^v[0-9]+\.[0-9]+\.[0-9]+$') { throw 'Invalid release version' }
Add-Type -AssemblyName System.Windows.Forms
Add-Type -AssemblyName System.Drawing
[Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12
$archName = if ($env:PROCESSOR_ARCHITEW6432) { $env:PROCESSOR_ARCHITEW6432 } else { $env:PROCESSOR_ARCHITECTURE }
$arch = switch ($archName) { 'AMD64' { 'amd64' } 'ARM64' { 'arm64' } default { throw 'FractureOS requires 64-bit Windows (x64 or ARM64)' } }
$root = Join-Path $env:LOCALAPPDATA 'FractureOS-v3'
$base = "https://github.com/sepisotoni/FractureOS-v3/releases/download/$version"
$form = New-Object Windows.Forms.Form
$form.Text = 'Install FractureOS'
$form.Size = New-Object Drawing.Size(560,300)
$form.StartPosition = 'CenterScreen'
$form.FormBorderStyle = 'FixedDialog'
$form.MaximizeBox = $false
$label = New-Object Windows.Forms.Label
$label.Location = New-Object Drawing.Point(24,24)
$label.Size = New-Object Drawing.Size(500,135)
$label.Text = "FractureOS $version`r`n`r`nInstall the lightweight tray app for your current account.`r`nConfigure your Linux hub and clock later in Settings.`r`n`r`nAn uninstaller will be added to Windows Installed Apps."
$button = New-Object Windows.Forms.Button
$button.Text = 'Install'
$button.Location = New-Object Drawing.Point(360,185)
$button.Size = New-Object Drawing.Size(140,38)
$form.Controls.AddRange(@($label,$button))
$button.Add_Click({
    $button.Enabled = $false
    $stage = Join-Path ([IO.Path]::GetTempPath()) ('fracture-' + [Guid]::NewGuid().ToString('N'))
    try {
        $label.Text = 'Downloading and verifying FractureOS...'
        $form.Refresh()
        if ((Test-Path $root) -and !(Test-Path (Join-Path $root '.fracture-owned'))) { throw 'Existing unmanaged installation directory; refusing to overwrite it.' }
        New-Item -ItemType Directory -Path $stage | Out-Null
        $archive = "fracture-$version-windows-$arch.zip"
        Invoke-WebRequest -UseBasicParsing "$base/$archive" -OutFile (Join-Path $stage $archive)
        Invoke-WebRequest -UseBasicParsing "$base/SHA256SUMS" -OutFile (Join-Path $stage 'SHA256SUMS')
        $entries = @(Get-Content (Join-Path $stage 'SHA256SUMS') | Where-Object { $_ -match ('^[a-f0-9]{64}  ' + [regex]::Escape($archive) + '$') })
        if ($entries.Count -ne 1) { throw 'Missing or ambiguous release checksum' }
        $expected = $entries[0].Substring(0,64)
        if ((Get-FileHash (Join-Path $stage $archive) -Algorithm SHA256).Hash.ToLowerInvariant() -ne $expected) { throw 'Release checksum mismatch' }
        $unpack = Join-Path $stage 'unpack'
        # Inspect archive entries before extraction; only these three files are shipped.
        Add-Type -AssemblyName System.IO.Compression.FileSystem
        $zip = [IO.Compression.ZipFile]::OpenRead((Join-Path $stage $archive))
        try { foreach ($entry in $zip.Entries) { if ($entry.FullName -notin @('fracture.exe','uninstall.ps1','README.txt')) { throw 'Unexpected archive entry' } } } finally { $zip.Dispose() }
        Expand-Archive -LiteralPath (Join-Path $stage $archive) -DestinationPath $unpack
        if (!(Test-Path (Join-Path $unpack 'fracture.exe'))) { throw 'Missing executable' }
        New-Item -ItemType Directory -Force -Path $root | Out-Null
        'FractureOS v3 managed installation' | Set-Content (Join-Path $root '.fracture-owned')
        # Stop only processes whose executable belongs to this managed installation.
        Get-Process -Name fracture -ErrorAction SilentlyContinue | ForEach-Object {
            if ($_.Path -eq (Join-Path $root 'fracture.exe')) { Stop-Process -Id $_.Id -ErrorAction Stop }
        }
        foreach ($name in @('fracture.exe','uninstall.ps1','README.txt')) { Copy-Item -LiteralPath (Join-Path $unpack $name) -Destination (Join-Path $root $name) -Force }
        $shell = New-Object -ComObject WScript.Shell
        $shortcutPath = Join-Path ([Environment]::GetFolderPath('Programs')) 'FractureOS.lnk'
        $shortcut = $shell.CreateShortcut($shortcutPath)
        $shortcut.TargetPath = Join-Path $root 'fracture.exe'
        $shortcut.WorkingDirectory = $root
        $shortcut.Save()
        $reg = 'HKCU:\Software\Microsoft\Windows\CurrentVersion\Uninstall\FractureOS-v3'
        New-Item -Path $reg -Force | Out-Null
        New-ItemProperty $reg DisplayName -Value 'FractureOS v3' -Force | Out-Null
        New-ItemProperty $reg DisplayVersion -Value $version -Force | Out-Null
        New-ItemProperty $reg InstallLocation -Value $root -Force | Out-Null
        New-ItemProperty $reg UninstallString -Value ('powershell.exe -NoProfile -ExecutionPolicy Bypass -File "' + (Join-Path $root 'uninstall.ps1') + '"') -Force | Out-Null
        New-ItemProperty $reg NoModify -PropertyType DWord -Value 1 -Force | Out-Null
        New-ItemProperty $reg NoRepair -PropertyType DWord -Value 1 -Force | Out-Null
        Start-Process (Join-Path $root 'fracture.exe') -ArgumentList 'tray'
        $label.Text = "Installed successfully.`r`n`r`nUse the tray icon to open Settings and enter your hub connection.`r`nOpen FractureOS from the Start menu on your next login.`r`n`r`nNo resource collection is enabled by default."
        $button.Text = 'Installed'
    } catch {
        $label.Text = 'Installation failed. No device firmware was changed.'
        [Windows.Forms.MessageBox]::Show($_.Exception.Message,'FractureOS') | Out-Null
        $button.Enabled = $true
    } finally {
        if (Test-Path $stage) { Remove-Item -LiteralPath $stage -Recurse -Force }
    }
})
[void]$form.ShowDialog()
