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
