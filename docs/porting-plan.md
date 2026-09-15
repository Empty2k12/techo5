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

Step 2 — in progress: the daemon is vendored under `echod/` and builds for cronos (default)
and the Dot (`-tags dot`). On the Show, `tools mic` captures and `tools mute` reads the latch.
`tools play` panics the kernel (see hardware.md); the daemon's playback path is blocked on
that. Not yet tried: `tools mute on/off`, `run` end to end, Home Assistant discovery.

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
