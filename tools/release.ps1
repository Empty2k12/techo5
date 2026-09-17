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
    [string]$Go = 'go',
    # A rootfs tarball (tools/linux/deploy-rootfs.sh builds one on the device under
    # /data/techo5-linux/); slot devices update from it, and it is published with the release.
    [string]$Rootfs = '',
    # The Echo Dot 2's rootfs tarball (techo5-dot: tools/linux/build-dot-rootfs.ps1). A Dot booting from
    # slots is offered a release only when it carries this; the Dot binary is built every time.
    [string]$DotRootfs = '',
    # The release signing key (ed25519 seed, base64). Devices take a manifest only with its signature, so
    # a release cannot be published without it. Keep it off every repository and backed up: $env:TECHO5_SIGN_KEY.
    [string]$SignKey = $env:TECHO5_SIGN_KEY,
    # A boot image built with build-image.sh --no-key, published as techo5-boot-<version>.img for new
    # units (docs/install.md). Refused if it carries an SSH key.
    [string]$Boot = ''
)
$ErrorActionPreference = 'Stop'
if (-not $SignKey -or -not (Test-Path $SignKey)) { throw "no release signing key: set TECHO5_SIGN_KEY or pass -SignKey" }
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
    Write-Host "== building echod-arm-dot $Version ($commit)"
    & $Go build -tags dot -trimpath -ldflags $ldflags -o (Join-Path $bin 'echod-arm-dot') ./cmd/echod
    if ($LASTEXITCODE -ne 0) { throw 'dot build failed' }
    $env:GOOS = $null; $env:GOARCH = $null; $env:GOARM = $null; $env:CGO_ENABLED = $null

    Write-Host "== manifest"
    $from = "https://github.com/$repo/releases/download/$Version"
    $mk = @('run', './cmd/mkmanifest', '-version', $Version, '-title', "TECHO5 $Version", '-notes', $Notes,
        '-release-url', "https://github.com/$repo/releases/tag/$Version",
        '-from', $from, '-arm', (Join-Path $bin 'echod-arm'), '-arm-dot', (Join-Path $bin 'echod-arm-dot'),
        '-out', (Join-Path $bin 'manifest.json'), '-sign-key', $SignKey)
    if ($Rootfs) { $mk += @('-rootfs-arm', $Rootfs) }
    if ($DotRootfs) { $mk += @('-rootfs-arm-dot', $DotRootfs) }
    & $Go @mk
    if ($LASTEXITCODE -ne 0) { throw 'mkmanifest failed' }
    Get-Content (Join-Path $bin 'manifest.json')
} finally { Pop-Location }

Write-Host "== release $Version"
$args = @('release', 'create', $Version, (Join-Path $bin 'echod-arm'), (Join-Path $bin 'echod-arm-dot'), (Join-Path $bin 'manifest.json'),
    (Join-Path $bin 'manifest.json.sig'), '--repo', $repo, '--title', $Version, '--notes', $Notes)
if ($Rootfs) { $args += $Rootfs }
if ($DotRootfs) { $args += $DotRootfs }
if ($Boot) {
    # An image built without --no-key carries the builder's key in its initramfs; that must not ship.
    # The check is for the file entry (its name ends in a NUL), not the init script that mentions it.
    $py = "import gzip,lzma,struct,sys; b=open(sys.argv[1],'rb').read(); ks,_,rs=struct.unpack('<3I',b[8:20]); ps=struct.unpack('<I',b[36:40])[0]; r0=ps+((ks+ps-1)//ps)*ps; r=b[r0:r0+rs]; d=gzip.decompress(r) if r[:2]==b'\x1f\x8b' else lzma.decompress(r); sys.exit(1 if b'root/.ssh/authorized_keys'+bytes(1) in d else 0)"
    python -c $py $Boot
    if ($LASTEXITCODE -ne 0) { throw "$Boot carries an SSH key: build it with build-image.sh --no-key" }
    $named = Join-Path $bin "techo5-boot-$Version.img"
    Copy-Item $Boot $named -Force
    $args += $named
}
if ($Prerelease) { $args += '--prerelease' }
& gh @args
if ($LASTEXITCODE -ne 0) { throw 'gh release create failed' }
Write-Host "published: https://github.com/$repo/releases/tag/$Version"
