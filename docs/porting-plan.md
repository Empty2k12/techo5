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
  Home Assistant) follows the latest release and replaces `/system/bin/techo5` in place; the
  v0.1.0 → v0.1.1 round trip was done from Home Assistant's update button on 2026-09-15, with
  init restarting the daemon into the new binary and the previous one kept for rollback.
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

Pick one and keep it thin:

1. A minimal Android kiosk app (WebView of a Home Assistant dashboard,
   Browser Mod for per-device control), or
2. ShowAssist with its satellite disabled, display only.

Expose screen state (on/off, brightness, current path) through the same
ESPHome device so Home Assistant sees one device, not two.

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
