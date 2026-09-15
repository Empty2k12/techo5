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
clean way to a daemon-owned panel is the mainline **DRM/KMS** driver for MT8163
(`mediatek-drm`): standard atomic modeset with dumb buffers, no CMDQ, no compat
guessing. That belongs in the Linux image (M4). So the display layer is folded
into M4, and until then the screen stays on ShowAssist (below).

`internal/mtkdisp`, `cmd/dispprobe` and `cmd/fbprobe` are kept as the record of
the vendor path and a working probe of the panel geometry, capture and registers.

### Interim screen (in place since 2026-09-14)

ShowAssist stays on the screen as a display-only web view of the `echo-show`
dashboard (its own satellite role is disabled), and the dashboard reacts to the
daemon: a card driven by `assist_satellite.<name>_assist_satellite` shows
"Listening…", "Thinking…" with the transcript, then transcript and reply while
the answer plays; the Now Playing and volume tiles and the station chips target
the daemon's `media_player`. All of that is Home Assistant configuration, no
device code. Screen state (on/off, brightness) can later be exposed through the
same ESPHome device so Home Assistant sees one device, not two.

## M3 — Slim the OS

With the daemon and kiosk carrying the load, remove what Android no longer
needs: launcher, dialer, messaging, browser, gallery, music, setup wizard,
print spooler. Measure boot time and idle memory before and after. Consider a
custom LineageOS build from the same device tree with those packages removed.

## M4 — Beyond Android (optional, later)

A Linux-only image on the downstream kernel: Alpine or Buildroot rootfs, the
Go daemon, a Wayland compositor with a WPE WebKit kiosk. This is the leanest
end state but the largest effort, and depends on M0 answers about audio.

## Ground rules

- Everything stays local; no cloud services.
- Upstream projects (EchoLocal, VACA, LineageOS device tree) are used under
  their licenses and credited. Changes worth sending upstream are proposed,
  not assumed.
