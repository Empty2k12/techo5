# Echo Show 5 (2nd gen, 2021) — `cronos`

Ground truth captured with `tools/hwdump.sh` on a unit running LineageOS 18.1
(unofficial). Raw output: [dumps/cronos-lineage-18.1-bench.txt](dumps/cronos-lineage-18.1-bench.txt).
Items marked *unverified* have not been confirmed on a unit by this project.

## Board

| Item | Value |
|---|---|
| SoC | MediaTek MT8163, 4× Cortex-A53 (`CPU part 0xd03`), 600 MHz – 1.3 GHz, 32-bit userspace (`armv8l`, `ABI: arm`) |
| GPU | ARM Mali (`kbase`) |
| RAM | 996 MB (`MemTotal`), plus a 483 MB zram swap on LineageOS |
| Storage | 7.6 GB eMMC (`8GTF4R`), partitions below |
| Display | 5.5-inch 960×480 IPS, DSI video mode, panel `st7701s` (cmdline `lcm=1-st7701s_wsvga_dsi_vdo_cronos_st_truly`), 59.64 Hz, backlight `/sys/class/leds/lcd-backlight` (0–255) |
| Touch | Goodix GT9xx (`gt9xx`, I²C 2-0x5d), input `goodix-ts`, raw axes 480×960 (panel is mounted rotated), 16 slots |
| Audio in | 4-mic array into a TI **TLV320AIC3101** ADC (I²C 0-0x18) |
| Audio out | one speaker on a Maxim **MAX98396** class-D amp (I²C 2-0x3d, reset on `gpio-392`) |
| Wi-Fi / BT | MediaTek **MT7668** SDIO combo (`mt76x8_wlan.ko`, `mt76x8_bt.ko`, firmware in `/vendor/firmware`) |
| Sensors | ambient light + proximity (`alsps`, I²C 0-0x44), exposed as input `m_alsps_input` and Android "Light Sensor" |
| Camera | main + sub camera on I²C 0, mechanical lens cover on `gpio-499` (`SW_CAMERA_LENS_COVER`) |
| Buttons | volume up (`gpio-393`), volume down (`gpio-394`), mic-mute (`gpio-404`) |
| Bootloader | Amazon LK; stock build `77c8c2e-20211019_182552`, amonet replaces it with `44072a3-20240709_162755`; preloader `29ba1b5-20210311_160043` |
| Kernel | Linux **4.9.337** (LineageOS build), Linaro GCC 6.3 |
| Stock OS | Fire OS 7 (Android 9 based) |

## Partitions (eMMC)

| Name | Device | Size |
|---|---|---|
| kb, dkb | p1, p2 | 1 MB each |
| lk | p3 | 1 MB |
| tee1, tee2 | p4, p6 | 5 MB each |
| logo | p5 | 1 MB |
| expdb | p7 | 16 MB |
| MISC | p8 | 512 KB |
| boot | p9 | 16 MB |
| recovery | p10 | 16 MB |
| swdl | p11 | 32 MB |
| system | p12 | 3.0 GB (LineageOS uses ~1.0 GB) |
| cache | p13 | 256 MB |
| persist | p14 | 16 MB |
| metadata | p15 | 40 MB |
| userdata | p16 | 3.9 GB |

`boot` is a plain 16 MB Android boot image, which is enough for a kernel plus
a small initramfs. The LineageOS kernel command line already carries
`androidboot.selinux=permissive` and `androidboot.veritymode=disabled`.

## Audio

One ALSA card, `mt-snd-card`, with 24 PCM devices. Only two matter:

| Path | Role | Format seen open |
|---|---|---|
| `/dev/snd/pcmC0D22c` | `TLV320AIC3101 Capture` | **S24_3LE, 4 channels, 16 000 Hz**, period 257, buffer 2570 |
| `/dev/snd/pcmC0D23p` | `MAX98396_Playback` | **S16_LE, 2 channels, 48 000 Hz**, period 768, buffer 1536 |

This is the same shape EchoLocal drives on the Dot (S24_3LE capture, 48 kHz
S16_LE stereo playback), so its raw-ioctl ALSA code should carry over with
the device numbers and channel count changed.

### Verified with `cmd/audioprobe` (raw ioctls, vendor HAL idle, satellite app stopped)

- Capture opens at 320-frame periods × 8 and reads 3 s with no overruns.
  Playback opens at 768 × 4 and plays with no underruns. Both devices are
  free once the satellite app is stopped; the vendor HAL keeps only
  `controlC0` open.
- **Channel map of `pcmC0D22c`:**

  | ch | content |
  |---|---|
  | 0 | microphone |
  | 1 | bit-identical copy of ch 0 |
  | 2 | playback loopback, **left** |
  | 3 | playback loopback, **right** |

  Proven by playing 1 kHz left / 1.5 kHz right while capturing: ch 2 and ch 3
  decorrelate (corr ≈ 0), ch 0 and ch 1 stay identical to the bit in every
  recording. The loopback pair is silent (exact zeros) whenever nothing plays
  and does not depend on `Audio_ExtCodec_EchoRef_Switch`.
- The quiet-room floor on the mic channel is about −69 dBFS RMS; a 0.3 FS
  tone from the device's own speaker lands at about −17 dBFS RMS on the mic.
- `ADC_A Left Mute = 1` did **not** change the captured audio, so the
  `ADC_A` controls are not in the path that produces this stream as
  configured, or the stream is a mono capture duplicated by the AFE. Whether
  a second, independent microphone channel can be enabled is open.
- Host-side analysis: `go run ./tools/wavstats file.wav`.

Mixer (`tinymix`, 118 controls) — the ones that look relevant:

- ADC (TLV320AIC3101): `ADC_A Digital Volume Control` (88 88), `ADC_A MICPGA Volume Ctrl` (40 40),
  `ADC_A Left/Right Mute`, `ADC_A Left/Right Fine Volume`, the `ADC_A * Ip Select` input routing
  (DIF1_L / DIF1_R selected), `DAI Sel Mux A`.
- Amp (MAX98396): `Digital Volume A` (127), `Speaker Volume A` (8), `Speaker Safe Mode A`,
  `Ramp Up/Down Switch A`, `Dither Switch A`, `Amp Fault Enable`, `VI Sense A Switch`.
- MediaTek AFE: `Ext_Speaker_Amp_Switch` (On), `Audio_I2S0dl1_hd_Switch` (On), `Board Channel Config` (Stereo).

Amazon's processing lives in userspace, in `audio.primary_amazon.mt8163.so`,
with its configs under `/vendor/etc/audio-algorithms/`: `AFE.cfg`,
`coefs_FBF.cfg` (fixed beamformer), `Tap_AEC_mic1/2.cfg` (echo cancellation),
EQ and multiband limiter tables. A replacement daemon gets the raw four
channels and must do its own beamforming or echo cancellation, or duck
playback while listening.

Per-unit microphone calibration is in `/proc/idme/miccal.0` … `miccal.3`.

### Warning

Do **not** run `dumpsys media.audio_flinger` on this device. It null-derefs in
the vendor audio HAL, audioserver restarts, and both capture and playback stay
silent until reboot. `dumpsys audio` is safe.

## Input and the mute button

| Input device | Node | Events |
|---|---|---|
| `gpio-privacy-state` | event0 | `SW_MUTE_DEVICE` — the hardware mute state |
| `gpio-privacy-button` | event1 | `KEY_POWER` (scan code 116) |
| `mtk-kpd` | event2 | none |
| `goodix-ts` | event3 | multitouch |
| `hwmdata` | event4 | REL_Y/REL_Z (sensor hub) |
| `m_alsps_input` | event5 | ABS_X = lux, ABS_WHEEL = proximity |
| `gpio-keys` | event6 | `KEY_VOLUMEUP`, `KEY_VOLUMEDOWN`, `SW_CAMERA_LENS_COVER` |

- Android's generic layout maps code 116 to POWER, so the mute button sleeps
  the screen. A per-device override in
  `/data/system/devices/keylayout/gpio-privacy-button.kl` containing
  `key 116 WAKEUP` fixes that (needs root, takes effect after reboot).
- The mute is a **hardware** function: the button toggles a latch, and
  `privacy-state-gpio` (`gpio-405`, input) reports it while
  `privacy-enable-gpio` (`gpio-384`, **output**, currently low) drives it.
  That output is the line a daemon can use to control the mic mute (and the
  red mute indicator, see below). After unmuting, an app that was capturing
  gets silence until it reopens the capture path.

GPIO block: `gpiochip0`, GPIOs 357–511 on `1000b000.pinctrl`.

## LEDs and light sensor

- `/sys/class/leds` only exposes `lcd-backlight`. The red mute indicator is not
  a Linux LED; it is most likely driven by the privacy circuit alongside
  `privacy-enable-gpio`. *(unverified — test by toggling gpio-384.)*
- Light sensor: `alsps` at I²C 0-0x44, read through the input device or the
  Android sensor HAL (`android.hardware.sensors@1.0-service`, sensor
  "Light Sensor" by `amazon-oss`). Calibration in `/proc/idme/alscal`.

## Factory data (`/proc/idme`)

`board_id`, `serial`, `mac_addr`, `bt_mac_addr`, `miccal.0-3`, `alscal`,
`sensorcal`, `ledparams`, `unlock_code`, `bootcount`, `bootmode`, `postmode`,
`dev_flags`, `fos_flags`, `region`, `locale`. `product_name`, `productid`
and `productid2` read `0` on the unit seen.

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
  options allows `adb root`. SELinux is permissive.
- Auto time zone is wrong; set `settings put global auto_time_zone 0` and
  `service call alarm 3 s16 <zone>`.
- The default launcher package is `com.android.launcher3`.
- Vendor services still running that a slimmer image can drop: camera HAL
  and cameraserver, Widevine and ClearKey DRM, CAS, `amazonthermal`,
  `vendor.power-amazon`, `securetime`, `kisd`, media codec services.

## Network behaviour seen in the field

- The satellite app listens on TCP 10800 (VACA / ShowAssist) and advertises
  over mDNS. mDNS across subnets needs a reflector rule for `_esphomelib._tcp`
  or the VACA service depending on the daemon in use.

## Open questions

- Whether toggling `privacy-enable-gpio` (gpio-384) from userspace mutes the
  array and lights the red indicator, and whether the latch can be cleared
  after a button press.
- Whether the AIC3101 needs any mixer setup beyond what the vendor HAL leaves
  behind at boot when the HAL is not running.
- Why capture channels 0 and 1 are identical: mono ADC, AFE duplication, or a
  second mic that needs routing. The `ADC_A` mute had no effect on it.
- Which Amazon GPL kernel source drop matches the 4.9.337 LineageOS kernel
  and the stock LK `77c8c2e-20211019`.
