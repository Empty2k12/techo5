<#
.SYNOPSIS
  Download the public build inputs (Alpine base, packages, busybox, apk.static, wake word models) into
  inputs/, for building TECHO5's images yourself. The installers don't need this: they use the release.

.DESCRIPTION
  What it fetches, over HTTPS from Alpine's CDN and GitHub:
    alpine-minirootfs-3.24.1-armv7.tar.gz    Alpine's base image (pinned sha256)
    busybox.static, apk.static                busybox-static (v3.24, armv7) and apk-tools-static 2.14 (v3.22, x86_64)
    models/                                   okay_nabu, hey_jarvis, hey_mycroft, alexa (.tflite + .json)
                                              from esphome/micro-wake-word-models (models/v2)
    show, spot:  apks/, apks312/              tools/linux/packages.txt (the rescue initramfs); the
                                              wpa_supplicant 2.9 set from Alpine v3.12, as the Wi-Fi
                                              drivers need
    spot:        apks/libgcc                  (mkfs.ext4 needs it in the Spot's rescue initramfs)
    dot:         apks-dot/, apks-bt-dot/      techo5-dot's tools/linux/packages-rescue.txt and packages-bt.txt

  A package list names exact versions. Alpine keeps only the newest build of each package, so when the
  listed one is gone the newest is taken and the script says so.

  What it can't fetch, because it comes from your own unit and is never published: the LineageOS boot
  image (Show, Spot) and the recovery backup (Dot; the installer keeps it). docs/building.md says how
  to take each. No vendor tree is needed: each unit keeps its own.

.EXAMPLE
  ./tools/fetch-inputs.ps1 -Device show
  ./tools/fetch-inputs.ps1 -Device dot -Dot ../techo5-dot
#>
param(
    [ValidateSet('show', 'dot', 'spot')][string]$Device = 'show',
    [string]$Out = $(if ($env:TECHO5_INPUTS) { $env:TECHO5_INPUTS } else { Join-Path (Join-Path $PSScriptRoot '..') 'inputs' }),
    # A techo5-dot checkout, for its package lists: ../techo5-dot beside this repository unless set.
    [string]$Dot = (Join-Path (Join-Path (Join-Path $PSScriptRoot '..') '..') 'techo5-dot')
)
$ErrorActionPreference = 'Stop'
$ProgressPreference = 'SilentlyContinue'
$mirror = 'https://dl-cdn.alpinelinux.org/alpine'
$root = Join-Path $PSScriptRoot '..'
New-Item -ItemType Directory -Force $Out | Out-Null
$Out = (Resolve-Path $Out).Path
$tmp = Join-Path $Out '.fetch'
New-Item -ItemType Directory -Force $tmp | Out-Null

function Get-Tar {
    if (-not $IsLinux -and -not $IsMacOS -and $env:SystemRoot) {
        $own = Join-Path (Join-Path $env:SystemRoot 'System32') 'tar.exe'
        if (Test-Path $own) { return $own }
    }
    'tar'
}
$tar = Get-Tar

# Each repository's index, once: name -> version.
$indexes = @{}
function Get-Index([string]$branch, [string]$repo, [string]$arch = 'armv7') {
    $key = "$branch/$repo/$arch"
    if ($indexes[$key]) { return $indexes[$key] }
    $gz = Join-Path $tmp "APKINDEX-$($branch -replace '\W', '')-$repo-$arch.tar.gz"
    Invoke-WebRequest -Uri "$mirror/$branch/$repo/$arch/APKINDEX.tar.gz" -OutFile $gz -UseBasicParsing
    $dir = Join-Path $tmp "idx-$($branch -replace '\W', '')-$repo-$arch"
    New-Item -ItemType Directory -Force $dir | Out-Null
    & $tar -xzf $gz -C $dir APKINDEX
    if ($LASTEXITCODE -ne 0) { throw "unpacking $key's index failed" }
    $map = @{}
    foreach ($block in ([IO.File]::ReadAllText((Join-Path $dir 'APKINDEX')) -split "`n`n")) {
        $p = [regex]::Match($block, '(?m)^P:(.+)$'); $v = [regex]::Match($block, '(?m)^V:(.+)$')
        if ($p.Success -and $v.Success) { $map[$p.Groups[1].Value] = $v.Groups[1].Value }
    }
    $indexes[$key] = $map
    $map
}

# A listed package file (name-version-rN.apk) into $dir: that version when the mirror still has it,
# else the newest.
function Get-Apk([string]$file, [string]$dir, [string]$branch = 'v3.24', [string]$arch = 'armv7') {
    $m = [regex]::Match($file, '^(.+?)-(\d[^-]*-r\d+)\.apk$')
    if (-not $m.Success) { throw "not a package file name: $file" }
    $name, $want = $m.Groups[1].Value, $m.Groups[2].Value
    foreach ($repo in 'main', 'community') {
        $have = (Get-Index $branch $repo $arch)[$name]
        if (-not $have) { continue }
        if ($have -ne $want) { Write-Host "   $name $want is gone from $branch; taking $have" -ForegroundColor Yellow }
        $target = Join-Path $dir "$name-$have.apk"
        if (-not (Test-Path $target)) {
            Invoke-WebRequest -Uri "$mirror/$branch/$repo/$arch/$name-$have.apk" -OutFile "$target.partial" -UseBasicParsing
            Move-Item -Force "$target.partial" $target
        }
        return $target
    }
    throw "$name is in neither main nor community of Alpine $branch"
}

function Get-List([string]$list, [string]$dir) {
    New-Item -ItemType Directory -Force $dir | Out-Null
    foreach ($line in Get-Content $list) {
        $a = ($line -replace '#.*', '').Trim()
        if (-not $a) { continue }
        $branch = 'v3.24'; $into = $dir
        # The Wi-Fi drivers need wpa_supplicant 2.9, which lives in Alpine 3.12 with its libraries.
        if ($a -match '^(wpa_supplicant-2\.9|libssl1\.1|libcrypto1\.1|libnl3-3\.5)') { $branch = 'v3.12'; $into = Join-Path $Out 'apks312' }
        if ($a -like 'busybox-static-*') { continue }
        New-Item -ItemType Directory -Force $into | Out-Null
        Get-Apk $a $into $branch | Out-Null
    }
}

Write-Host "== Alpine base"
$alpine = Join-Path $Out 'alpine-minirootfs-3.24.1-armv7.tar.gz'
$alpineSha = '50942d567e6ee422c16cb46d5c282ed9d8adc9007c2a483faf4148a18c64ce32'
if (-not (Test-Path $alpine) -or (Get-FileHash -Algorithm SHA256 $alpine).Hash.ToLower() -ne $alpineSha) {
    Invoke-WebRequest -Uri "$mirror/v3.24/releases/armv7/alpine-minirootfs-3.24.1-armv7.tar.gz" -OutFile "$alpine.partial" -UseBasicParsing
    if ((Get-FileHash -Algorithm SHA256 "$alpine.partial").Hash.ToLower() -ne $alpineSha) { throw 'the Alpine base image does not match its checksum' }
    Move-Item -Force "$alpine.partial" $alpine
}

Write-Host "== busybox.static, apk.static"
# busybox for the device (armv7); apk.static for the Linux build host (x86_64), the 2.x tools mkrootfs.sh uses.
foreach ($pair in @(@('busybox-static-1.37.0-r31.apk', 'bin/busybox.static', 'busybox.static', 'v3.24', 'armv7'), @('apk-tools-static-2.14.12-r0.apk', 'sbin/apk.static', 'apk.static', 'v3.22', 'x86_64'))) {
    $apk = Get-Apk $pair[0] $tmp $pair[3] $pair[4]
    $x = Join-Path $tmp 'x'
    New-Item -ItemType Directory -Force $x | Out-Null
    & $tar -xzf $apk -C $x $pair[1] 2>$null
    if ($LASTEXITCODE -ne 0 -or -not (Test-Path (Join-Path $x $pair[1]))) { throw "no $($pair[1]) in $apk" }
    Copy-Item -Force (Join-Path $x $pair[1]) (Join-Path $Out $pair[2])
}

Write-Host "== wake word models"
$models = Join-Path $Out 'models'
New-Item -ItemType Directory -Force $models | Out-Null
foreach ($m in 'okay_nabu', 'hey_jarvis', 'hey_mycroft', 'alexa') {
    foreach ($e in 'tflite', 'json') {
        $f = Join-Path $models "$m.$e"
        if (-not (Test-Path $f)) {
            Invoke-WebRequest -Uri "https://raw.githubusercontent.com/esphome/micro-wake-word-models/main/models/v2/$m.$e" -OutFile $f -UseBasicParsing
        }
    }
}

Write-Host "== packages for the $Device"
switch ($Device) {
    { $_ -in 'show', 'spot' } {
        Get-List (Join-Path (Join-Path (Join-Path $root 'tools') 'linux') 'packages.txt') (Join-Path $Out 'apks')
    }
    'spot' { Get-Apk 'libgcc-15.2.0-r5.apk' (Join-Path $Out 'apks') | Out-Null }
    'dot' {
        $lists = Join-Path (Join-Path $Dot 'tools') 'linux'
        if (-not (Test-Path (Join-Path $lists 'packages-rescue.txt'))) { throw "no techo5-dot checkout at ${Dot}: pass -Dot" }
        Get-List (Join-Path $lists 'packages-rescue.txt') (Join-Path $Out 'apks-dot')
        Get-List (Join-Path $lists 'packages-bt.txt') (Join-Path $Out 'apks-bt-dot')
    }
}
Remove-Item -Recurse -Force $tmp
Write-Host "Inputs are in $Out."
switch ($Device) {
    'dot' { Write-Host "Still to come from your own unit or builds: see techo5-dot's docs/building.md." }
    default { Write-Host "Still to come from your own unit, for a boot image: the LineageOS boot image (docs/building.md)." }
}
