# Echo Show 5 (2nd gen, 2021) — `cronos`

What is known so far. Items marked *unverified* come from third-party
teardowns or forum posts and have not been confirmed on a unit by this
project.

## Board

| Item | Value |
|---|---|
| SoC | MediaTek MT8163 (quad Cortex-A53, 32-bit userspace on LineageOS: `ABI: arm`) |
| RAM | 1 GB *(unverified)* |
| Storage | eMMC, 8 GB class *(unverified)* |
| Display | 5.5-inch, 960×480, capacitive touch |
| Audio in | far-field microphone array with a hardware privacy (mute) switch |
| Audio out | single speaker |
| Radios | Wi-Fi 2.4/5 GHz, Bluetooth |
| Sensors | ambient light sensor, camera with mechanical shutter |
| Buttons | volume up, volume down, mic mute |
| Bootloader | Amazon LK, build `77c8c2e-20211019_182552` on the units seen so far |
| Stock OS | Fire OS 7 (Android 9 based) |

## Unlock and recovery

- **amonet-cronos v2.0.1** (k4y0z, r0rt1z2). With the device on mains power,
  hold all three buttons until the screen shows `=> FASTBOOT mode`, connect
  USB, run the fastbrick payload. The exploit reboots the device into TWRP.
  Never interrupt it.
- Fastboot reports `product: CRONOS`; `unlock_status` flips to `true` afterwards.
- TWRP works over `adb shell twrp ...` (format data, wipe, install zip).
  TWRP running at all is the proof that the kernel, display and touch work
  outside Android: it is the same downstream kernel with a small userspace.

## LineageOS 18.1 (unofficial, r0rt1z2)

- Android 11, `userdebug`, test keys. Build seen: `lineage-18.1-20260904-UNOFFICIAL-cronos`.
- Enable USB debugging after first boot; "Rooted debugging" in developer
  options allows `adb root`.
- Auto time zone is wrong; set `settings put global auto_time_zone 0` and
  `service call alarm 3 s16 <zone>`.
- The default launcher package is `com.android.launcher3`.

## Input and the mute button

- The mute button is input device `gpio-privacy-button`, scan code 116, which
  the generic key layout maps to POWER, so pressing it sleeps the screen.
  A per-device override in `/data/system/devices/keylayout/gpio-privacy-button.kl`
  containing `key 116 WAKEUP` fixes that (needs root, takes effect after reboot).
- The same button also toggles a **hardware** microphone mute
  (`gpio-privacy-state`, reported by AudioService as `mic mute FromSwitch=true`).
  After unmuting, an app that was capturing gets silence until it reopens the
  capture path. Any satellite daemon must reopen the mic on unmute.

## Audio HAL warning

Do **not** run `dumpsys media.audio_flinger` on this device. It null-derefs in
the vendor audio HAL, audioserver restarts, and both capture and playback stay
silent until reboot. `dumpsys audio` is safe.

## Network behaviour seen in the field

- The satellite app listens on TCP 10800 (VACA / ShowAssist) and advertises
  over mDNS. mDNS across subnets needs a reflector rule for `_esphomelib._tcp`
  or the VACA service depending on the daemon in use.

## Open questions

- Exact ALSA/tinyalsa device numbering, capture format and channel count of
  the mic array under the stock kernel (EchoLocal on the Dot uses S24_3LE
  capture and 48 kHz S16_LE stereo playback; expect something similar but
  verify with `tinymix`/`/proc/asound`).
- Whether the ambient light sensor and mute LED are reachable over sysfs/I2C
  without the vendor HAL.
- Which Amazon GPL kernel source drop matches the LK build above.
