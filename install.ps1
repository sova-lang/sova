<#
.SYNOPSIS
    Sova installer for Windows.

.DESCRIPTION
    Downloads the latest Sova release archive from GitHub, extracts the
    compiler binary plus the bundled stdlib into %LOCALAPPDATA%\sova, and
    adds that directory to the current user's PATH. Re-running upgrades
    an existing installation in-place.

.EXAMPLE
    iwr -useb https://raw.githubusercontent.com/sova-lang/sova/main/install.ps1 | iex

.EXAMPLE
    $env:SOVA_VERSION = 'v1.2.3'; iwr -useb https://raw.githubusercontent.com/sova-lang/sova/main/install.ps1 | iex
#>

[CmdletBinding()]
param(
    [string]$Version    = $env:SOVA_VERSION,
    [string]$Repo       = $(if ($env:SOVA_REPO) { $env:SOVA_REPO } else { 'sova-lang/sova' }),
    [string]$InstallDir = $(if ($env:SOVA_INSTALL_DIR) { $env:SOVA_INSTALL_DIR } else { Join-Path $env:LOCALAPPDATA 'sova' })
)

$ErrorActionPreference = 'Stop'
[Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12

function Write-Step($msg)  { Write-Host "==> $msg" -ForegroundColor Cyan }
function Write-Warn2($msg) { Write-Host "!! $msg"  -ForegroundColor Yellow }
function Write-Die($msg)   { Write-Host "xx $msg"  -ForegroundColor Red; exit 1 }

function Resolve-Arch {
    $arch = (Get-CimInstance Win32_Processor | Select-Object -First 1).Architecture
    # 9 = x64, 12 = arm64
    switch ($arch) {
        9  { return 'x64' }
        12 { return 'arm64' }
    }
    # Fallback: trust PROCESSOR_ARCHITECTURE
    switch ($env:PROCESSOR_ARCHITECTURE) {
        'AMD64' { return 'x64' }
        'ARM64' { return 'arm64' }
        default { Write-Die "unsupported processor architecture: $($env:PROCESSOR_ARCHITECTURE)" }
    }
}

function Resolve-Version($repo, $requested) {
    if ($requested) { return $requested }
    Write-Step "resolving latest release from github.com/$repo"
    $api = "https://api.github.com/repos/$repo/releases/latest"
    try {
        $rel = Invoke-RestMethod -Uri $api -Headers @{ 'Accept' = 'application/vnd.github+json' } -UserAgent 'sova-install'
    } catch {
        Write-Die "failed to query GitHub releases: $($_.Exception.Message)"
    }
    if (-not $rel.tag_name) { Write-Die 'GitHub response did not include a tag_name' }
    return $rel.tag_name
}

function Download-Asset($url, $dest) {
    Write-Step "downloading $([IO.Path]::GetFileName($url))"
    try {
        Invoke-WebRequest -Uri $url -OutFile $dest -UserAgent 'sova-install' -UseBasicParsing
    } catch {
        Write-Die "download failed: $($_.Exception.Message)"
    }
}

function Install-Archive($archive, $destDir) {
    Write-Step "extracting into $destDir"
    if (-not (Test-Path $destDir)) {
        New-Item -ItemType Directory -Path $destDir -Force | Out-Null
    }
    $stdPath = Join-Path $destDir 'std'
    $exePath = Join-Path $destDir 'sova.exe'
    if (Test-Path $stdPath) { Remove-Item $stdPath -Recurse -Force }
    if (Test-Path $exePath) {
        try {
            Remove-Item $exePath -Force
        } catch {
            Write-Die "could not remove existing sova.exe (close any running instance and retry): $($_.Exception.Message)"
        }
    }
    Expand-Archive -Path $archive -DestinationPath $destDir -Force
}

function Update-UserPath($dir) {
    $current = [Environment]::GetEnvironmentVariable('Path', 'User')
    if (-not $current) { $current = '' }
    $parts = $current -split ';' | Where-Object { $_ -and ($_ -ne $dir) }
    $parts = ,$dir + $parts
    $new = ($parts -join ';')
    if ($new -ne $current) {
        [Environment]::SetEnvironmentVariable('Path', $new, 'User')
        Write-Step "added $dir to user PATH"
    } else {
        Write-Step "user PATH already contains $dir"
    }
    if (-not ($env:Path -split ';' | Where-Object { $_ -eq $dir })) {
        $env:Path = "$dir;$env:Path"
    }
}

function Verify-Install($exe, $fallbackVersion) {
    if (-not (Test-Path $exe)) {
        Write-Die "post-install check failed: $exe is missing"
    }
    try {
        $v = & $exe version --short 2>$null
    } catch { $v = $fallbackVersion }
    if (-not $v) { $v = $fallbackVersion }
    Write-Step "installed: $v"
    Write-Step "location:  $exe"
}

# ---- main ----

$arch       = Resolve-Arch
$resolvedV  = Resolve-Version $Repo $Version
$asset      = "sova-win-$arch.zip"
$url        = "https://github.com/$Repo/releases/download/$resolvedV/$asset"

Write-Step "target: win/$arch @ $resolvedV"

$tmp = Join-Path ([IO.Path]::GetTempPath()) ("sova-install-" + [Guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $tmp -Force | Out-Null
try {
    $archive = Join-Path $tmp $asset
    Download-Asset $url $archive
    Install-Archive $archive $InstallDir
} finally {
    Remove-Item $tmp -Recurse -Force -ErrorAction SilentlyContinue
}

Update-UserPath $InstallDir
Verify-Install (Join-Path $InstallDir 'sova.exe') $resolvedV

Write-Host ""
Write-Host "Sova $resolvedV installed." -ForegroundColor Green
Write-Host "Open a new terminal (or restart VS Code) so the updated PATH takes effect, then run: sova --help"
