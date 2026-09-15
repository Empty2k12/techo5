# Porting plan

The order is chosen so that every milestone leaves a usable device.

## M0 — Ground truth on the hardware — done

`tools/hwdump.sh` run on a LineageOS unit; findings in `docs/hardware.md`.
Still open from M0: whether `privacy-enable-gpio` controls the mute and the
red indicator from userspace, and the matching GPL kernel source drop.

## M1 — Voice daemon on cronos

Step 1 — done: `cmd/audioprobe` (pure Go, `internal/alsa` from EchoLocal)
captures and plays through the raw devices with no vendor HAL involvement.
Channel map established: mic, mic copy, loopback L, loopback R. Build with
`GOOS=linux GOARCH=arm GOARM=7 CGO_ENABLED=0 go build ./cmd/audioprobe`.

Step 2 — **working end to end** (2026-09-14). The daemon is vendored under `echod/` and builds
for cronos (default) and the Dot (`-tags dot`). On the Show it captures, plays, detects the wake
word (microWakeWord, on the CPU at ~27 % of one core), streams to Home Assistant, and speaks
the reply. Home Assistant discovers it over zeroconf as an ESPHome device ("Echo Show 3D4E5F",
manufacturer TECHO5) with 102 entities. First complete turn: "Hey Jarvis, what time is it?" →
"7:11 PM", at a sensible volume.

What it took, all in `docs/hardware.md`: hold an AFE node so the DL1 driver uses DRAM, never
toggle the amp switch, treat the mute latch as one-way, decode announcement WAVs by header,
and keep Android's audio stack off the devices — for now by stopping the framework
(`tools/bench-nofw.sh`).

Later the same day: Android moved to its null primary audio HAL
(`ro.hardware.audio.primary=default` in `/system/build.prop`), which keeps audioserver off the
PCM devices for good, and the daemon became an init service (`tools/init/techo5.rc`,
`/system/bin/techo5`) that starts on `sys.boot_completed`. Android, ShowAssist on screen and
the daemon now run side by side, and the daemon survives a reboot.

Still to do for M1:

- Done since: `tools/install-cronos.ps1` provisions a Show in one run (binary, init service,
  null HAL, name, key, wake models, key layout); the bench unit is named "Bench Show" with
  the `bench_show_` entity prefix and ShowAssist's satellite is disabled ("Bench Show
  Screen"); the Alexa microWakeWord model is installed from esphome/micro-wake-word-models.
- Understand the second capture channel (identical copy of the mic) and whether a second mic
  exists.
- Volume: even the top of the curve (unity) was heard as a little quiet in the room. The next
  step up is the amplifier's own gain, `Speaker Volume A` (MAX98396, 8 of 17 as the HAL leaves
  it); raise it carefully and re-measure headroom before making it the default.
- Releases: `tools/release.ps1` builds a versioned daemon, writes `manifest.json` with
  `cmd/mkmanifest` and publishes a GitHub release. The daemon's updater (the `update` entity in
  Home Assistant) follows the latest release and replaces `/system/bin/techo5` in place; init
  restarts the daemon into the new binary (the service must not be `oneshot`). The new binary
  runs **on trial** for five minutes, then commits. Cronos has no vendor boot hook, so a trial
  binary that dies is rolled back in-process by the next start (verified 2026-09-15 with
  v0.1.2 → v0.1.1 in five seconds, no reboot). Consequence: `setprop ctl.stop`/`ctl.restart`
  is a SIGKILL and looks like a crash — during a trial, stop the daemon with `kill -TERM` (the
  bench scripts and the installer do), or simply leave it for five minutes.
- Loudness: the amplifier's safe mode was the cap (see hardware.md); the daemon clears it,
  speech is normalised to −14 dBFS RMS, and the volume curve sits 6 dB under the Dot's.

The port covers:

- New device layout: paths, board name, model string.
- `hardware/mic`: open `pcmC0D22c` as S24_3LE, 4 channels, 16 kHz; ch 0 is
  the mic, ch 2/3 the playback reference for the canceller (the Dot has
  7 mics + 2 refs, so `Mics`/`Refs` become 1 and 2 and the beamformer is
  bypassed); re-open on the privacy switch.
- `hardware/speaker`: `pcmC0D23p` at 48 kHz S16_LE stereo; volume through
  the MAX98396 `Digital Volume A` / `Speaker Volume A` controls.
- `hardware/buttons` and `hardware/privacy`: `gpio-keys` (event6),
  `gpio-privacy-button` (event1) and `gpio-privacy-state` (event0);
  drive `privacy-enable-gpio` (gpio-384) for mute.
- Light sensor from `m_alsps_input` (event5, ABS_X = lux).
- Drop the Dot-only LED ring; add the mute LED if reachable.
- Install as an init service on LineageOS (`/system/etc/init/techo5.rc`)
  instead of the Fire OS `ledcontroller` takeover that EchoLocal uses on the Dot.

Result: the Show appears in Home Assistant as an ESPHome device with a voice
satellite and a media player, exactly like the Dots, while Android keeps
running the screen.

## M2 — Display layer

Baseline in place (2026-09-14): ShowAssist stays on the screen as a display-only web view of
the `echo-show` dashboard (its own satellite role is disabled), and the dashboard reacts to the
daemon: a card driven by `assist_satellite.<name>_assist_satellite` shows "Listening…",
"Thinking…" with the transcript, then transcript and reply while the answer plays; the Now
Playing and volume tiles and the station chips target the daemon's `media_player`. All of that
is Home Assistant configuration, no device code.

Direction chosen (2026-09-15): the daemon gets its **own** display layer, so the
end device runs no Android UI at all. ShowAssist stays only as the interim screen
until that layer works. Where the daemon-owned layer runs is the open question
below.

### What was learned reaching for the panel from userspace

The goal was to paint the panel directly from a small Go program
(`internal/mtkdisp`, `cmd/dispprobe`), with the Android framework stopped. The
kernel is MediaTek's 4.9 `mtkfb`/`mtk_disp_mgr` stack (sources at
`amazon-oss/android_kernel_amazon_mt8163`, branch `lineage-18.1`). Findings, all
verified on the bench unit:

- **The Linux framebuffer is a dead end.** `/dev/graphics/fb0` reports
  `smem_len = 0`; the driver allocates the real buffer itself and exposes it only
  through the overlay path, so `mmap` on fb0 is refused at every size, with or
  without SurfaceFlinger running. `cmd/fbprobe` records this.
- **The display-manager API works up to the last step.** `/dev/mtk_disp_mgr`
  takes a 32-bit compat ABI; the struct sizes were confirmed against the running
  kernel by probing which argument size each ioctl accepts (`ScanIoctlSize`).
  `internal/mtkdisp` creates the primary session, reads its info (480×960,
  ~60 Hz, physical 63×125 mm), allocates ION multimedia buffers that get valid
  M4U addresses (get-phys returns non-zero), switches the session between direct
  link and decouple (which visibly moves the RDMA registers), reads back the
  overlay/RDMA registers, captures what the panel is scanning out, and waits on
  vsync — all working.
- **The overlay config never latches.** With a valid buffer address handed to
  the overlay (`src_phy_addr`, so the kernel's own fence lookup is bypassed),
  `SET_INPUT_BUFFER` + `TRIGGER_SESSION` return success and the CMDQ record shows
  the config tasks executing for our process — yet `OVL0 src_con` stays 0 and the
  layer-0 address stays at the boot framebuffer. The frame config is accepted and
  submitted but is never committed to the overlay registers. That commit is the
  CMDQ trigger-loop / display-mutex machinery the hardware composer builds at its
  own init; a bare session join does not reproduce it, and reconstructing it means
  reimplementing most of `primary_display.c` against undocumented CMDQ tokens.

Conclusion: driving this vendor stack from userspace is a poor investment. The
display layer is folded into M4, and until then the screen stays on ShowAssist (below).

**Resolved later the same day (2026-09-15): the plain Linux framebuffer works once
Android is out of the picture.** Booted into TWRP, `cmd/fbprobe` mapped `fb0`
(5.5 MB, 480×960×32 bpp, two pages) and painted the panel. The LineageOS kernel
reports `smem_len = 0` only because it is built with `CONFIG_FREE_FB_BUFFER`: the
display driver frees the boot framebuffer the first time the Android compositor
configures its own overlay layers. Nothing in a Linux boot triggers that, so the
same kernel keeps its framebuffer there. Details in `docs/hardware.md` (Display).
So the daemon-owned screen is **fbdev on the downstream kernel**, drawn by the
daemon itself — no DRM, no compositor, no Android. Mainline `mediatek-drm` was
checked and is not needed (and, see M4, mainline is ruled out anyway by Wi-Fi).

`internal/mtkdisp`, `cmd/dispprobe` and `cmd/fbprobe` are kept as the record of
the vendor path and a working probe of the panel geometry, capture and registers;
`fbprobe` is the seed of the display layer.

### Interim screen (in place since 2026-09-14)

ShowAssist stays on the screen as a display-only web view of the `echo-show`
dashboard (its own satellite role is disabled), and the dashboard reacts to the
daemon: a card driven by `assist_satellite.<name>_assist_satellite` shows
"Listening…", "Thinking…" with the transcript, then transcript and reply while
the answer plays; the Now Playing and volume tiles and the station chips target
the daemon's `media_player`. All of that is Home Assistant configuration, no
device code. Screen state (on/off, brightness) can later be exposed through the
same ESPHome device so Home Assistant sees one device, not two.

## M3 — Slim the OS (skipped)

Superseded by M4 on 2026-09-15: with fbdev proven and the daemon already owning
audio, going straight to a Linux-only image is less work than trimming Android.

## M4 — Linux-only image (the end state)

Decided 2026-09-15: the end device boots a Linux image with no Android userspace
at all — the downstream kernel, a small Alpine (armv7) rootfs, the daemon owning
mic, speaker, wake word, ESPHome API **and the screen** (fbdev, see M2), Wi-Fi
through the vendor driver, and Bluetooth for earbuds. M3 (slimming Android) is
skipped; Android stays only as the fallback in the `boot` partition until the
Linux image is trusted.

### Kernel: downstream 4.9, not mainline

Surveyed 2026-09-15, all details in `docs/hardware.md` (Kernels, Wi-Fi/Bluetooth):

- Mainline (bengris32 `linux-mtk`, branch `mt8163/7.0`, March 2026) has MT8163
  `mediatek-drm` and cronos device trees, but the cronos tree is a skeleton (eMMC,
  USB, keys, light sensor — no panel, touch, audio codecs or SDIO), and the
  mainline `mt76` driver has **no MT7668 Wi-Fi** support at all (only its
  Bluetooth half, `btmtksdio`). Wi-Fi alone rules mainline out.
- Two downstream kernels exist for cronos, both from `amazon-oss/android_kernel_amazon_mt8163`:
  - `cronos/lineage-18.1`: **4.9.337, arm64** with 32-bit userspace — what LineageOS
    boots and what the daemon is validated on. Wi-Fi/BT are vendor modules shipped
    in LineageOS `/vendor/lib/modules` (`mt76x8_wlan.ko`, `mt76x8_bt.ko`) and load
    with a plain `insmod`. USB gadget (configfs: ACM, RNDIS, FunctionFS) built in.
  - `cm-14.1`: **4.9.77, 32-bit ARM** — what TWRP and the postmarketOS
    `amazon-checkers` port boot (touch driver, audio codecs and the MT7668 combo
    glue built in, but the Wi-Fi/BT drivers themselves are out-of-tree).
- **Choice: `cronos/lineage-18.1`.** Same kernel as the Android fallback, the audio
  quirks are already mapped, and the Wi-Fi/BT modules exist as binaries, so the
  first Linux boot needs no kernel build at all. `cm-14.1` stays the fallback if
  the arm64 kernel misbehaves outside Android.
- A kernel rebuild is still needed later, for Bluetooth: neither kernel has
  `CONFIG_BT`. The vendor BT driver exposes `/dev/stpbt` (raw H4 packets), not a
  Linux HCI device, so BlueZ needs the kernel BT core plus either `hci_vhci` (a
  small userspace bridge stpbt ↔ vhci) or `hci_uart` H4 over a pty. Rebuilding
  means also rebuilding the vendor Wi-Fi/BT modules (sources: the LineageOS
  vendor tree / `gitlab.com/echo-pmos/amazon-checkers-vendor`, `amazon/wlan` and
  `amazon/bluetooth`). WSL Ubuntu 24.04 is on the build host; toolchain not yet
  installed (needs sudo).

### Bootloader facts that shape the plan

- amonet's LK (kaeru 2.0.0) does **not** implement `fastboot boot`; every test
  image has to be flashed. Use the `recovery` slot (16 MB) for Linux images and
  keep the Android `boot` untouched as the fallback. Backups of all three:
  `D:\platform-tools\echoshow\{recovery-twrp-cronos,boot-lineage-18.1-20260904-cronos,lk-amonet-cronos}.img`.
- `fastboot reboot` after `adb reboot bootloader` lands back in fastboot;
  `fastboot continue` boots normally.
- Kernel + initramfs must fit 16 MB: the LineageOS kernel is 7.4 MB gzip, so the
  initramfs has ~8 MB. Enough for Alpine's minirootfs (3.2 MB gz) plus the daemon
  (~4 MB gz, or lzma/xz which the kernel accepts). The real rootfs can live on
  `system`/`userdata` later; a self-contained initramfs is the first target.

### Steps

1. **First Linux boot** (no kernel build): LineageOS kernel + an initramfs built
   from the Alpine 3.24 armv7 minirootfs with a tiny init (`tools/linux/init`)
   that populates `/dev` (no devtmpfs in this kernel), **clears the recovery
   bootloader message in MISC first**, keeps a boot log in `/data/techo5-linux/`
   on userdata (readable from Android afterwards), paints the panel with
   `fbprobe` (clock ticking), offers a root shell over USB CDC ACM (Windows sees
   a COM port; RNDIS is out because Windows 11 24H2+ dropped it), insmods the
   vendor Wi-Fi module from the mounted Android system partition as a first
   probe, and reboots into Android after 15 minutes unless `/tmp/stay` exists.
   `tools/linux/mkimage.py` builds the boot image straight from the tarball
   (no root, no cpio binary). Built 2026-09-15 as
   `D:\platform-tools\echoshow\techo5-linux-test.img` (13 MB; not yet booted).
   Flash to `recovery`, `adb reboot recovery`. Failure mode: if init never runs
   (kernel panic before userspace), MISC keeps the recovery message and the unit
   loops into the test image — volume-down at power-up into fastboot, then
   `fastboot flash recovery recovery-twrp-cronos.img`. So: first boot with the
   unit at hand.
2. **Wi-Fi**: mount the LineageOS `system` partition read-only, `insmod` the
   vendor `mt76x8_wlan.ko` with `firmware_class.path` pointing at its `vendor/firmware`,
   `wpa_supplicant` + DHCP. Then SSH (dropbear) replaces the USB cable.
3. **Daemon**: run `techo5` from the initramfs — audio is already raw ALSA, so
   it should work unchanged; verify the DL1 hold trick and the amp safe-mode
   clear still apply on a non-Android boot.
4. **Display layer** in the daemon: a framebuffer renderer (Go, no GPU) for the
   clock/voice/media screens, touch from `goodix-ts` (event3), backlight from
   `/sys/class/leds/lcd-backlight`, light sensor from event5. Screen state exposed
   as ESPHome entities. Retire ShowAssist and the `echo-show` dashboard for this device.
5. **Rootfs on eMMC**: move from initramfs to a persistent Alpine on `system`
   (3 GB) with an A/B or rollback story that fits the existing updater's trial
   semantics; the initramfs stays as the rescue environment.
6. **Bluetooth** (earbuds, user requirement 2026-09-15): rebuild the kernel with
   `CONFIG_BT` + `hci_vhci`, bridge `/dev/stpbt`, BlueZ + `bluez-alsa` (or
   PipeWire) as an A2DP source; route the daemon's playback to the earbuds when
   connected. Pairing driven from the on-screen UI.
7. Second unit rollout with the installer rewritten for the Linux image.

## Ground rules

- Everything stays local; no cloud services.
- Upstream projects (EchoLocal, VACA, LineageOS device tree) are used under
  their licenses and credited. Changes worth sending upstream are proposed,
  not assumed.
