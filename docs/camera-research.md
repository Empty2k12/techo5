# Camera on the Show without Android — research notes

Started 2026-09-15 late. Goal: one raw frame from the Echo Show 5's own OV02B10 into the
daemon, with no Android camera stack. What is known so far, what the probe showed, what is next.

## What the kernel offers

The LineageOS cronos kernel at the commit we build (8d928c5176cc) already carries everything
jxlarrea/lineageos-echo-show-camera patched in for Android: the OV02B10 driver with its
register tables, `CONFIG_CUSTOM_KERNEL_IMGSENSOR="ov02b10_mipi_raw"`, Amazon's struct
layouts (`kd_imgsensor_define_checkers.h` is what `CONFIG_CRONOS` selects — the 8-byte MCLK
struct). Nothing to patch on the kernel side. Device nodes on the image: `/dev/kd_camera_hw`
(imgsensor), `/dev/camera-isp`, `/dev/camera-sysram`, `/dev/camera-pipemgr`; the ISP probes at
boot (`[Camera-ISP][ISP_probe]`, base PA 0x15000000, irqs 258/259).

**Sensor side (`/dev/kd_camera_hw`, magic `'i'`):** `SET_DRIVER` (35, two u32: main socket =
`1<<16 | index`, index 0 is the OV02B10), `T_OPEN` (0: power sequence + I2C init from the driver's
table), `T_CHECK_IS_ALIVE` (30), `GETINFO2` (65: `{u32 id; ptr info; ptr resolution}`),
`GETRESOLUTION2` (10), `FEATURECONCTROL` (15, `{u32 invoke; u32 feature; ptr para; ptr len}`;
feature ids are `3000 + position` in the checkers header's enum), `T_CLOSE` (25). The daemon
is 32-bit on a 64-bit kernel, so the compat layouts (32-bit pointers) apply; the driver has
compat conversions for all of these. `cmd/camprobe` does the open/identify/close sequence.

**ISP side (`/dev/camera-isp`, magic `'k'`):** no V4L2. The driver exposes raw register access
(`ISP_READ_REGISTER` / `ISP_WRITE_REGISTER`, `{ptr ISP_REG_STRUCT[]; u32 count}`, addresses
inside `ISP_REG_RANGE` at `CAMINF_BASE + 0x4000`), `ISP_WAIT_IRQ` / `ISP_READ_IRQ` /
`ISP_CLEAR_IRQ` (with `IMGO_DONE` bits), `ISP_BUFFER_CTRL` (enqueue/dequeue/is-ready on a
DMA ring per output, `_imgo_` = 4 is the raw output), `ISP_RESET`, `ISP_REF_CNT`. In Android
all of the ISP pipeline programming — SENINF/CSI receiver, timing generator, RAW path,
IMGO DMA — is done from userspace by MediaTek's libcamdrv through those register writes. The
kernel driver only carries the interrupt and DMA-ring plumbing and a handful of register
offsets it needs itself (`ISP_REG_ADDR_EN1 = +0x4`, `INT_STATUS = +0x24`, `DMA_INT = +0x28`…).
**The full register map is not in the kernel tree.** That is the research: the SENINF, TG and
IMGO register offsets for this ISP generation (MT8163 is the MT6735/MT8127 family, "ISP 3.0"),
which older public MediaTek kernels and leaked libcamdrv headers carry as `isp_reg.h`.

## What the probe showed (2026-09-15 18:20)

```
SET_DRIVER ok: index 0 -> 0x10000
[kd_sensorlist] [kdSetDriver] :[0][1][1][ov02b10_mipi_raw][32]
[PowerON]pinSetIdx:0, currSensorName: ov02b10_mipi_raw
[kd_MultiSensorOpen] switch I2C BUS0
[iReadRegI2C] I2C send failed!!, Addr = 0x2 / 0x3
ov02b_camera_sensor [open] Mute on will not init sensor
[PowerDown]pinSetIdx:0
T_OPEN: input/output error
```

The driver selects the sensor, runs Amazon's power-on sequence, then refuses: **the privacy
latch is engaged** ("Mute on"), which on this hardware cuts the camera's power as well as the
microphones' (jxlarrea's issue #4 and patches 0015/0016 are about exactly this latch), so the
I2C reads of the sensor id fail and it powers back down. The bench unit had been muted with the
button earlier that day. The latch is one-way from software: only the button releases it. So
the next step is simply to unmute and run `camprobe` again; expected then: `T_OPEN ok`,
`GETINFO2` with the OV02B10's id (0x2b per the driver) and 1600×1200 full / 800×600 preview
resolutions.

## Next

1. Unmute (button), rerun `camprobe` — confirms the sensor initialises and streams MIPI.
2. Find the ISP 3.0 register map: MT6735/MT8127 kernel trees (`drivers/misc/mediatek/imgsensor`
   siblings, `mt6735/isp_reg.h`, `camera_isp_reg.h`), or the MT8163 libcamdrv headers. Needed:
   SENINF (CSI-2 lane config, mux to CAM), TG (`TG_SEN_MODE`, `TG_VF_CON`, grab window),
   `CAM_CTL_EN`/`DMA_EN`/`FMT_SEL`/`SEL`, IMGO (`BASE_ADDR`, `XSIZE`, `YSIZE`, `STRIDE`), `CAM_CTL_START`.
3. With the map: `cmd/camprobe -frame`: `ISP_RESET`, program SENINF+TG+IMGO for the preview
   mode's size and Bayer format, enqueue a buffer (physical address — the `camera-sysram` /
   `ISP_BUFFER_CTRL` ring, or ION), `CAM_CTL_START`, wait `IMGO_DONE`, dequeue, dump the RAW10
   frame, demosaic in Go (nearest-neighbour is enough for a first picture).
4. Then a still-capture action for the daemon and a "show me" page. Video would be after that.

## The register map is found (2026-09-15 late)

Our ISP driver names 49 registers; the four that matter for placing the generation match the
MT6592 userspace header from the BQ Aquaris E10 GPL drop
(`mediatek/platform/mt6592/hardware/include/mtkcam/drv/isp_reg.h`, kept at
`D:\platform-tools\echoshow\camera\isp_reg_mt6592_bq_aquaris_E10.h`; the register XML it was
generated from is MT6582's): CTL_EN1 +0x004, CTL_INT_STATUS +0x024, IMGO_BASE_ADDR +0x300,
TG_VF_CON +0x414 — all identical to what `camera_isp.c` for mt8163 uses. MT6795 and later
(IMGO at +0x3300, INT at +0x4C) are a different ISP; MT6589/MT8127/MT6580/MT8163 are this one.
So the MT6592 header is the map, offsets relative to `CAMINF_BASE` (0x15000000) with the
ISP block at +0x4000 (the header's comments say 4xxx):

| register | offset | fields |
| --- | --- | --- |
| CAM_CTL_START | 0x4000 | PASS2_START, FMT_START, CQ0_START… (pass 1 runs on VF, not START) |
| CAM_CTL_EN1 | 0x4004 | per-block enables (TG1, PASS1 path…) |
| CAM_CTL_DMA_EN | 0x400C | IMGO_EN bit0, LSCI, ESFKO, AAO, IMGI, IMG2O bit10 |
| CAM_CTL_FMT_SEL | 0x4010 | SCENARIO[2:0], SUB_MODE, CAM_IN_FMT[11:8], CAM_OUT_FMT, TG1_FMT[18:16], TWO_PIX, TG1_SW |
| CAM_CTL_SEL | 0x4018 | path selects |
| CAM_CTL_INT_STATUS | 0x4024 | (driver: IMGO_DONE bit 0 of DMA_INT 0x4028, FBC_IMGO_DONE bit 28) |
| CAM_IMGO_BASE_ADDR | 0x4300 | physical address of the frame buffer |
| CAM_IMGO_XSIZE | 0x4308 | XSIZE[13:0] in bytes − 1 |
| CAM_IMGO_YSIZE | 0x430C | lines − 1 |
| CAM_IMGO_STRIDE | 0x4310 | bytes per line (+ bus size bits) |
| CAM_TG_SEN_MODE | 0x4410 | CMOS_EN bit0, DBL_DATA_BUS, SOT_MODE… |
| CAM_TG_VF_CON | 0x4414 | VFDATA_EN bit0 (this is what starts pass 1), SINGLE_MODE bit1 |
| CAM_TG_SEN_GRAB_PXL | 0x4418 | PXL_S[14:0], PXL_E[30:16] |
| CAM_TG_SEN_GRAB_LIN | 0x441C | LIN_S, LIN_E |

Not in this header: the SENINF / CSI-2 receiver block. Settled where it lives: **userspace
too.** Our ISP driver's `ISP_WRITE_REGISTER` accepts addresses in these ranges (camera_isp.c
lines 140–175, "the same with the value in seninf_drv.cpp"): ISP `0x15000000` (+0x10000),
SENINF `0x15008000` (+0x4000), MIPI RX config `0x1500C000` (+0x100), MIPI RX analog
`0x10217000` (+0x3000), PLL `0x10000000` (+0x1000), GPIO `0x10005000`. So the CSI receiver,
the MIPI D-PHY and the sensor clock PLL are all programmed from userspace through the same
ioctl; the map for those is MediaTek's `seninf_reg.h` (same generation, the MT6592 one is
not at the isp_reg.h path in the BQ E10 drop — find it in another MT6592/MT8127 GPL drop, or
the MT6580/MT6582 ones, which list the same "MT6582 xml" origin). The imgsensor kernel driver
does no SENINF work at all (no `seninf` in `src/mt8163`).

Frame buffers: `ISP_BUFFER_CTRL` ENQUE takes an `ISP_RT_BUF_INFO_STRUCT {memID, size,
base_vAddr, base_pAddr, …}` — a physical address the caller already has, so the buffer comes
from ION (`/dev/ion`, the multimedia heap the display probe already used in `internal/mtkdisp`)
or `/dev/camera-sysram`; the driver tracks it in a ring per DMA and reports `bFilled` on dequeue.

## Plan for the first frame

1. Unmute (button), `camprobe` → sensor id and resolutions.
2. SENINF: find who programs the CSI receiver; if userspace, take its offsets from the
   MT6592 `seninf_reg.h`.
3. A buffer the ISP can write: `ISP_BUFFER_CTRL` on `_imgo_`, or `/dev/camera-sysram`, or an
   ION buffer's physical address — check what the driver's enqueue expects.
4. Program: FMT_SEL (TG1 raw 10-bit, scenario pass-1), DMA_EN.IMGO_EN, IMGO base/xsize/ysize/
   stride for the preview mode, TG grab window from the sensor's `GET_CROP_INFO`, TG_SEN_MODE
   CMOS_EN, then TG_VF_CON.VFDATA_EN=1; wait `IMGO_DONE`; VFDATA_EN=0.
5. Dump the RAW10 buffer, unpack, nearest-neighbour demosaic, PNG. Then the daemon's
   "take a picture" action and a page.

## Sensor alive (2026-09-16)

The OV02B10 answers from Linux. The blocker was the sensor master clock: the SENINF timing
generator (TG1) that divides the 48 MHz camtg clock down to the CMMCLK pad is programmed by
nobody in the kernel unless the board's device tree says "cmmclk-always-on" (cronos does not),
and Android's libcamdrv writes those registers from userspace through ISP_WRITE_REGISTER. The
kd_camera_hw power-on only flips the MCLK1_EN bit (SENINF_TG1_PH_CNT bit 29); with the divider
unset the pad is silent and an OmniVision sensor NACKs every I2C transfer. Neither the "Mute on
will not init sensor" line (it just means the chip ID did not match) nor the "unbalanced
disables for vcama" warning (a double power-down when open fails) was the cause.

camprobe now mirrors camera_isp.c's ISP_set_mclk1(clkcnt) + ISP_MCLK1_EN(1) before T_OPEN, as
CAMINF-relative offsets through the ISP register ioctls (magic 'k', READ_REG = 2, WRITE_REG = 3,
compat ISP_REG_IO_STRUCT {u32 pData; u32 Count}, ISP_REG_STRUCT {u32 Addr; u32 Val}, Addr is
an offset from 0x15000000 and must lie in 0x4000..0xFFFF):

| offset | register | value set |
|---|---|---|
| 0x8000 | SENINF_TOP_CTRL | clear 0xc00, or 0x300 |
| 0x8100 | SENINF1_CTRL | bit 0 (kernel writes SENINF1_EN this way) |
| 0x8120 | SENINF1_MUX_CTRL | bit 31 MUX_EN |
| 0x8200 | SENINF_TG1_PH_CNT | bit 31 PCEN, bits 0-1 TGCLK_SEL = 1, bit 2 CLKFL_POL = !(clkcnt&1), bit 6 PADCLK_INV = 0, bit 28 CLK_POL = 0, bit 29 MCLK1_EN |
| 0x8204 | SENINF_TG1_SEN_CK | bits 0-5 CLKFL = clkcnt>1 ? (clkcnt+1)/2 : 1, bits 8-13 CLKRS = 0, bits 16-21 CLKCNT = clkcnt |

clkcnt 1 with the 48 MHz group gives 24 MHz, what the driver's imgsensor_info.mclk asks for.
Result of `camprobe`: "i2c write id: 0x78, sensor id: 0x2b", "Sensor init" (the whole init
table went over I2C), GETINFO2 with SensorId = 1 (main socket; it is an input) reports mclk 24,
interface 1 (MIPI), output format 3, MIPI lane count field 0 (= 1 lane), and 1600 x 1200 for
preview/capture/video/high-speed/slim.

Corrections to the notes above: the cronos SET_MCLK_PLL struct is the *default* 12-byte layout
{u8 on; u32 freq; u8 TG}, not the checkers one (the 8-byte command is also accepted through the
AMZN alias). Two ioctls are unusable from our 32-bit daemon: CHECK_IS_ALIVE power-cycles the
sensor on its own (it undoes T_OPEN), and GETRESOLUTION2's compat path allocates
sizeof(pointer) bytes for a 16-byte struct and corrupts the caller's stack (camprobe crashed at
PC 0). GETINFO2 returns the resolutions anyway.

Power/pins that are fine as they are: regulators vcama/vcamio are real mt6323 consumers of
15008000.camera1 and enable on T_OPEN; CMRST is GPIO 22, CMPDN GPIO 23 (pinctrl states
cam0_rst0/1, cam0_pnd0/1); the privacy (mute) driver's state is 0 = off; CMMCLK is pin 119 and
its mode is left as the bootloader set it (the "cam_mclk" pinctrl state is never selected by
the driver, and the sensor works without it).

Next: the receiver. To get a frame we must program from userspace, in this order, SENINF1
CSI-2 receiver (1 lane, RAW10) + MIPI RX analog (0x10217000) + SENINF1 mux to TG1/CAM, the ISP
TG (TG_SEN_MODE/TG_VF_CON at +0x410/+0x414 in isp_reg.h terms), the RAW path enable (CAM_CTL_EN1
+0x4 etc.), IMGO DMA (base +0x300, xsize/ysize/stride) with an ION buffer, then SensorControl /
FEATURE_SET_SCENARIO to start streaming and wait for IMGO_DONE. The SENINF/CSI2/MIPI-RX
offsets are the outstanding research item (seninf_reg.h for the MT6582/MT6592/MT8127 family).

## First frame (2026-09-16, later)

`cmd/camframe` captures a full 1600 x 1200 raw Bayer frame from Linux. Two things unlocked it.

**The register map.** The genuine MT8163 libcamdrv sources exist on GitHub
(488315-archive/mt8163-vendor, `mediatek/proprietary/hardware/mtkcam/legacy/platform/mt8163/`):
`seninf_reg.h`, `seninf_drv.cpp`, `HalSensor.control.cpp`, `isp_reg.h`. Copies and a digest live in
`D:\platform-tools\echoshow\camera\` (`SENINF-notes.md` has every offset and bit used). The MT6582 and
MT8127 SENINF layouts are different and must not be used; the ISP side (isp_reg.h) is byte-identical
to the MT6592 header.

**Register windows, not ioctls.** The ISP driver's mmap hands out the physical blocks directly
(page offset must equal the base): CAMINF 0x15000000 (0x10000, CAM registers at +0x4000), SENINF
0x15008000 (0x4000), MIPI RX analog 0x10217000 (0x3000). camframe maps all three and programs them
the way SeninfDrvImp does; the imgsensor ioctls are used only for power, clock mux, init and the
preview-mode control.

Sequence that produced the frame (see the code for the exact bits):

1. Open camera-isp (clocks on), SET_MCLK_PLL 48 MHz group, SENINF TG1 divider clkcnt 1 (24 MHz),
   TGCLK_SEL 1, CLKPOL 1 (the HAL's value for a polarity-LOW sensor).
2. ION multimedia heap buffer, ION_MM_CONFIG_BUFFER with module id 24 (M4U_PORT_IMGO) then
   ION_SYS_GET_PHYS for the MVA; the IMGO M4U port switched to translated mode through
   `/proc/m4u` ioctl MTK_M4U_T_CONFIG_PORT (_IOW 'g' 11, M4U_PORT_STRUCT, Virtuality 1, Distance 1).
   The m4u device is a proc node, not /dev.
3. SET_DRIVER, SET_CURRENT_SENSOR, T_OPEN.
4. CAM: ISP_RESET ioctl; INT_EN 0 (poll, so the kernel's ring-buffer code never runs);
   CTL_CLK_EN 0x1FFFF; EN1 = TG1_EN | PAK_EN; DMA_EN IMGO; FMT_SEL TG1_FMT RAW10 (1<<16);
   PIX_ID 0; MUX_SEL2 IMGO_MUX 0 with IMGO_MUX_EN; IMGO_FBC 0; IMGO_BASE = MVA; XSIZE 1599,
   YSIZE 1199, STRIDE 1600; CTL_IMGO_SIZE 1600<<16|1200; TG_SEN_MODE CMOS_EN|SOT_MODE;
   TG_VF_CON SPDELAY_MODE; GRAB_PXL 0..1600; GRAB_LIN 0..1200; PATH_CFG 0.
5. SENINF/CSI (setSeninf1NCSI2 verbatim): analog 0x4C/0x50 &= 0xFEFBEFBE, lanes 0x00..0x10 |= 8,
   0x24 |= 1, 30 us, 0x20 |= 3, 1 us, lanes |= 1; HSRX calibration (0x3D8 = 0x1F, 0x338 |= 1,
   0x33C = 0x1541, 0x338 |= 4, 500 us, check 0x344 & 0x10001 and 0x348 & 0x101, undo) — it passes;
   SENINF1_MUX_CTRL = MUX_EN | SRC 8 | FIFO_FULL_WR_EN 1 | FLUSH 0x3B | PUSH 0x3F; SENINF1_CTRL =
   EN | SRC 8 | PAD2CAM 10-bit; NCSI2_DPCM 0; NCSI2_CTL |= HSRX_DET_EN, REF_SYNC_DET_EN, ED_SEL
   (ECC order 1), CLOCK_LANE_EN, DATA_LANE0_EN; LNRD_TIMING settle = 85 ns * 364 / 1000 = 30 << 8;
   NCSI2_INT_EN 0 (no handler in our kernel for that line); mux soft-reset pulse; TOP_MUX_CTRL[3:0] 0.
6. CONTROL(preview) on the sensor (its mode table ends with stream-on), then TG_VF_CON.VFDATA_EN.

What the DMA delivers with this setup is **one byte per pixel** (the top 8 of the TG's 12 bits;
values 5..79 for a dim room, 255 at a lamp), Bayer R first. Findings on the way there:

- With `IMGO_MUX` 0 the DMA is fed by the packer: without `PAK_EN` nothing is written at all,
  whatever XSIZE says. With PAK_EN and XSIZE 1999 the DMA wrote 600 rows of 2000 bytes: each row
  was a 1600-byte line plus the first 400 bytes of the next one (the DMA fills XSIZE+1 bytes per
  row and resyncs at the next line start). XSIZE 1599 / stride 1600 gives the whole frame,
  1,920,000 bytes, every time. `IMGO_STRIDE.FORMAT/FORMAT_EN` made no difference (FORMAT 1) or
  killed the DMA (FORMAT 0); left clear. The 10-bit packed output the HAL uses needs another knob
  (probably the PAK format select) — not found yet, and 8 bits are enough for a preview.
- The CAM interrupt status registers (INT_STATUS, DMA_INT) read 0 throughout with INT_EN 0, so
  the "IMGO done" poll never fires and camframe reports "no frame" even when the buffer is full.
  Frame sync must come from elsewhere: TG_INTER_ST (0x444C, CAM_FRM_CNT bits 23:16), the
  NCSI2 FRAME_LINE_NUM counter, or the driver's own ring buffer with interrupts enabled.
- TG_SOF_CNT stays 0 and TG_EOT_CNT reads 0x0FFFFFFF; SENINF1_MUX_INTSTA shows CRCERR, VSIZEERR
  and HSIZEERR set once the stream runs — the frame is still clean, so these are noted, not
  understood.
- Register readbacks after the run: SENINF1_CTRL 0x8001, MUX_CTRL 0x9EFF8080, NCSI2_CTL
  0x058961F1 (bits beyond the ones we set are reset defaults), analog 0x00/0x04 = 0x8009,
  0x20 = 0xFF000003, 0x24 = 0x24248801.

The first picture (a person in front of it, sharp, green-tinted because it is raw Bayer with no white
balance) is kept outside the repo at `D:\platform-tools\echoshow\camera\first-frame-2026-09-16.png`.

Next steps, in order: frame sync via the TG frame counter; continuous capture into two buffers
(or the driver's ring with interrupts enabled — then the kernel handles the base-address flip);
white balance and a real demosaic in the daemon; a live "camera" page on the panel; a snapshot
served to Home Assistant (the ESPHome camera image API or a plain HTTP endpoint) so the Show works
as a camera entity; 10-bit output later.
