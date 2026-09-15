# Linux image for cronos (porting plan M4)

An Android boot image that carries the LineageOS 4.9.337 kernel and an Alpine
armv7 initramfs instead of Android. Flashed to the `boot` partition it boots to
the TECHO5 daemon with no Android userspace; TWRP stays in `recovery` as the
rescue, and the LineageOS `boot` image restores Android.

State on 2026-09-15: boots in 12 s to a root shell on USB (CDC ACM), joins the
Wi-Fi network Android had saved, starts dropbear and the daemon; a full voice
turn works. Everything still lives in the initramfs (13 MB of the 16 MB slot).

## Build

Inputs (kept out of the repo; see `packages.txt` for the exact Alpine packages):

- `boot-lineage-18.1-20260904-cronos.img` — LineageOS boot image (kernel + header)
- `alpine-minirootfs-3.24.1-armv7.tar.gz`
- `busybox-static-1.37.0-r31.apk` — PID 1 runs on the static busybox
- the packages in `packages.txt`, downloaded from `dl-cdn.alpinelinux.org`
- `bin/fbprobe-arm`, `bin/audioprobe-arm`, `bin/rebootto-arm` (Go, `GOOS=linux GOARCH=arm GOARM=7 CGO_ENABLED=0`)
- an SSH public key for `/root/.ssh/authorized_keys`

```
python3 tools/linux/mkimage.py --kernel-image boot-lineage-18.1-20260904-cronos.img \
  --rootfs alpine-minirootfs-3.24.1-armv7.tar.gz \
  --apk apks312/wpa_supplicant-2.9-r8.apk --apk apks312/libssl1.1-1.1.1o-r0.apk \
  --apk apks312/libcrypto1.1-1.1.1o-r0.apk --apk apks312/libnl3-3.5.0-r0.apk \
  --apk apks/dropbear-2026.91-r0.apk --apk apks/zlib-1.3.2-r0.apk \
  --apk apks/utmps-libs-0.1.3.3-r0.apk --apk apks/skalibs-libs-2.15.0.0-r0.apk \
  --apk apks/iw-6.17-r0.apk --apk apks/wireless-tools-30_pre9-r5.apk --apk apks/wireless-tools-libs-30_pre9-r5.apk \
  --init tools/linux/init \
  --add busybox.static=/bin/busybox.static \
  --add bin/fbprobe-arm=/usr/local/bin/fbprobe --add bin/audioprobe-arm=/usr/local/bin/audioprobe \
  --add bin/rebootto-arm=/usr/local/bin/rebootto \
  --copy techo5_ed25519.pub=/root/.ssh/authorized_keys \
  --compress xz --cmdline-append techo5=linux -o techo5-linux-boot.img
```

In Git Bash set `MSYS_NO_PATHCONV=1` and use Windows-style paths, or the
`x=/bin/y` arguments get rewritten.

Flash and boot:

```
fastboot flash boot techo5-linux-boot.img
fastboot continue
```

From Android: `adb reboot bootloader` first. From the Linux image:
`rebootto bootloader`. Restore Android with
`fastboot flash boot boot-lineage-18.1-20260904-cronos.img`.

## Why these choices

- **Boot slot, not recovery.** amonet's LK boots 64-bit kernels only from
  `boot`; from `recovery` even the stock LineageOS image fails silently and the
  watchdog falls back to `boot`. TWRP's 32-bit kernel is what makes `recovery` work.
- **wpa_supplicant 2.9, not 2.11.** The vendor `mt76x8` driver writes its own RSN
  element (capabilities 0) into the association request; 2.11 advertises 16
  replay counters (0x000c) in the handshake, hostapd on the access points sees the
  mismatch and deauthenticates with "wrong key". 2.9 does not advertise them.
- **Static busybox as `/init`'s interpreter**, breadcrumbs in the spare area of
  MISC (`readmisc.sh`), boot log on userdata: the image explains its own failures.
- **Wi-Fi credentials come from Android's saved networks** on userdata
  (`/data/misc/apexdata/com.android.wifi/WifiConfigStore.xml`), so nothing is
  typed and nothing leaves the device.

## What init does

`/init` populates `/dev` (no devtmpfs in this kernel), mounts userdata and the
Android system partition read-only (for the vendor Wi-Fi module, firmware and
the daemon binary), paints the panel, brings up USB serial, loads Wi-Fi, joins
the network, sets the clock, routes the microphone, starts dropbear and the
daemon. Without a network it reboots into Android after 15 minutes so an
unattended unit never stays stuck; `touch /tmp/stay` cancels that.

## Next

Persistent rootfs on the `system` partition, the daemon's own display layer,
and (after a kernel rebuild with `CONFIG_BT`) Bluetooth. See `docs/porting-plan.md`.
