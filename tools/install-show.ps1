<#
.SYNOPSIS
  One command from an Echo Show 5 (2nd gen, cronos) running LineageOS 18.1 to TECHO5 Linux running from a
  slot: named, keyed for Home Assistant, on the Wi-Fi LineageOS already had, with Bluetooth.

.DESCRIPTION
  docs/install.md, automated, each step checked before the next:

    1. checks    adb sees the unit as cronos on the LineageOS kernel TECHO5's is built from
    2. backup    with rooted debugging on, LineageOS's boot image into backups/<serial>/ (the way back)
    3. release   the latest signed release (or -Release): the boot image and the root filesystem, each
                 checked against its checksum
    4. push      the root filesystem onto the unit's storage, checked by md5
    5. flash     the boot image, from the bootloader's fastboot
    6. store     in the rescue environment, over the USB serial console: LineageOS's system partition
                 becomes the slot store (THIS ERASES LINEAGEOS), the root filesystem goes into slot a,
                 and the name, the Home Assistant key and an SSH key (-SshKey) are provisioned
    7. watch     the first boot from slot a to a running daemon

  Runs on Windows, Linux and macOS with PowerShell 7 (pwsh). Needs adb and fastboot; nothing is built.
  The release's boot image is the same for every Show: LineageOS's kernel rebuilt with Bluetooth, and
  TECHO5's rescue environment, with no SSH key inside.

  The Home Assistant key is kept in backups/<serial>/api.psk (git-ignored), and an existing one is reused,
  so Home Assistant keeps the device.

  Undo: TWRP stays in recovery. Boot it, flash the LineageOS zip, and flash backups/<serial>/boot-lineage.img
  (or the boot image from the LineageOS zip) to boot.

.EXAMPLE
  ./tools/install-show.ps1 -Serial <serial> -Name Kitchen
  ./tools/install-show.ps1 -Serial <serial> -Name Kitchen -SshKey ~/.ssh/id_ed25519.pub
#>
param(
    [Parameter(Mandatory)][string]$Serial,
    [Parameter(Mandatory)][string]$Name,
    # A release tag, or latest.
    [string]$Release = 'latest',
    [string]$KeyFile,
    # An SSH public key the unit accepts from the start (SSH is switched on with it).
    [string]$SshKey,
    # A boot image of your own (docs/building.md) instead of the release's.
    [string]$Boot,
    # Download and check the release and stop; nothing is written to the unit.
    [switch]$DryRun,
    # Do not ask before erasing LineageOS's system partition.
    [switch]$Force,
    [string]$BackupRoot = $(if ($env:TECHO5_BACKUPS) { $env:TECHO5_BACKUPS } else { Join-Path (Join-Path $PSScriptRoot '..') 'backups' }),
    [string]$WorkDir = $(if ($env:TECHO5_WORK) { $env:TECHO5_WORK } else { Join-Path (Join-Path $PSScriptRoot '..') 'build' }),
    [string]$Adb = 'adb',
    [string]$Fastboot = 'fastboot'
)
$ErrorActionPreference = 'Stop'
$ProgressPreference = 'SilentlyContinue'
$Releases = 'https://github.com/HuskerMinion/techo5/releases'
# The LineageOS kernel commit TECHO5's kernel is rebuilt from: the vendor modules only load on it.
$KernelRelease = '4.9.337-g8d928c5176cc'
$BackupRoot = $ExecutionContext.SessionState.Path.GetUnresolvedProviderPathFromPSPath($BackupRoot)
$WorkDir = $ExecutionContext.SessionState.Path.GetUnresolvedProviderPathFromPSPath($WorkDir)
$backup = Join-Path $BackupRoot $Serial
$console = Join-Path $PSScriptRoot 'serial-console.ps1'
if (-not $KeyFile) { $KeyFile = Join-Path $backup 'api.psk' }

function Step([string]$what) { Write-Host "== $what" -ForegroundColor Cyan }
function Note([string]$what) { Write-Host "   $what" }
function Need([string]$exe) { if (-not (Get-Command $exe -ErrorAction SilentlyContinue)) { throw "$exe not found (install the Android platform tools)" } }
function AdbSh([string]$cmd) { (& $Adb -s $Serial shell $cmd) -join "`n" }
function Md5Of([string]$path) { (Get-FileHash -Algorithm MD5 $path).Hash.ToLower() }
function Sha256Of([string]$path) { (Get-FileHash -Algorithm SHA256 $path).Hash.ToLower() }
function Show([string]$cmd, [int]$waitMs = 8000) {
    $o = & $console -Serial $Serial -Cmd $cmd -WaitMs $waitMs -Port $global:Techo5ConsolePort
    if ($null -eq $o) { return $null }
    return [string]$o
}
function WaitFor([string]$what, [int]$seconds, [scriptblock]$test) {
    $deadline = (Get-Date).AddSeconds($seconds)
    while ((Get-Date) -lt $deadline) {
        if (& $test) { return }
        Start-Sleep -Seconds 5
    }
    throw "timed out after $seconds s waiting for $what"
}
function Get-Checked([string]$url, [string]$out, [string]$sha256) {
    if ((Test-Path $out) -and (Sha256Of $out) -eq $sha256) { return }
    Invoke-WebRequest -Uri $url -OutFile "$out.partial" -UseBasicParsing
    $got = Sha256Of "$out.partial"
    if ($got -ne $sha256) { Remove-Item -Force "$out.partial"; throw "$url does not match its checksum ($got, wanted $sha256)" }
    Move-Item -Force "$out.partial" $out
}
# Text for the rescue shell, single-quoted.
function Quote([string]$s) { "'" + ($s -replace "'", "'\''") + "'" }

# ------------------------------------------------------------------------------------------ 1. checks
Step 'checks'
foreach ($exe in $Adb, $Fastboot) { Need $exe }
if ($Name -match "[`r`n]") { throw 'the name must be one line' }
if ($SshKey -and -not (Test-Path $SshKey)) { throw "no SSH public key at $SshKey" }
$state = (& $Adb -s $Serial get-state 2>$null)
if ($state -ne 'device') { throw "adb does not see $Serial running LineageOS (state '$state'): turn on USB debugging and accept this computer" }
$dev = (AdbSh 'getprop ro.product.device').Trim()
if ($dev -ne 'cronos') { throw "$Serial reports '$dev', not cronos (an Echo Show 5 2nd gen)" }
$kr = (AdbSh 'uname -r').Trim()
if ($kr -ne $KernelRelease) { throw "$Serial runs kernel ${kr}, not ${KernelRelease}: install the LineageOS 18.1 build the getting started guide links" }
Note "cronos, LineageOS kernel $kr"

# ------------------------------------------------------------------------------------------ 2. backup
Step 'backup'
New-Item -ItemType Directory -Force $backup, $WorkDir | Out-Null
$losBoot = Join-Path $backup 'boot-lineage.img'
if (Test-Path $losBoot) {
    Note "LineageOS boot image already kept: $losBoot"
} else {
    & $Adb -s $Serial root 2>$null | Out-Null; Start-Sleep -Seconds 3; & $Adb -s $Serial wait-for-device
    if ((AdbSh 'id') -match '^uid=0') {
        & $Adb -s $Serial pull /dev/block/mmcblk0p9 "$losBoot.partial" | Out-Null
        $want = (AdbSh 'md5sum /dev/block/mmcblk0p9').Split(' ')[0]
        if ((Md5Of "$losBoot.partial") -ne $want) { throw 'LineageOS boot image md5 mismatch' }
        Move-Item "$losBoot.partial" $losBoot
        Note "LineageOS boot image: $losBoot"
        $wifi = (AdbSh 'grep -c SSID /data/misc/apexdata/com.android.wifi/WifiConfigStore.xml 2>/dev/null').Trim()
        if ($wifi -eq '' -or $wifi -eq '0') { throw 'LineageOS has no saved Wi-Fi network: join one first (TECHO5 uses it)' }
        Note 'a saved Wi-Fi network'
    } else {
        Note 'adb is not root (Rooted debugging off): no boot image backup; the LineageOS zip has one'
        Note 'make sure LineageOS is on your Wi-Fi: TECHO5 joins the network it saved'
    }
}

# ------------------------------------------------------------------------------------------ 3. release
Step 'the release'
$dl = if ($Release -eq 'latest') { "$Releases/latest/download" } else { "$Releases/download/$Release" }
$manifest = Invoke-RestMethod -Uri "$dl/manifest.json" -UseBasicParsing
$Version = $manifest.version
$rel = Join-Path $WorkDir "show-release-$Version"
New-Item -ItemType Directory -Force $rel | Out-Null
if (-not $manifest.rootfs.arm) { throw "release $Version has no root filesystem for the Show" }
$rootfsName = Split-Path -Leaf ([uri]$manifest.rootfs.arm.url).AbsolutePath
$rootfs = Join-Path $rel $rootfsName
Get-Checked $manifest.rootfs.arm.url $rootfs $manifest.rootfs.arm.sha256
Note "root filesystem $rootfsName checked against the signed manifest"
if ($Boot) {
    if (-not (Test-Path $Boot)) { throw "no boot image at $Boot" }
    $bootImg = (Resolve-Path $Boot).Path
    Note "boot image: your own, $bootImg"
} else {
    $sums = @{}
    $text = (Invoke-WebRequest -Uri "$dl/SHA256SUMS" -UseBasicParsing).Content
    if ($text -is [byte[]]) { $text = [Text.Encoding]::ASCII.GetString($text) }
    foreach ($line in ($text -split "`n")) { if ($line -match '^([0-9a-f]{64})\s+\*?(\S+)') { $sums[$Matches[2]] = $Matches[1] } }
    $bootName = "techo5-boot-$Version.img"
    if (-not $sums[$bootName]) { throw "release $Version has no $bootName in SHA256SUMS; pick another with -Release, or pass -Boot" }
    $bootImg = Join-Path $rel $bootName
    Get-Checked "$dl/$bootName" $bootImg $sums[$bootName]
    Note "boot image $bootName checked"
}
if ($DryRun) {
    Write-Host "Dry run: TECHO5 $Version downloaded and checked in $rel; nothing written to the unit."
    return
}

# ------------------------------------------------------------------------------------------ 4. push
Step 'root filesystem onto the unit'
$remote = "/sdcard/Download/$rootfsName"
& $Adb -s $Serial push $rootfs $remote | Out-Null
$got = (AdbSh "md5sum $remote").Split(' ')[0]
if ($got -ne (Md5Of $rootfs)) { throw "md5 mismatch after pushing $rootfsName" }
Note "$remote ok"

New-Item -ItemType Directory -Force (Split-Path $KeyFile) | Out-Null
if (Test-Path $KeyFile) {
    $psk = (Get-Content $KeyFile -Raw).Trim()
    Note "Home Assistant key: the existing one in $KeyFile"
} else {
    $rnd = [byte[]]::new(32); [Security.Cryptography.RandomNumberGenerator]::Create().GetBytes($rnd)
    $psk = [Convert]::ToBase64String($rnd)
    [IO.File]::WriteAllText($KeyFile, $psk)
    Note "Home Assistant key: new, in $KeyFile"
}

# ------------------------------------------------------------------------------------------- 5. flash
Step 'flash the boot image'
& $Adb -s $Serial reboot bootloader
WaitFor 'fastboot' 90 { (& $Fastboot devices) -match [regex]::Escape($Serial) }
& $Fastboot -s $Serial flash boot $bootImg
if ($LASTEXITCODE -ne 0) { throw 'fastboot flash boot failed' }
& $Fastboot -s $Serial continue
Note 'booting the rescue environment (no slot store yet)'
$nudged = $false
WaitFor 'the rescue console' 300 {
    $o = Show 'test -e /run/techo5/slot || echo RESCUE-UP' 4000
    if ($o -match 'RESCUE-UP') { return $true }
    # A leftover bootloader request can stop the boot at "hacked fastboot" once; continue it.
    if (-not $nudged -and ((& $Fastboot devices) -match [regex]::Escape($Serial))) {
        Start-Sleep -Seconds 20
        if ((& $Fastboot devices) -match [regex]::Escape($Serial)) {
            & $Fastboot -s $Serial continue | Out-Null
            $script:nudged = $true
        }
    }
    return $false
}
Note "rescue console on $global:Techo5ConsolePort"

# ------------------------------------------------------------------------------------------- 6. store
Step 'slot store'
if (-not $Force) {
    Write-Host "   Next: LineageOS's system partition (mmcblk0p12) is erased and becomes the slot store." -ForegroundColor Yellow
    if ((Read-Host '   Type ERASE to go on') -ne 'ERASE') { throw 'stopped before erasing; the unit stays in rescue (flash the LineageOS boot image to go back)' }
}
$tar = "/data/media/0/Download/$rootfsName"
$o = Show "umount /android 2>/dev/null; slotctl mkstore /dev/mmcblk0p12 --i-know-this-erases-it >/tmp/mkstore.log 2>&1 && echo MKSTORE-OK; tail -3 /tmp/mkstore.log" 300000
if ($o -notmatch 'MKSTORE-OK') { throw "mkstore failed:`n$o" }
$o = Show "STORE=/store slotctl install $tar >/tmp/install.log 2>&1 && echo INSTALL-OK; tail -2 /tmp/install.log; STORE=/store slotctl status" 900000
if ($o -notmatch 'INSTALL-OK') { throw "slot install failed:`n$o" }
Note ($o -split "`n" | Where-Object { $_ -match '^slot a' })

$prov = "mkdir -p /data/misc/techo5 && printf '%s\n' $(Quote $Name) > /data/misc/techo5/name && (umask 077; printf '%s\n' $(Quote $psk) > /data/misc/techo5/psk)"
if ($SshKey) {
    $pub = (Get-Content $SshKey -Raw).Trim()
    if ($pub -notmatch '^(ssh|ecdsa|sk)-') { throw "$SshKey does not look like an SSH public key" }
    $prov += " && mkdir -p -m 700 /data/misc/techo5/ssh && (umask 077; printf '%s\n' $(Quote $pub) > /data/misc/techo5/ssh/authorized_keys)"
    $prov += " && { [ -e /data/misc/techo5/state.json ] || printf '{`"security`":{`"ssh`":true}}\n' > /data/misc/techo5/state.json; }"
}
$o = Show "$prov && sync && echo PROV-OK" 15000
if ($o -notmatch 'PROV-OK') { throw "provisioning failed:`n$o" }
Note "name '$Name' and Home Assistant key$(if ($SshKey) { ', SSH key' }) provisioned"
Show 'sync; (sleep 2; /bin/busybox.static reboot -f) >/dev/null 2>&1 &' 3000 | Out-Null

# ------------------------------------------------------------------------------------------- 7. watch
Step 'first boot'
$nudged = $false
WaitFor 'slot a with the daemon running' 300 {
    $o = Show 'echo slot=$(cat /run/techo5/slot 2>/dev/null)- daemon=$(pidof techo5)-' 4000
    if ($o -match 'slot=a- daemon=\d') { return $true }
    if (-not $nudged -and ((& $Fastboot devices) -match [regex]::Escape($Serial))) {
        & $Fastboot -s $Serial continue | Out-Null
        $script:nudged = $true
    }
    return $false
}
$o = Show 'ip -4 addr show wlan0 | sed -n "s/.*inet \([0-9.]*\).*/\1/p"; cat /etc/techo5-release' 8000
Note ($o -replace "`n", "`n   ")

Write-Host ''
Write-Host "Done. '$Name' runs TECHO5 $Version from slot a; the slot commits itself after five healthy minutes." -ForegroundColor Green
Write-Host "Home Assistant finds it as an ESPHome device; the key is in $KeyFile."
Write-Host 'Later versions arrive through Home Assistant''s update card.'
