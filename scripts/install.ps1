# Multigent installer for Windows.
#
# Recommended:
#   irm https://raw.githubusercontent.com/multigent/multigent/main/scripts/install.ps1 | iex
#
# Options:
#   $env:MULTIGENT_BIN_DIR="$env:USERPROFILE\.multigent\bin"
#   $env:MULTIGENT_VERSION="v0.1.0"

$ErrorActionPreference = "Stop"

$RepoWebUrl = if ($env:MULTIGENT_REPO_WEB_URL) { $env:MULTIGENT_REPO_WEB_URL } else { "https://github.com/multigent/multigent" }
$BinDir = if ($env:MULTIGENT_BIN_DIR) { $env:MULTIGENT_BIN_DIR } else { Join-Path $env:USERPROFILE ".multigent\bin" }
$Version = if ($env:MULTIGENT_VERSION) { $env:MULTIGENT_VERSION } else { "" }

function Write-Info { param([string]$Msg) Write-Host "==> $Msg" -ForegroundColor Cyan }
function Write-Ok { param([string]$Msg) Write-Host "[OK] $Msg" -ForegroundColor Green }
function Write-Fail { param([string]$Msg) Write-Host "[ERROR] $Msg" -ForegroundColor Red; exit 1 }

function Convert-ToCliArch {
    param([object]$Value)
    if ($null -eq $Value) { return $null }
    $normalized = "$Value".Trim().ToUpperInvariant()
    switch ($normalized) {
        "9" { return "amd64" }
        "AMD64" { return "amd64" }
        "X64" { return "amd64" }
        "X86_64" { return "amd64" }
        "12" { return "arm64" }
        "ARM64" { return "arm64" }
        "AARCH64" { return "arm64" }
        default { return $null }
    }
}

function Get-WindowsCliArch {
    $signals = @()
    try {
        if (Get-Command Get-CimInstance -ErrorAction SilentlyContinue) {
            $signals += [pscustomobject]@{
                Source = "Win32_Processor.Architecture"
                Value = (Get-CimInstance -ClassName Win32_Processor -ErrorAction Stop | Select-Object -First 1 -ExpandProperty Architecture)
            }
        }
    } catch {}
    try {
        $signals += [pscustomobject]@{
            Source = "RuntimeInformation.OSArchitecture"
            Value = [System.Runtime.InteropServices.RuntimeInformation]::OSArchitecture
        }
    } catch {}
    $signals += [pscustomobject]@{ Source = "PROCESSOR_ARCHITEW6432"; Value = $env:PROCESSOR_ARCHITEW6432 }
    $signals += [pscustomobject]@{ Source = "PROCESSOR_ARCHITECTURE"; Value = $env:PROCESSOR_ARCHITECTURE }

    foreach ($signal in $signals) {
        $arch = Convert-ToCliArch $signal.Value
        if ($arch) { return $arch }
    }
    Write-Fail "Unsupported Windows architecture. Multigent supports x64 and ARM64."
}

function Get-LatestVersion {
    if ($Version) { return $Version }
    try {
        $release = Invoke-RestMethod -Uri "https://api.github.com/repos/multigent/multigent/releases/latest" -ErrorAction Stop
        return $release.tag_name
    } catch {
        Write-Fail "Could not determine latest release. Check your network connection."
    }
}

if (-not [Environment]::Is64BitOperatingSystem) {
    Write-Fail "Multigent requires a 64-bit Windows installation."
}

$arch = Get-WindowsCliArch
$tag = Get-LatestVersion
$archive = "multigent-$tag-windows-$arch.zip"
$url = "$RepoWebUrl/releases/download/$tag/$archive"
$tmpDir = Join-Path ([System.IO.Path]::GetTempPath()) "multigent-install"

Write-Info "Target release: $tag (windows/$arch)"
Write-Info "Downloading $url ..."

if (Test-Path $tmpDir) { Remove-Item $tmpDir -Recurse -Force }
New-Item -ItemType Directory -Path $tmpDir | Out-Null

try {
    Invoke-WebRequest -Uri $url -OutFile (Join-Path $tmpDir $archive) -UseBasicParsing
} catch {
    Remove-Item $tmpDir -Recurse -Force
    Write-Fail "Failed to download $archive`: $_"
}

Expand-Archive -Force (Join-Path $tmpDir $archive) $tmpDir
New-Item -ItemType Directory -Path $BinDir -Force | Out-Null
Copy-Item (Join-Path $tmpDir "multigent.exe") (Join-Path $BinDir "multigent.exe") -Force
Copy-Item (Join-Path $tmpDir "mga.exe") (Join-Path $BinDir "mga.exe") -Force
Remove-Item $tmpDir -Recurse -Force

$userPath = [Environment]::GetEnvironmentVariable("Path", "User")
if (-not ($userPath -split ';' | Where-Object { $_ -eq $BinDir })) {
    [Environment]::SetEnvironmentVariable("Path", "$BinDir;$userPath", "User")
    Write-Host "Added $BinDir to your user PATH. Restart your shell if 'multigent' is not found." -ForegroundColor Yellow
}

Write-Ok "Installed multigent and mga to $BinDir"
Write-Ok "Done. Run: multigent start --dir .\multigent-data --open"
