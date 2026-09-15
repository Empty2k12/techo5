<#
.SYNOPSIS
  Build a versioned daemon, write the update manifest, and publish a GitHub release.

.DESCRIPTION
  The daemon's built-in updater fetches
  https://github.com/HuskerMinion/techo5/releases/latest/download/manifest.json (stable) and
  installs the binary it names. This script produces both files and publishes them with gh.

.EXAMPLE
  .\tools\release.ps1 -Version v0.1.0 -Notes "First release: voice satellite on the Echo Show 5."
  .\tools\release.ps1 -Version v0.1.1 -Notes "..." -Prerelease
#>
param(
    [Parameter(Mandatory)][ValidatePattern('^v\d+\.\d+\.\d+(-[0-9A-Za-z.]+)?$')][string]$Version,
    [Parameter(Mandatory)][string]$Notes,
    [switch]$Prerelease,
    [string]$Go = 'go'
)
$ErrorActionPreference = 'Stop'
$repo = 'HuskerMinion/techo5'
$root = Resolve-Path (Join-Path $PSScriptRoot '..')
$bin = Join-Path $root 'bin'
New-Item -ItemType Directory -Force $bin | Out-Null

Push-Location (Join-Path $root 'echod')
try {
    $commit = (git rev-parse --short HEAD).Trim()
    $date = (Get-Date).ToUniversalTime().ToString('yyyy-MM-ddTHH:mm:ssZ')
    $pkg = 'github.com/HuskerMinion/techo5/echod/internal/layout'
    $ldflags = "-s -w -X '$pkg.Version=$Version' -X '$pkg.GitCommit=$commit' -X '$pkg.BuildDate=$date'"

    Write-Host "== building echod-arm $Version ($commit)"
    $env:GOOS = 'linux'; $env:GOARCH = 'arm'; $env:GOARM = '7'; $env:CGO_ENABLED = '0'
    & $Go build -trimpath -ldflags $ldflags -o (Join-Path $bin 'echod-arm') ./cmd/echod
    if ($LASTEXITCODE -ne 0) { throw 'build failed' }
    $env:GOOS = $null; $env:GOARCH = $null; $env:GOARM = $null; $env:CGO_ENABLED = $null

    Write-Host "== manifest"
    $from = "https://github.com/$repo/releases/download/$Version"
    & $Go run ./cmd/mkmanifest -version $Version -title "TECHO5 $Version" -notes $Notes `
        -release-url "https://github.com/$repo/releases/tag/$Version" `
        -from $from -arm (Join-Path $bin 'echod-arm') -out (Join-Path $bin 'manifest.json')
    if ($LASTEXITCODE -ne 0) { throw 'mkmanifest failed' }
    Get-Content (Join-Path $bin 'manifest.json')
} finally { Pop-Location }

Write-Host "== release $Version"
$args = @('release', 'create', $Version, (Join-Path $bin 'echod-arm'), (Join-Path $bin 'manifest.json'),
    '--repo', $repo, '--title', $Version, '--notes', $Notes)
if ($Prerelease) { $args += '--prerelease' }
& gh @args
if ($LASTEXITCODE -ne 0) { throw 'gh release create failed' }
Write-Host "published: https://github.com/$repo/releases/tag/$Version"
