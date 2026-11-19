# Install the dna CLI from the latest GitHub release (Windows).
#
#   irm https://raw.githubusercontent.com/domainreseller/cli/main/scripts/install.ps1 | iex
#
# Environment:
#   DNA_VERSION      version to install, e.g. 0.2.0 (default: latest)
#   DNA_INSTALL_DIR  target directory (default: %LOCALAPPDATA%\Programs\dna)
$ErrorActionPreference = 'Stop'
$repo = 'domainreseller/cli'

$arch = switch ($env:PROCESSOR_ARCHITECTURE) {
    'AMD64' { 'amd64' }
    'ARM64' { 'arm64' }
    default { throw "Unsupported architecture: $env:PROCESSOR_ARCHITECTURE" }
}

$version = $env:DNA_VERSION
if (-not $version) {
    $release = Invoke-RestMethod -Uri "https://api.github.com/repos/$repo/releases/latest" -Headers @{ 'User-Agent' = 'dna-installer' }
    $version = $release.tag_name
}
$version = $version.TrimStart('v')

$archive = "dna_${version}_windows_${arch}.zip"
$base = "https://github.com/$repo/releases/download/v$version"
$tmp = Join-Path ([System.IO.Path]::GetTempPath()) ("dna-install-" + [guid]::NewGuid())
New-Item -ItemType Directory -Path $tmp | Out-Null

try {
    Write-Host "Downloading dna $version (windows/$arch)..."
    Invoke-WebRequest -Uri "$base/$archive" -OutFile (Join-Path $tmp $archive) -UseBasicParsing
    Invoke-WebRequest -Uri "$base/checksums.txt" -OutFile (Join-Path $tmp 'checksums.txt') -UseBasicParsing

    $line = Get-Content (Join-Path $tmp 'checksums.txt') | Where-Object { $_ -match " $([regex]::Escape($archive))$" } | Select-Object -First 1
    if (-not $line) { throw "$archive is not listed in checksums.txt" }
    $expected = ($line -split '\s+')[0].ToLower()
    $actual = (Get-FileHash -Algorithm SHA256 (Join-Path $tmp $archive)).Hash.ToLower()
    if ($expected -ne $actual) { throw "Checksum mismatch for $archive" }

    Expand-Archive -Path (Join-Path $tmp $archive) -DestinationPath $tmp -Force

    $dir = $env:DNA_INSTALL_DIR
    if (-not $dir) { $dir = Join-Path $env:LOCALAPPDATA 'Programs\dna' }
    New-Item -ItemType Directory -Path $dir -Force | Out-Null
    $target = Join-Path $dir 'dna.exe'
    # Copies left by earlier updates; still-running ones stay until they exit.
    Get-ChildItem -Path $dir -Filter 'dna.exe.old*' -ErrorAction SilentlyContinue | Remove-Item -Force -ErrorAction SilentlyContinue
    try {
        Copy-Item (Join-Path $tmp 'dna.exe') $target -Force
    }
    catch [System.IO.IOException] {
        # A running dna.exe (an MCP server started by Claude, Cursor or another AI
        # app, or an open dna shell) cannot be overwritten, but Windows lets us
        # rename it. Move it aside and put the new version in its place; the
        # running copies keep working until they are restarted.
        $old = Join-Path $dir ('dna.exe.old-' + [guid]::NewGuid().ToString('N').Substring(0, 8))
        Move-Item -Path $target -Destination $old -Force
        Copy-Item (Join-Path $tmp 'dna.exe') $target -Force
        Write-Host 'dna was running (for example as an MCP server of an AI app). The new version is installed;'
        Write-Host 'restart those apps (or start a new session) to use it.'
    }

    $userPath = [Environment]::GetEnvironmentVariable('Path', 'User')
    if (-not (($userPath -split ';') -contains $dir)) {
        [Environment]::SetEnvironmentVariable('Path', ($userPath.TrimEnd(';') + ';' + $dir), 'User')
        Write-Host "Added $dir to your user PATH (open a new terminal to use it)."
    }
    & (Join-Path $dir 'dna.exe') version
    Write-Host "Installed to $dir\dna.exe"
    Write-Host 'Next: dna auth login --env ote'
}
finally {
    Remove-Item -Recurse -Force $tmp -ErrorAction SilentlyContinue
}
