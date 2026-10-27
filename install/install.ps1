# Detect Windows architecture, verify the release checksum, then launch Inno Setup.
$ErrorActionPreference = 'Stop'
$version = if ($env:FRACTURE_VERSION) { $env:FRACTURE_VERSION } else { 'v0.1.10' }
if ($version -notmatch '^v[0-9]+\.[0-9]+\.[0-9]+$') { throw 'Invalid release version' }
[Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12
$archName = if ($env:PROCESSOR_ARCHITEW6432) { $env:PROCESSOR_ARCHITEW6432 } else { $env:PROCESSOR_ARCHITECTURE }
$arch = switch ($archName) { 'AMD64' { 'amd64' } 'ARM64' { 'arm64' } default { throw 'FractureOS requires Windows x64 or ARM64' } }
$base = "https://github.com/sepisotoni/FractureOS-v3/releases/download/$version"
$name = "fracture-$version-windows-$arch-setup.exe"
$stage = Join-Path ([IO.Path]::GetTempPath()) ('fracture-' + [Guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $stage | Out-Null
try {
    Invoke-WebRequest -UseBasicParsing "$base/$name" -OutFile (Join-Path $stage $name)
    Invoke-WebRequest -UseBasicParsing "$base/SHA256SUMS" -OutFile (Join-Path $stage 'SHA256SUMS')
    $entries = @(Get-Content (Join-Path $stage 'SHA256SUMS') | Where-Object { $_ -match ('^[a-f0-9]{64}  ' + [regex]::Escape($name) + '$') })
    if ($entries.Count -ne 1) { throw 'Missing or ambiguous release checksum' }
    $actual = (Get-FileHash (Join-Path $stage $name) -Algorithm SHA256).Hash.ToLowerInvariant()
    if ($actual -ne $entries[0].Substring(0,64)) { throw 'Release checksum mismatch' }
    $process = Start-Process -FilePath (Join-Path $stage $name) -Wait -PassThru
    if ($process.ExitCode -ne 0) { throw "Setup exited with code $($process.ExitCode)" }
} finally {
    Remove-Item -LiteralPath $stage -Recurse -Force
}
