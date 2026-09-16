//go:build linux

// camframe grabs one raw frame from the Echo Show's camera without Android: it does what
// MediaTek's libcamdrv does from userspace on this ISP generation. The kernel drivers only
// power the sensor (imgsensor), gate clocks and hand out register windows (camera-isp); every
// register of the sensor interface, the CSI-2 receiver, the timing generator and the IMGO DMA
// is programmed here through the ISP driver's mmap.
//
//	camframe [-out /tmp/frame] [-timeout 3s]
//
// Writes <out>.raw10 (the packed RAW10 frame as the DMA wrote it) and <out>.png (an 8-bit
// grey half-size preview, 2x2 Bayer cells averaged). docs/camera-research.md has the map.
package main

import (
	"errors"
	"flag"
	"fmt"
	"image"
	"image/png"
	"os"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/unix"
)

// ---- imgsensor (/dev/kd_camera_hw, magic 'i') ----

const (
	sensMagic     = 'i'
	nrOpen        = 0
	nrControl     = 20
	nrClose       = 25
	nrSetDriver   = 35
	nrSetMCLK     = 60
	nrSetCurrent  = 70
	dualMainShift = 16
	sensorMain    = 1 // DUAL_CAMERA_MAIN_SENSOR
	scenPreview   = 0 // MSDK_SCENARIO_ID_CAMERA_PREVIEW
)

// ---- ISP (/dev/camera-isp, magic 'k') ----

const (
	ispMagic   = 'k'
	nrIspReset = 0

	// Physical windows the ISP driver's mmap hands out (page offset must equal the base).
	ispBase, ispLen       = 0x15000000, 0x10000 // CAMINF: cam ctl/dma/tg at +0x4000
	seninfBase, seninfLen = 0x15008000, 0x4000  // sensor interface + CSI-2 receivers
	mipiBase, mipiLen     = 0x10217000, 0x3000  // MIPI RX analog

	// CAM registers, offsets inside the CAMINF window (isp_reg.h names, MT6582 family).
	regImgSwRst    = 0x000C
	regCtlStart    = 0x4000
	regCtlEn1      = 0x4004
	regCtlEn2      = 0x4008
	regCtlDmaEn    = 0x400C
	regCtlFmtSel   = 0x4010
	regCtlSel      = 0x4018
	regCtlPixID    = 0x401C
	regCtlIntEn    = 0x4020
	regCtlIntSt    = 0x4024
	regCtlDmaInt   = 0x4028
	regCtlSwCtl    = 0x405C
	regCtlMuxSel   = 0x4074
	regCtlMuxSel2  = 0x4078
	regImgoFbc     = 0x40F4
	regImgoBase    = 0x4300
	regImgoOfst    = 0x4304
	regImgoXsize   = 0x4308
	regImgoYsize   = 0x430C
	regImgoStride  = 0x4310
	regImgoCon     = 0x4314
	regImgoCon2    = 0x4318
	regTgSenMode   = 0x4410
	regTgVfCon     = 0x4414
	regTgGrabPxl   = 0x4418
	regTgGrabLin   = 0x441C
	regTgPathCfg   = 0x4420
	regTgInt1      = 0x4428
	regTgSofCnt    = 0x4430
	regTgSotCnt    = 0x4434
	regTgEotCnt    = 0x4438
	regTgErrCtl    = 0x443C
	regTgDatNo     = 0x4440
	intPass1Done   = 1 << 10
	intSof1        = 1 << 12
	dmaIntImgoDone = 1 << 0

	// SENINF registers, offsets inside the SENINF window.
	regSeninfTopCtrl  = 0x0000
	regSeninfTopMux   = 0x0008
	regSeninf1Ctrl    = 0x0100
	regSeninf1Mux     = 0x0120
	regSeninf1MuxInt  = 0x0128
	regSeninf1MuxSize = 0x012C
	regTG1PhCnt       = 0x0200
	regTG1SenCk       = 0x0204
	regMipiRxCon38    = 0x0338
	regMipiRxCon3C    = 0x033C
	regMipiRxCon44    = 0x0344
	regMipiRxCon48    = 0x0348
	regNcsi2Ctl       = 0x03A0
	regNcsi2LnrdTim   = 0x03A8
	regNcsi2Dpcm      = 0x03AC
	regNcsi2IntEn     = 0x03B0
	regNcsi2IntSt     = 0x03B4
	regNcsi2LnrcFsm   = 0x03C8
	regNcsi2LnrdFsm   = 0x03CC
	regNcsi2FrameLine = 0x03D0
	regNcsi2HsrxDbg   = 0x03D8

	// more CAM registers
	regCtlImgoSize = 0x414C
	regCtlClkEn    = 0x4150
	regTgInterSt   = 0x444C
)

// ---- ION (/dev/ion) and M4U (/proc/m4u) ----

const (
	ionHeapMultimedia = 10
	ionCmdSystem      = 0
	ionCmdMultimedia  = 1
	ionSysGetPhys     = 1
	ionMMConfigBuffer = 0
	m4uPortIMGO       = 24 // M4U_PORT_IMGO in the mt8163 port enum
)

type ionAlloc struct {
	Len, Align, HeapIDMask, Flags uint32
	Handle                        int32
}
type ionFd struct{ Handle, Fd int32 }
type ionHandle struct{ Handle int32 }
type ionCustom struct{ Cmd, Arg uint32 }
type ionMMConfig struct {
	Handle                                           int32
	ModuleID, Security, Coherent, IovaStart, IovaEnd uint32
}
type ionMMData struct {
	MMCmd  uint32
	Config ionMMConfig
	_      [64]byte
}
type ionSysGetPhysP struct {
	Handle       int32
	PhyAddr, Len uint32
}
type ionSysData struct {
	SysCmd uint32
	Phys   ionSysGetPhysP
	_      [128]byte
}
type m4uPort struct {
	PortID                                            int32
	Virtuality, Security, Domain, Distance, Direction uint32
}

func ioc(dir, typ, nr, size uintptr) uintptr { return dir<<30 | size<<16 | typ<<8 | nr }

func ioctl(fd int, req uintptr, arg unsafe.Pointer) error {
	_, _, e := syscall.Syscall(syscall.SYS_IOCTL, uintptr(fd), req, uintptr(arg))
	if e != 0 {
		return e
	}
	return nil
}

// mmio is one mapped register window.
type mmio struct {
	mem  []byte
	name string
}

func mapWindow(fd int, base, length int64, name string) (*mmio, error) {
	mem, err := syscall.Mmap(fd, base, int(length), syscall.PROT_READ|syscall.PROT_WRITE, syscall.MAP_SHARED)
	if err != nil {
		return nil, fmt.Errorf("mmap %s at %#x: %w", name, base, err)
	}
	return &mmio{mem: mem, name: name}, nil
}

func (m *mmio) rd(off uint32) uint32 {
	return atomic.LoadUint32((*uint32)(unsafe.Pointer(&m.mem[off])))
}

func (m *mmio) wr(off, val uint32) {
	atomic.StoreUint32((*uint32)(unsafe.Pointer(&m.mem[off])), val)
}

func (m *mmio) mask(off, clear, set uint32) { m.wr(off, m.rd(off)&^clear|set) }

func (m *mmio) show(tag string, offs ...uint32) {
	fmt.Printf("%s %s:", m.name, tag)
	for _, o := range offs {
		fmt.Printf(" %04x=%08x", o, m.rd(o))
	}
	fmt.Println()
}

// setMCLK1 mirrors camera_isp.c's ISP_set_mclk1(clkcnt) + ISP_MCLK1_EN(1): the CMMCLK pad runs
// at camtg / (clkcnt + 1); with the 48 MHz group clkcnt 1 is the sensor's 24 MHz.
func setMCLK1(s *mmio, clkcnt uint32) {
	clkfPol := uint32(0)
	if clkcnt&1 == 0 {
		clkfPol = 1
	}
	clkfEdge := uint32(1)
	if clkcnt > 1 {
		clkfEdge = (clkcnt + 1) >> 1
	}
	s.mask(regTG1PhCnt, 1<<31, 1<<31)
	s.mask(regSeninfTopCtrl, 0xc00, 0x300)
	s.mask(regTG1SenCk, 0x3f|0x3f00|0x3f0000, clkfEdge|clkcnt<<16)
	s.mask(regTG1PhCnt, 0x3|0x4|1<<6|1<<28, 1|clkfPol<<2|1<<28) // TGCLK_SEL=1, CLKPOL=1 (HAL: polarity LOW -> 1)
	s.mask(regSeninf1Mux, 1<<31, 1<<31)
	s.mask(regSeninf1Ctrl, 1<<31, 1)
	s.mask(regTG1PhCnt, 1<<29, 1<<29)
}

// frame geometry: the OV02B10's only mode.
const (
	width, height = 1600, 1200
	maxFrameBytes = width * 2 * height // room for the widest IMGO output format
)

// imgoFormat is the IMGO DMA's pixel format (IMGO_STRIDE.FORMAT, with FORMAT_EN set):
// 0 = 8 bits per pixel, 1 = 10-bit packed (4 pixels in 5 bytes, PAK on), 2 = 16-bit words.
type imgoFormat int

var (
	formatEnable bool
	nFrames      = 1
)

func (f imgoFormat) bytesPerLine() int {
	switch f {
	case 1:
		return width * 10 / 8
	case 2:
		return width * 2
	}
	return width
}

func main() {
	out := flag.String("out", "/tmp/frame", "output path stem (.raw and .png are appended)")
	timeout := flag.Duration("timeout", 3*time.Second, "how long to wait for the first frame")
	noCSI := flag.Bool("no-csi", false, "skip the CSI-2 receiver setup (for register experiments)")
	bits := flag.Int("bits", 8, "IMGO output: 8, 10 (packed) or 16 bits per pixel")
	fmtEn := flag.Bool("fmten", false, "also set IMGO_STRIDE.FORMAT/FORMAT_EN for the chosen width")
	frames := flag.Int("frames", 1, "how many frame starts to run through before stopping")
	flag.Parse()
	formatEnable = *fmtEn
	nFrames = *frames

	var f imgoFormat
	switch *bits {
	case 8:
		f = 0
	case 10:
		f = 1
	case 16:
		f = 2
	default:
		fmt.Println("camframe: -bits must be 8, 10 or 16")
		os.Exit(2)
	}
	if err := run(*out, *timeout, !*noCSI, f); err != nil {
		fmt.Println("camframe:", err)
		os.Exit(1)
	}
}

func run(out string, timeout time.Duration, csi bool, format imgoFormat) error {
	bytesPerLine := format.bytesPerLine()
	frameBytes := bytesPerLine * height
	// 1. Devices and register windows. Opening camera-isp turns the ISP/SENINF clocks on.
	isp, err := syscall.Open("/dev/camera-isp", syscall.O_RDWR, 0)
	if err != nil {
		return fmt.Errorf("open /dev/camera-isp: %w", err)
	}
	defer syscall.Close(isp)
	cam, err := mapWindow(isp, ispBase, ispLen, "cam")
	if err != nil {
		return err
	}
	sen, err := mapWindow(isp, seninfBase, seninfLen, "seninf")
	if err != nil {
		return err
	}
	mipi, err := mapWindow(isp, mipiBase, mipiLen, "mipi")
	if err != nil {
		return err
	}
	fmt.Println("register windows mapped")

	sens, err := syscall.Open("/dev/kd_camera_hw", syscall.O_RDWR, 0)
	if err != nil {
		return fmt.Errorf("open /dev/kd_camera_hw: %w", err)
	}
	defer syscall.Close(sens)

	// 2. Sensor master clock: camtg mux to the 48 MHz group, TG1 divider to 24 MHz.
	mclk := [3]uint32{1, 1, 0}
	if err := ioctl(sens, ioc(3, sensMagic, nrSetMCLK, 12), unsafe.Pointer(&mclk[0])); err != nil {
		return fmt.Errorf("SET_MCLK_PLL: %w", err)
	}
	defer func() {
		mclk[0] = 0
		ioctl(sens, ioc(3, sensMagic, nrSetMCLK, 12), unsafe.Pointer(&mclk[0]))
	}()
	setMCLK1(sen, 1)

	// 3. The frame buffer: ION multimedia heap, mapped for the IMGO M4U port, and the port itself
	// switched to translated mode so the DMA address we hand the ISP is the one we own.
	buf, mva, err := allocFrame(2 * maxFrameBytes)
	if err != nil {
		return err
	}
	fmt.Printf("frame buffer: %d bytes (frame %d), device address %#x\n", maxFrameBytes, frameBytes, mva)
	if err := m4uConfig(m4uPortIMGO); err != nil {
		return err
	}
	for i := range buf {
		buf[i] = 0xA5 // sentinel: unchanged bytes mean the DMA never wrote
	}

	// 4. Sensor up: driver select, power + init table over I2C.
	idx := [2]uint32{sensorMain<<dualMainShift | 0, 0}
	if err := ioctl(sens, ioc(3, sensMagic, nrSetDriver, 8), unsafe.Pointer(&idx[0])); err != nil {
		return fmt.Errorf("SET_DRIVER: %w", err)
	}
	cur := uint32(sensorMain)
	ioctl(sens, ioc(3, sensMagic, nrSetCurrent, 4), unsafe.Pointer(&cur))
	if err := ioctl(sens, ioc(0, sensMagic, nrOpen, 0), nil); err != nil {
		return fmt.Errorf("T_OPEN: %w", err)
	}
	defer ioctl(sens, ioc(0, sensMagic, nrClose, 0), nil)
	fmt.Println("sensor open")

	// 5. ISP pass-1 path: TG -> PAK -> IMGO, RAW10 packed, no interrupts to the kernel (we poll,
	// so the driver's ring-buffer code never runs on our frames).
	if err := ioctl(isp, ioc(0, ispMagic, nrIspReset, 0), nil); err != nil {
		fmt.Println("ISP_RESET:", err)
	}
	cam.show("before", regCtlEn1, regCtlDmaEn, regCtlFmtSel, regCtlSel, regCtlMuxSel2, regImgoStride, regImgoCon, regImgoCon2, regTgSenMode, regTgVfCon, regTgGrabPxl, regTgGrabLin, regImgoFbc)
	cam.wr(regCtlIntEn, 0)
	cam.wr(regTgVfCon, 0)
	cam.wr(regCtlClkEn, 0x1FFFF) // every data-path clock, as pass-1 start does
	// TG1_EN | PAK_EN: with IMGO_MUX 0 the DMA is fed by the packer, so without PAK_EN nothing
	// reaches memory at all (seen: zero bytes written for every format). What the packer hands
	// over here is one byte per pixel, 1600 bytes a line.
	cam.wr(regCtlEn1, 1<<0|1<<12)
	cam.wr(regCtlEn2, 0)
	cam.wr(regCtlDmaEn, 1<<0)   // IMGO_EN
	cam.wr(regCtlFmtSel, 1<<16) // TG1_FMT = RAW10
	cam.wr(regCtlSel, 0)
	cam.wr(regCtlPixID, 0)               // Bayer R first
	cam.mask(regCtlMuxSel2, 1<<4, 1<<18) // IMGO_MUX = 0 (from PAK), IMGO_MUX_EN
	cam.wr(regImgoFbc, 0)
	cam.wr(regImgoBase, mva)
	cam.wr(regImgoOfst, 0)
	cam.wr(regImgoXsize, uint32(bytesPerLine-1))
	cam.wr(regImgoYsize, height-1)
	stride := uint32(bytesPerLine)
	if formatEnable {
		stride |= 1<<23 | uint32(format)<<20 // FORMAT_EN, FORMAT
	}
	cam.wr(regImgoStride, stride)
	cam.wr(regCtlImgoSize, uint32(width)<<16|uint32(height))
	if cam.rd(regImgoCon) == 0 {
		cam.wr(regImgoCon, 0x80000040)
		cam.wr(regImgoCon2, 0x00200020)
	}
	cam.wr(regTgSenMode, 1<<0|1<<2) // CMOS_EN, SOT_MODE (setTg1ViewFinderMode)
	cam.wr(regTgVfCon, 1<<12)       // SPDELAY_MODE, continuous
	cam.wr(regTgGrabPxl, uint32(width)<<16)
	cam.wr(regTgGrabLin, uint32(height)<<16)
	cam.wr(regTgPathCfg, 0) // SEN_IN_LSB 0
	cam.rd(regCtlIntSt)     // read-clear
	cam.rd(regCtlDmaInt)
	cam.show("configured", regCtlEn1, regCtlDmaEn, regCtlFmtSel, regCtlMuxSel2, regImgoBase, regImgoXsize, regImgoYsize, regImgoStride, regImgoCon, regTgSenMode, regTgGrabPxl, regTgGrabLin)

	// 6. Receiver: MIPI D-PHY analog, CSI-2 decoder, SENINF mux to the TG.
	if csi {
		if err := setupCSI2(sen, mipi); err != nil {
			return err
		}
	}

	// 7. Stream: sensor preview mode (its stream-on is in the mode table), then the TG's view
	// finder enable. The first frame completes when the IMGO DMA raises its done bit.
	var window, config [256]byte
	ctl := [4]uint32{sensorMain, scenPreview, uint32(uintptr(unsafe.Pointer(&window[0]))), uint32(uintptr(unsafe.Pointer(&config[0])))}
	if err := ioctl(sens, ioc(3, sensMagic, nrControl, 16), unsafe.Pointer(&ctl[0])); err != nil {
		return fmt.Errorf("CONTROL(preview): %w", err)
	}
	fmt.Println("sensor streaming")
	time.Sleep(50 * time.Millisecond)

	// Frame sync comes from the TG's frame counter (TG_INTER_ST bits 23:16): the CAM interrupt
	// status registers stay 0 while INT_EN is 0. The buffer is two halves; whenever the counter
	// moves we point the DMA at the other half, so the half it just left holds a whole frame.
	halves := [2]uint32{mva, mva + uint32(maxFrameBytes)}
	cur = 0
	cam.wr(regImgoBase, halves[cur])
	cam.mask(regTgVfCon, 0, 1) // VFDATA_EN
	deadline := time.Now().Add(timeout)
	start := time.Now()
	lastCnt := cam.rd(regTgInterSt) >> 16 & 0xFF
	frames := 0
	var lastTick time.Time
	var periods []time.Duration
	for time.Now().Before(deadline) && frames < nFrames {
		cnt := cam.rd(regTgInterSt) >> 16 & 0xFF
		if cnt != lastCnt {
			now := time.Now()
			if !lastTick.IsZero() {
				periods = append(periods, now.Sub(lastTick))
			}
			lastTick = now
			lastCnt = cnt
			frames++
			cur ^= 1
			cam.wr(regImgoBase, halves[cur]) // next frame goes to the other half
		}
		time.Sleep(500 * time.Microsecond)
	}
	cam.mask(regTgVfCon, 1, 0)
	elapsed := time.Since(start)
	fmt.Printf("%d frame starts in %v (%.1f fps), periods %v\n", frames, elapsed.Round(time.Millisecond), float64(frames)/elapsed.Seconds(), periods)
	fmt.Printf("int status %08x, dma int %08x, tg inter %08x, tg err %08x dat %08x\n",
		cam.rd(regCtlIntSt), cam.rd(regCtlDmaInt), cam.rd(regTgInterSt), cam.rd(regTgErrCtl), cam.rd(regTgDatNo))
	showCSI2(sen, mipi)
	time.Sleep(60 * time.Millisecond) // let a frame in flight finish before we read

	for h := 0; h < 2; h++ {
		part := buf[h*maxFrameBytes : h*maxFrameBytes+frameBytes]
		written := 0
		for _, b := range part {
			if b != 0xA5 {
				written++
			}
		}
		fmt.Printf("half %d: %d of %d bytes written by the DMA\n", h, written, frameBytes)
	}
	// The half the DMA is aimed at now may be mid-frame; the other one is complete.
	frame := buf[int(cur^1)*maxFrameBytes : int(cur^1)*maxFrameBytes+frameBytes]
	if err := os.WriteFile(out+".raw", frame, 0o644); err != nil {
		return err
	}
	if err := savePreview(out+".png", frame, format); err != nil {
		return err
	}
	other := buf[int(cur)*maxFrameBytes : int(cur)*maxFrameBytes+frameBytes]
	if err := savePreview(out+"-other.png", other, format); err != nil {
		return err
	}
	fmt.Printf("wrote %s.raw (%d bytes, %d bits/pixel), %s.png and %s-other.png\n", out, frameBytes, []int{8, 10, 16}[format], out, out)
	if frames == 0 {
		return errors.New("no frame: the TG frame counter never moved")
	}
	return nil
}

// setupCSI2 is SeninfDrvImp's MIPI bring-up for one sensor on SENINF1/CSI0 feeding TG1, in the
// order HalSensor.control.cpp uses it: analog D-PHY on, HSRX offset calibration, SENINF1 mux
// and source select, the NCSI2 receiver (1 data lane, ECC order 1, settle 85 ns at the 364 MHz
// ISP clock), then the top mux TG1 <- SENINF1.
func setupCSI2(s, ana *mmio) error {
	// D-PHY analog, CSI0 instance.
	ana.mask(0x4C, ^uint32(0xFEFBEFBE), 0) // GPI*_IES off: pads in MIPI mode
	ana.mask(0x50, ^uint32(0xFEFBEFBE), 0)
	for _, off := range []uint32{0x00, 0x04, 0x08, 0x0C, 0x10} {
		ana.mask(off, 0, 1<<3) // lane input select = MIPI
	}
	ana.mask(0x24, 0, 1) // bandgap core
	time.Sleep(30 * time.Microsecond)
	ana.mask(0x20, 0, 3) // LDO core, HSRX byte clock invert
	time.Sleep(time.Microsecond)
	for _, off := range []uint32{0x00, 0x04, 0x08, 0x0C, 0x10} {
		ana.mask(off, 0, 1) // lane LDO out
	}
	// HSRX offset calibration.
	s.wr(regNcsi2HsrxDbg, 0x1F)
	s.mask(regMipiRxCon38, 0, 1)
	s.wr(regMipiRxCon3C, 0x1541)
	s.mask(regMipiRxCon38, 0, 1<<2)
	time.Sleep(500 * time.Microsecond)
	c44, c48 := s.rd(regMipiRxCon44), s.rd(regMipiRxCon48)
	if c44&0x10001 != 0 && c48&0x101 != 0 {
		fmt.Printf("hsrx calibration ok (%08x %08x)\n", c44, c48)
	} else {
		fmt.Printf("hsrx calibration did not apply (%08x %08x), continuing\n", c44, c48)
	}
	s.mask(regMipiRxCon38, 1, 0)
	s.wr(regNcsi2HsrxDbg, 0)

	// SENINF1 mux: enable, source 8 (MIPI sensor), 1-pixel mode, FIFO settings for non-JPEG
	// MIPI, sync polarities low.
	mux := s.rd(regSeninf1Mux)
	mux &^= 0xF<<12 | 1<<8 | 0x3<<28 | 0x3F<<22 | 0x3F<<16 | 1<<10 | 1<<9
	mux |= 1<<31 | 8<<12 | 1<<28 | 0x3B<<22 | 0x3F<<16
	s.wr(regSeninf1Mux, mux)
	// SENINF1 control: enable, PAD2CAM 10-bit, source 8.
	s.mask(regSeninf1Ctrl, 0x7<<28|0xF<<12, 1|8<<12)

	// NCSI2 receiver.
	s.wr(regNcsi2Dpcm, 0)
	s.wr(regSeninf1Ctrl, s.rd(regSeninf1Ctrl)&0xFFFF0FFF|0x8000)
	s.mask(regNcsi2Ctl, 0, 1<<7) // HSRX_DET_EN (settle-delay mode auto)
	settle := uint32(85*364/1000) & 0xFF
	s.wr(regNcsi2LnrdTim, settle<<8)
	s.mask(regNcsi2Ctl, 0, 1<<26|1<<16|1<<4|1) // REF_SYNC_DET_EN, ED_SEL (ECC order 1), clock lane, data lane 0
	s.wr(regNcsi2IntEn, 0)                     // status bits only; no interrupt line to a kernel that has no handler
	s.mask(regSeninf1Mux, 0, 3)                // mux + irq soft reset pulse
	s.mask(regSeninf1Mux, 3, 0)
	// Top mux: TG1 takes SENINF1.
	s.mask(regSeninfTopMux, 0xF, 0)
	s.show("csi2 set", regSeninfTopCtrl, regSeninfTopMux, regSeninf1Ctrl, regSeninf1Mux, regNcsi2Ctl, regNcsi2LnrdTim, regTG1PhCnt, regTG1SenCk)
	return nil
}

// showCSI2 prints what the receiver saw: frame/line counters, lane state machines, mux size.
func showCSI2(s, ana *mmio) {
	s.show("csi2 status", regNcsi2IntSt, regNcsi2FrameLine, regNcsi2LnrcFsm, regNcsi2LnrdFsm, regSeninf1MuxInt, regSeninf1MuxSize)
	ana.show("analog", 0x00, 0x04, 0x20, 0x24, 0x4C, 0x50)
}

// allocFrame takes size bytes from the ION multimedia heap, maps them uncached for the CPU, and
// resolves the IMGO port's device address (an M4U MVA).
func allocFrame(size int) ([]byte, uint32, error) {
	ion, err := syscall.Open("/dev/ion", syscall.O_RDWR, 0)
	if err != nil {
		return nil, 0, fmt.Errorf("open /dev/ion: %w", err)
	}
	// Keep the client open for the buffer's life: closing it frees the handle.
	a := ionAlloc{Len: uint32(size), Align: 4096, HeapIDMask: 1 << ionHeapMultimedia}
	if err := ioctl(ion, ioc(3, 'I', 0, unsafe.Sizeof(a)), unsafe.Pointer(&a)); err != nil {
		return nil, 0, fmt.Errorf("ion alloc: %w", err)
	}
	share := ionFd{Handle: a.Handle}
	if err := ioctl(ion, ioc(3, 'I', 4, unsafe.Sizeof(share)), unsafe.Pointer(&share)); err != nil {
		return nil, 0, fmt.Errorf("ion share: %w", err)
	}
	mem, err := unix.Mmap(int(share.Fd), 0, size, unix.PROT_READ|unix.PROT_WRITE, unix.MAP_SHARED)
	if err != nil {
		return nil, 0, fmt.Errorf("mmap ion buffer: %w", err)
	}
	cfg := ionMMData{MMCmd: ionMMConfigBuffer, Config: ionMMConfig{Handle: a.Handle, ModuleID: m4uPortIMGO}}
	c := ionCustom{Cmd: ionCmdMultimedia, Arg: uint32(uintptr(unsafe.Pointer(&cfg)))}
	if err := ioctl(ion, ioc(3, 'I', 6, unsafe.Sizeof(c)), unsafe.Pointer(&c)); err != nil {
		return nil, 0, fmt.Errorf("ion config buffer for IMGO: %w", err)
	}
	phys := ionSysData{SysCmd: ionSysGetPhys, Phys: ionSysGetPhysP{Handle: a.Handle}}
	c = ionCustom{Cmd: ionCmdSystem, Arg: uint32(uintptr(unsafe.Pointer(&phys)))}
	if err := ioctl(ion, ioc(3, 'I', 6, unsafe.Sizeof(c)), unsafe.Pointer(&c)); err != nil {
		return nil, 0, fmt.Errorf("ion get phys: %w", err)
	}
	if phys.Phys.PhyAddr == 0 {
		return nil, 0, errors.New("ion gave no device address for the IMGO port")
	}
	return mem, phys.Phys.PhyAddr, nil
}

// m4uConfig puts one M4U port into translated (virtual) mode, MTK_M4U_T_CONFIG_PORT on /proc/m4u.
func m4uConfig(port int32) error {
	fd, err := syscall.Open("/proc/m4u", syscall.O_RDWR, 0)
	if err != nil {
		return fmt.Errorf("open /proc/m4u: %w", err)
	}
	defer syscall.Close(fd)
	p := m4uPort{PortID: port, Virtuality: 1, Distance: 1}
	if err := ioctl(fd, ioc(1, 'g', 11, 4), unsafe.Pointer(&p)); err != nil {
		return fmt.Errorf("m4u config port %d: %w", port, err)
	}
	fmt.Printf("m4u port %d set to translated mode\n", port)
	return nil
}

// unpackLine converts one DMA line to 16-bit samples in dst (len width) for the given format.
// Packed RAW10 is 4 pixels in 5 bytes, LSB first; 16-bit words are little-endian.
func unpackLine(line []byte, dst []uint16, format imgoFormat) {
	switch format {
	case 0:
		for i := range dst {
			dst[i] = uint16(line[i])
		}
	case 1:
		for i, j := 0, 0; i+5 <= len(line) && j+4 <= len(dst); i, j = i+5, j+4 {
			b0, b1, b2, b3, b4 := uint16(line[i]), uint16(line[i+1]), uint16(line[i+2]), uint16(line[i+3]), uint16(line[i+4])
			dst[j] = b0 | (b1&3)<<8
			dst[j+1] = b1>>2 | (b2&0xF)<<6
			dst[j+2] = b2>>4 | (b3&0x3F)<<4
			dst[j+3] = b3>>6 | b4<<2
		}
	case 2:
		for i := range dst {
			dst[i] = uint16(line[2*i]) | uint16(line[2*i+1])<<8
		}
	}
}

// savePreview writes a half-size colour PNG: each 2x2 Bayer cell (R G / G B, the sensor's
// "R first" order) becomes one pixel, and the levels are stretched so the brightest one
// percent of cells hit white. It is a look, not a proper demosaic.
func savePreview(path string, raw []byte, format imgoFormat) error {
	bpl := format.bytesPerLine()
	w, h := width/2, height/2
	cells := make([][3]uint32, w*h)
	row0 := make([]uint16, width)
	row1 := make([]uint16, width)
	var hist [65536]int
	for y := 0; y < h; y++ {
		unpackLine(raw[(2*y)*bpl:(2*y+1)*bpl], row0, format)
		unpackLine(raw[(2*y+1)*bpl:(2*y+2)*bpl], row1, format)
		for x := 0; x < w; x++ {
			r := uint32(row0[2*x])
			g := (uint32(row0[2*x+1]) + uint32(row1[2*x])) / 2
			b := uint32(row1[2*x+1])
			cells[y*w+x] = [3]uint32{r, g, b}
			hist[g]++
		}
	}
	// 99th percentile of green sets white.
	white, seen := uint32(1), 0
	for v := 0; v < len(hist); v++ {
		seen += hist[v]
		if seen >= w*h*99/100 {
			white = uint32(v)
			break
		}
	}
	if white == 0 {
		white = 1
	}
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for i, c := range cells {
		for k := 0; k < 3; k++ {
			v := c[k] * 255 / white
			if v > 255 {
				v = 255
			}
			img.Pix[i*4+k] = uint8(v)
		}
		img.Pix[i*4+3] = 255
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, img)
}
