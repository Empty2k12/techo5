# TECHO5 in one page

What the device does, how it is built, and where to look. The porting plan is the history; this
is the state.

## What a TECHO5 is

An Amazon Echo Show 5 (MediaTek MT8163, "cronos") running our own Linux image instead of
Android, with one Go daemon, `echod`, doing everything the device does:

- **Voice satellite for Home Assistant** over the ESPHome API: local wake word (microWakeWord on
  the CPU), streaming to Home Assistant's pipeline, spoken replies, echo cancellation so the
  wake word works over music.
- **Screen**: clock and weather, the conversation as it happens, a now-playing page with song and
  artwork, a forecast page, live views of Home Assistant cameras and of the Show's own camera, and
  a swipe-down settings sheet with tabs (Device, Bluetooth, Cameras, Radio, Theme) and Wi-Fi
  setup with an on-screen keyboard. A first-run card shows once.
- **Radio**: stations from Home Assistant's lists, playing on the device's own speaker or on
  Bluetooth earbuds; song, artist and cover from iHeartRadio or TuneIn behind the page.
- **Camera**: the front camera as a Home Assistant camera entity, plus JPEG and MJPEG over HTTP,
  with auto-exposure. Off unless something is looking, and off while the mute button is engaged.
- **Bluetooth audio** to earbuds or a speaker; a Bluetooth proxy for Home Assistant (scanning
  through BlueZ on the Show), off by default.
- **Updates**: two root filesystem slots with a trial and automatic fallback; a release that
  carries a rootfs tarball installs over the air from Home Assistant's update entity.

## How it is built

| Layer | What | Where |
|---|---|---|
| Bootloader | Amazon's LK, patched for our logo and unlocked with the amonet/kaeru chain | `tools/linux/patch-lk-logo.py`, `docs/hardware.md` |
| Kernel | LineageOS cronos 4.9 rebuilt with Bluetooth and MODVERSIONS, vendor Wi-Fi/BT modules | `tools/linux/build-kernel.sh`, `patch-dtb.py`, `build-image.sh` |
| Root filesystem | Alpine armv7 plus our overlay, built in WSL, two slots on the userdata partition | `tools/linux/mkrootfs.sh`, `wsl-build.sh`, `deploy-rootfs.sh`, `rootfs/` overlay, `slotctl` |
| Daemon | Go, one binary, features as components registered at init | `echod/` |
| Audio | ALSA directly on the vendor PCM devices; MAX98396 amplifier controls; echo canceller in `hardware/mic` | `echod/internal/hardware/{speaker,mic}` |
| Screen | Framebuffer, our own renderer, touch from the input device | `echod/internal/hardware/{screen,touch}`, `feature/display` |
| Camera | Sensor interface, CSI-2 receiver, ISP timing generator and DMA programmed from userspace through the ISP driver's register windows | `echod/internal/hardware/camera`, `docs/camera-research.md` |
| Bluetooth | Vendor `/dev/stpbt` bridged to `/dev/vhci` by `cmd/btbridge`; BlueZ and bluez-alsa | `cmd/btbridge`, `echod/internal/lib/{bluez,bluealsa}`, `feature/btaudio` |
| Home Assistant | ESPHome device API (go-esphome-device), state subscription, device actions, REST with a long-lived token for weather and camera proxies | `feature/api`, `feature/hastate`, `feature/home`, `lib/hass` |
| Updates | Manifest with binaries and a rootfs; slot devices install through `slotctl` | `echod/internal/update`, `tools/release.ps1`, `echod/cmd/mkmanifest` |

## Working on it

- **Build the daemon**: `GOOS=linux GOARCH=arm GOARM=7 CGO_ENABLED=0 go build ./cmd/echod` in
  `echod/`. `-tags dot` builds the Echo Dot variant.
- **Try a build on the bench**: copy the binary over SSH to `/usr/local/bin/techo5` (remount `/`
  read-write first) and kill the running daemon; the supervisor restarts it. `deploy-rootfs.sh`
  is the real path: it builds a rootfs and installs it into the spare slot, on trial.
- **See the screen from the PC**: `http://<device>:8181/screen.png` with `?sheet=<tab>`,
  `?theme=<name>`, `?radio=<station>`, `?wifi=list|keyboard` to put pages up first.
- **Logs**: `/data/techo5-linux/techo5.log` on the device; `dmesg` for the kernel.
- **Slots**: `slotctl status`, `slotctl install <tar.gz>`, `slotctl switch <a|b>`; a trial slot
  commits after five minutes of a healthy daemon.
- **Release**: `tools/release.ps1 -Version vX.Y.Z -Notes "..." -Rootfs <tarball>` builds the
  daemon, writes the manifest, and publishes the release Home Assistant will offer.

## Known gaps

- The Bluetooth proxy on the Show only scans: BlueZ owns the controller, so there is no beacon
  and no active connections for Home Assistant. It stays off by default until it has run a while.
- Song metadata rests on two undocumented service endpoints; when one changes shape the page
  falls back to the station logo.
- Exposure has no scene awareness: a bright window behind a face still darkens the face.
- Timers and alarms have no screen of their own.
- The Wi-Fi keyboard offers letters, digits and common symbols; no other alphabets.
