# Porting plan

The order is chosen so that every milestone leaves a usable device.

## M0 — Ground truth on the hardware

Boot LineageOS (or stock Fire OS 7) with root and record:

- `/proc/asound/cards`, `/proc/asound/devices`, `tinymix` control list,
  capture and playback formats that actually open.
- Input devices (`getevent -pl`), GPIO and LED nodes under `/sys/class`.
- The ambient light sensor path.
- Which vendor daemons hold the audio and mic-mute lines.

Deliverable: `docs/hardware.md` filled in, plus a `tools/` script that dumps
all of this from a rooted shell.

## M1 — Voice daemon on cronos

Port the EchoLocal daemon (`echod`, pure Go, MIT) to `cronos`:

- New device layout: paths, board name, model string.
- `hardware/mic`: open the cronos capture device with the right format and
  channel map; re-open on the privacy switch.
- `hardware/speaker`: playback device and mixer controls.
- `hardware/buttons` and `hardware/privacy`: `gpio-privacy-button` and
  `gpio-privacy-state`.
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
