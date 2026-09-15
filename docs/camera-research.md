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
