# Echo Show 5 (1st gen, 2019): `checkers`

Desk research, 2026-09-18. **Nothing here has been run on a unit.** Every claim is read
out of the kernel sources the LineageOS build for `checkers` comes from
(`github.com/amazon-oss/android_kernel_amazon_mt8163`, branch `cronos/lineage-18.1`,
which carries both projects' defconfigs and device trees), or out of the public unlock
and release pages. Items are marked *(unverified)* where a unit is needed to settle them.

The short version: `checkers` is far closer to `cronos` than the Dot or the Spot are. The
panel, the touch controller, the capture path, both PCM device numbers, the radio and the
partition layout are the same. Four things differ, and one of them (the speaker) is real work.

## Why the two are so close

| | `cronos` (2021) | `checkers` (2019) |
|---|---|---|
| SoC | MT8163, 32-bit userspace | same |
| Kernel | 4.9.337 arm64, `cronos_defconfig` | 4.9.337 arm64, `checkers_defconfig`, same tree and branch |
| LineageOS 18.1 (r0rt1z2) | `lineage-18.1-20260904-UNOFFICIAL-cronos` | `lineage-18.1-20260904-UNOFFICIAL-checkers`, same build date |
| Unlock | amonet `mt8163-cronos` | amonet `mt8163-checkers`, same button combo and fastbrick flow |
| Panel | ST7701S 480x960 portrait, 63x125 mm | identical: `st7701s_wsvga_dsi_vdo_checkers.c` has the same `FRAME_WIDTH`/`FRAME_HEIGHT` and physical size |
| Framebuffer | `CONFIG_FREE_FB_BUFFER=y`, so fbdev lives outside Android | same |
| Touch | Goodix `gt9xx` at i2c 2-0x5d | same (`checkers_pvt.dts` deletes the focaltech node) |
| Capture | FPGA on SPI into `amzn-mt-spi-pcm`, 4 channels S24_3LE 16 kHz, `pcmC0D22c` | same: neither defconfig sets `CONFIG_SND_SOC_4_MICS`/`8_MICS`, so `SPI_N_CHANNELS` is 4 on both |
| Mic ADC | TLV320AIC3101 at i2c 0-0x18 (0x19 disabled) | same, disabled in `checkers_pvt.dts` the same way |
| Playback node | `pcmC0D23p` | same: `mt_soc_dai_common[23]` is the playback link in the shared machine driver |
| Wi-Fi / BT | MT7668 SDIO, `mt76x8_wlan.ko` / `mt76x8_bt.ko`, `/dev/stpbt` | same, down to `mt76x8_pmu_en_gpio = <&pio 32 0>` |
| Buttons | 114 / 115 / 116, plus `SW_CAMERA_LENS_COVER` | same codes, same lens-cover GPIO |
| Light sensor | `alsps` behind MediaTek hwmsensor | same interface, different chip (`mediatek,jsa1214`) |
| Partitions | MISC p8, boot p9, system p12, userdata p16 | same, which is every partition this project hardcodes |

So the whole display layer (`feature/display` and the `render_*` files, the settings sheet,
themes), `hardware/screen`, `hardware/touch`, `hardware/mic`, `hardware/ambient`,
`hardware/buttons`, the slot store, the rescue initramfs, the updater and the rootfs
tarball are expected to work unchanged.

## What actually differs

### 1. The speaker is a different chip

`checkers` has no MAX98396 and no TAS5805m. Its codec is a Realtek **RT5616** at i2c 2-0x1b
(`codec: alc5616@1b` in `checkers.dtsi`), with an external amplifier on a GPIO
(`realtek,amp-gpio = <&pio 35 0>`, pinctrl states `extamp-pullhigh` / `extamp-pulllow`).
The shared machine driver leaves `mt_soc_dai_common[23]` at its default, `RT5616_Playback`
with codec `rt5616.2-001b`; only `CONFIG_CRONOS` rewrites that entry to MAX98396 / TAS5805m /
RT5616E by board id.

Consequences for `hardware/speaker`:

- There is no `Speaker Safe Mode A` to clear, and no `Digital Volume A` / `Speaker Volume A`.
  The RT5616 controls are `HP Playback Switch`, `HP Playback Volume`, `HPVOL Playback Switch`,
  `OUT Playback Switch`, `OUT Playback Volume` and `DAC1 Playback Volume`.
- `Ext_Speaker_Amp_Switch` is a genuine amplifier enable here, unlike the MAX98396 reset line
  on `cronos`, but it is **active low**: it drives `amp_gpio` (pio 35), and LineageOS plays with
  the control `Off` and the pin low, leaving it `On` and high when idle. Switching it **On** to
  play, the way `paths_dot.go` and `paths_spot.go` do, is what silenced this board. It is set
  `Off` once in `initSequence` and `AmpSwitch` stays empty, so the amplifier is simply left
  enabled. *(measured 2026-09-18, six samples across one stream.)*
- The RT5616's routing is what the rest of the silence was: the kernel leaves the DAC connected
  to nothing and the vendor HAL wires it once at boot, so the codec configures, powers up and
  converts nothing. The path runs DAC → `OUT MIX` → `OUTVOL` → `LOUT`, never through the
  headphone pins (`HPO MIX DAC1` and `HP Playback` stay off). `AUD_CLK_BUF_Switch` stays `Off`
  throughout, this codec needs no PMIC clock buffer, and the pinmux is byte-identical idle and
  playing, so neither is the problem it looked like.
- **There are two mutes in `LOUT_CTRL1` (reg 03), and clearing one is not enough.**
  `OUT Playback Switch` is the output mute (bits 15 and 7); `OUT Channel Switch` is the volume
  stage's own (bits 14 and 6). The RT5616 comes out of reset with both set, and LineageOS has the
  second clear even when idle. With only the first cleared the register sits at `4848` and the
  board is silent while *every other piece of evidence says it should not be*: the DAPM dump is
  byte-identical to a playing LineageOS unit, `amp_gpio` is low, the PCM is open and fed. Both
  together give `0808` against Amazon's `0a0a`, two volume steps apart, and the speaker plays.
  That cost a whole debugging round: on this codec, check reg 03 itself rather than trusting the
  DAPM graph. *(confirmed on the unit over the serial console, 2026-09-18.)*
- The volume curve is the MAX98396's, tuned by ear for that amplifier. It has to be measured
  and retuned for this one; keep the software curve, keep the limiter.
- The DL1 SRAM hold (`DRAMHold`) and the "leave the codec as the kernel brought it up"
  posture come from the MediaTek DL1 driver and apply here too. *(unverified)*

### 2. The mute latch is `amazon-gating`, not `gpio-privacy`

`checkers_defconfig` has `CONFIG_GATING=y` and no `CONFIG_KEYBOARD_GPIO_PRIVACY`. The node is
`amazon-gating` (`state-gpios = <&pio 48>`, `enable-gpios = <&pio 27>`, button on `<&pio 47>`),
driven by `drivers/misc/gating.c`. It exposes the same pair of files under
`/sys/devices/platform/amazon-gating/`: `state` readable, `enable` write-only, and
`set_gating_state()` refuses `UNGATED`, so the latch is one-way from software exactly as on
`cronos`.

The difference is the input side: one input device named `gating` reporting key code 116, and
**no** `SW_MUTE_DEVICE` switch device. So the mute state comes from the sysfs file alone, and
the button arrives as a key event like any other. `hardware/buttons` needs nothing, it opens
every input node and maps by code. `hardware/privacy` needs the other directory and no switch
node.

### 3. Kernel build inputs

`checkers_defconfig` appends five device trees (`checkers_proto`, `_hvt`, `_evt`, `_dvt`,
`_pvt`) where `cronos` appends eleven, and carries only the `i2s_to_spi_4ch_v193.bin` FPGA
bitstream (`cronos` ships v193 and v208 and adds `fpga-compatible-old` / `control_reset`
properties that `checkers` does not have). So:

- `tools/linux/build-kernel.sh` needs the defconfig as a parameter (`checkers_defconfig`).
  Everything else about it, the commit pinning, the Bluetooth options, the module ABI story,
  is unchanged. The checkers LineageOS build is the same commit: a unit reports
  `4.9.337-g8d928c5176cc`, so the vendor modules load. *(confirmed 2026-09-18.)*
- `tools/linux/patch-dtb.py` walks the appended trees; five instead of eleven, and all five
  carry `amzn,mic-downmix` (built 2026-09-18: "5 device trees, 5 changes"). checkers averages
  its two microphones into both slots exactly as cronos did, and the same delete undoes it.

### 4. The camera is a different sensor

`CONFIG_CUSTOM_KERNEL_IMGSENSOR="ov9734_mipi_raw"`, and the 1st gen ships a 1 MP camera
against the 2nd gen's 2 MP. Everything in `hardware/camera` that is ISP work (receiver, TG,
CMMCLK divider, IMGO DMA ring, debayer, exposure) carries over, and so does the sensor
programming: the kernel's imgsensor driver owns the power sequence and the I2C init table for
both parts, and both are one-lane MIPI RAW10. What the daemon has to know is the geometry, so
`sensorGeometry` carries it per board: 1600x1200 for the OV02B10, 1280x720 for the OV9734,
with the exposure ceilings scaled to the shorter frame. The OV9734 numbers are its nominal mode,
not yet read off a unit, so `open` logs what the driver reports for line length, frame length
and pixel clock (`SENSOR_FEATURE_GET_PERIOD` and `GET_PIXEL_CLOCK_FREQ`, the same eight-byte
parameter `SET_ESHUTTER` uses) next to the geometry it assumed. One boot with the camera
acquired says whether the table is right. *(unverified)*

Two things the table cannot answer and a first picture can: whether the Bayer order is the same
RGGB (red and blue swapped is unmistakable), and whether the receiver needs anything the
OV02B10 did not.

## The shape of the port: a variant, not a fourth build tag

Adding a `checkers` tag the way `dot` and `spot` were added means editing roughly forty
`!dot && !spot` build constraints, writing about fourteen near-duplicate device files, and
running a second release channel, all for three runtime differences on hardware that is
otherwise the same board.

Cheaper, and what this document recommends: **one binary that runs on both Show 5
generations**, deciding at runtime.

- Mute: use whichever of `/sys/devices/platform/amazon-gating` and
  `/sys/devices/platform/gpio-privacy` exists, and take the state from sysfs when there is no
  `SW_MUTE_DEVICE` node.
- Speaker: pick the init sequence from the mixer controls the card actually has
  (`Speaker Safe Mode A` present means MAX98396), or from `lcm=` in `/proc/cmdline`, which
  names the panel driver and therefore the project. `layout.Idme` is already there if a
  factory field turns out cleaner.
- Model string: the same switch, so Home Assistant shows "Echo Show 5 1st gen (checkers)".

Feature detection also means neither `internal/update`'s `archSuffix` nor the manifest gains a
key: one `arm` build, one rootfs tarball, one update entity, both devices. The only per-device
artifact is the boot image, because the kernel and its appended device trees differ. That is
one more file in the release and one more `--boot` default in the installer.

If the two ever diverge enough that the runtime switches outnumber the shared code, the build
tag is still there to reach for.

## What is in the tree (2026-09-18)

Written from the sources above, not yet run on a 1st gen:

- `layout.Board` and `layout.Model` are worked out at start by `boardName`, from the `lcm=` panel
  name on the kernel command line, falling back to the presence of the `amazon-gating` directory.
  `layout.Checkers()` is what the rest asks.
- `hardware/privacy` takes whichever of `amazon-gating` and `gpio-privacy` the unit has; the
  semantics (one-way latch, the pulse on `enable`, the lag before `state` follows) are shared.
- `hardware/speaker`: `speakerPath` returns the init sequence and `AmpSwitch` for the board.
  cronos clears the MAX98396's safe mode and never touches the amplifier switch; checkers uses the
  switch as a plain enable and writes nothing to the codec. The volume curve is still the one
  tuned for the MAX98396.
- `hardware/camera` drives both sensors: `sensorGeometry` is the only board-dependent part, and
  `Width`, `Height`, the line and frame sizes and the exposure ceilings follow from it. The
  sensor's own line and frame lengths are logged at open to check that table against a unit.
- Bluetooth needs nothing: the same MT7668 behind the same `mt76x8_bt.ko` and `/dev/stpbt`, the
  same `/proc/idme/bt_mac_addr`, and `build-kernel.sh` turns the Bluetooth options on whichever
  defconfig it started from. It is a smoke test, not a port.
- `tools/linux/build-kernel.sh` and `build-image.sh` take `DEVICE=checkers`; `patch-dtb.py`
  already rewrites however many device trees it finds.
- `tools/install-show.py` takes `--device checkers` (its own boot image, and the kernel release
  check waits for a unit to report one), and `tools/release.ps1` takes `-BootCheckers` to publish
  `techo5-boot-checkers-<version>.img` beside the shared daemon, manifest and root filesystem.

## Work list

1. **Bring-up on a unit.** amonet `mt8163-checkers`, then LineageOS 18.1 for checkers, then
   `tools/hwdump.sh`. Answers the kernel commit, the mixer control list, the input node names,
   `/proc/idme` fields and the partition map in one go.
2. **`audioprobe`.** Capture on `pcmC0D22c` (expect 4 channels, S24_3LE, 16 kHz) and confirm
   which slots are microphones and which are the playback loopback, and whether ch0 and ch1 are
   bit-identical (downmix). Play on `pcmC0D23p` with the DL1 hold.
3. **Speaker path.** Confirm the amplifier switch is safe and needed here, find out whether the
   RT5616 needs any control written at all, then the volume curve by ear against the same
   announcement `cronos` was tuned with.
4. **Privacy.** Confirm the latch, the red indicator and the button through `amazon-gating`.
5. **Kernel and boot image.** `DEVICE=checkers tools/linux/build-kernel.sh`, device trees patched,
   image built the usual way, flashed to `boot`.
6. **Camera.** Acquire it once and read the logged line/frame lengths against the assumed
   geometry, then take a picture: skew means the line length is wrong, red and blue swapped mean
   the Bayer order is.
7. **Everything else is a smoke test**: screen, touch, Wi-Fi, Bluetooth, wake word, the slot
   store, an update, an alarm.
8. **Installer and release.** Fill in the kernel release for `checkers` in `install-show.py`'s
   `DEVICES` once a unit reports one, and publish the boot image with
   `release.ps1 -BootCheckers`.

## Open questions for the first unit

Three of the six are answered; the dumps behind them are on
[PR #2](https://github.com/HuskerMinion/techo5/pull/2).

- Does the OV9734 come up at 1280x720 with the same RGGB order, or does the table need both
  corrected?
- ~~Is `Ext_Speaker_Amp_Switch` safe to switch here, and does the speaker need it On?~~ Safe, and
  it needs it **Off**: the control is active low.
- ~~Is the LineageOS checkers kernel the same commit the vendor modules were built against?~~
  Yes, `4.9.337-g8d928c5176cc`.
- ~~Is the recovery slot p11 here rather than p10?~~ No: by-name gives `recovery` p10 (16 MB) and
  `swdl` p11 (32 MB), the same layout as cronos. Writing `boot-recovery` into the first 32 bytes
  of MISC (p8) and rebooting lands in TWRP, so the bootloader reads the BCB.
- Does the boot slot boot 64-bit kernels only, as on `cronos`?
- Does the mute latch light the red indicator and cut the microphones the same way, and is it
  equally one-way? The red LED is not in `/sys/class/leds`, which holds only `lcd-backlight`.

## Sources

- Kernel: `github.com/amazon-oss/android_kernel_amazon_mt8163`, branch `cronos/lineage-18.1`:
  `arch/arm64/configs/{checkers,cronos}_defconfig`,
  `arch/arm64/boot/dts/mediatek/checkers.dtsi` and `checkers_pvt.dts`,
  `drivers/misc/gating.c`, `drivers/misc/mediatek/lcm/st7701s_wsvga_dsi_vdo_checkers/`,
  `sound/soc/mediatek/mt_soc_audio_8163_amzn/mt_soc_machine.c`,
  `sound/soc/mediatek/mt_soc_audio_8163_amzn/amzn-spi-pcm/`, `sound/soc/codecs/rt5616.c`.
- Unlock: `github.com/R0rt1z2/amonet`, branch `mt8163-checkers`, and its XDA thread
  "[UNLOCK][ROOT][TWRP][UNBRICK] Amazon Echo Show 5 1st Gen - 2019 (checkers)".
- LineageOS builds: `github.com/amazon-oss/releases`, tags `lineage-18.1-checkers-*`.
- Partition map and install flow write-up: `github.com/dallanwagz/echo-show-jailbreak`
  (`docs/JAILBREAK-SHOW5.md`, `docs/PARTITION-REFERENCE.md`), third party, unverified here.
