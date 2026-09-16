//go:build linux

// camprobe talks to the Echo Show's camera sensor through the MediaTek imgsensor driver, the
// way Android's camera HAL starts: turn on the sensor's master clock, pick the driver, open it
// (which powers the sensor and runs its init table over I2C), ask what it is and what it can
// do, close it. No frame — this is the first step of docs/porting-plan.md item 9, and it says
// whether the sensor is alive from Linux at all.
//
//	camprobe [-drv N] [-mclk G] [-div N]
//
// The kernel never programs the sensor clock divider itself (only a board with
// "cmmclk-always-on" in its device tree gets that at probe, and cronos does not); Android's
// libcamdrv does it from userspace through the ISP driver's raw register write. Without it
// the CMMCLK pad is silent and an OmniVision sensor will not answer I2C, so this probe does
// the same writes to the SENINF timing generator before it opens the sensor.
//
// The daemon is 32-bit and the kernel 64-bit, so the structs here are the compat layouts.
package main

import (
	"encoding/binary"
	"flag"
	"fmt"
	"os"
	"syscall"
	"unsafe"
)

const (
	magic = 'i'

	// _IO / _IOWR numbers, from kd_imgsensor.h.
	nrOpen        = 0
	nrClose       = 25
	nrSetDriver   = 35
	nrSetMCLK     = 60
	nrGetInfo2    = 65
	nrSetCurrent  = 70
	dualMainShift = 16 // DUAL_CAMERA_MAIN_SENSOR = 1 in the upper half of the index

	// /dev/camera-isp, magic 'k': ISP_CMD_READ_REG = 2, ISP_CMD_WRITE_REG = 3, both _IOWR
	// with the compat ISP_REG_IO_STRUCT {u32 pData; u32 Count}. Register addresses are
	// offsets from CAMINF (0x15000000); the driver accepts 0x4000 ≤ off < 0x10000.
	ispMagic   = 'k'
	nrIspRead  = 2
	nrIspWrite = 3

	// SENINF block at CAMINF + 0x8000 (ISP_ADDR + 0x4000 in camera_isp.c).
	regSeninfTopCtrl = 0x8000
	regSeninf1Ctrl   = 0x8100
	regSeninf1Mux    = 0x8120
	regTG1PhCnt      = 0x8200
	regTG1SenCk      = 0x8204
)

func ioc(dir, nr, size uint32) uintptr {
	return uintptr(dir<<30 | size<<16 | uint32(magic)<<8 | nr)
}

func iocISP(nr, size uint32) uintptr {
	return uintptr(3<<30 | size<<16 | uint32(ispMagic)<<8 | nr)
}

func ioctl(fd int, req uintptr, arg unsafe.Pointer) error {
	_, _, e := syscall.Syscall(syscall.SYS_IOCTL, uintptr(fd), req, uintptr(arg))
	if e != 0 {
		return e
	}
	return nil
}

// ispReg is the compat ISP_REG_STRUCT {u32 Addr; u32 Val}.
type ispReg struct {
	Addr, Val uint32
}

func ispRead(fd int, off uint32) (uint32, error) {
	r := ispReg{Addr: off}
	io := [2]uint32{uint32(uintptr(unsafe.Pointer(&r))), 1}
	if err := ioctl(fd, iocISP(nrIspRead, 8), unsafe.Pointer(&io[0])); err != nil {
		return 0, err
	}
	return r.Val, nil
}

func ispWrite(fd int, off, val uint32) error {
	r := ispReg{Addr: off, Val: val}
	io := [2]uint32{uint32(uintptr(unsafe.Pointer(&r))), 1}
	return ioctl(fd, iocISP(nrIspWrite, 8), unsafe.Pointer(&io[0]))
}

// ispMask mirrors camera_isp.c's isp_wr32_mask: clear mask, or in value << shift.
func ispMask(fd int, off, mask, shift, val uint32) error {
	v, err := ispRead(fd, off)
	if err != nil {
		return err
	}
	v &^= mask
	v |= val << shift
	return ispWrite(fd, off, v)
}

// setMCLK1 is ISP_set_mclk1(clkcnt) plus ISP_MCLK1_EN(1) from camera_isp.c: the CMMCLK pad
// runs at camtg / (clkcnt + 1). With the 48 MHz camtg group, clkcnt 1 gives the 24 MHz the
// OV02B10 driver asks for (imgsensor_info.mclk = 24).
func setMCLK1(fd int, clkcnt uint32) error {
	clkfPol := uint32(0)
	if clkcnt&1 == 0 {
		clkfPol = 1
	}
	clkfEdge := uint32(1)
	if clkcnt > 1 {
		clkfEdge = (clkcnt + 1) >> 1
	}
	steps := []struct{ off, mask, shift, val uint32 }{
		{regTG1PhCnt, 1 << 31, 31, 1},       // PCEN
		{regSeninfTopCtrl, 0xc00, 0, 0x300}, // clear top clock gating
		{regTG1SenCk, 0x3f, 0, clkfEdge},    // CLKFL
		{regTG1SenCk, 0x3f00, 8, 0},         // CLKRS
		{regTG1SenCk, 0x3f0000, 16, clkcnt}, // CLKCNT
		{regTG1PhCnt, 0x3, 0, 1},            // TGCLK_SEL
		{regTG1PhCnt, 0x4, 2, clkfPol},      // CLKFL_POL
		{regTG1PhCnt, 1 << 6, 6, 0},         // PADCLK_INV
		{regTG1PhCnt, 1 << 28, 28, 0},       // CLK_POL
		{regSeninf1Mux, 1 << 31, 31, 1},     // SENINF1_MUX_EN
		{regSeninf1Ctrl, 1 << 31, 0, 1},     // SENINF1_EN (as the kernel writes it)
		{regTG1PhCnt, 1 << 29, 29, 1},       // ISP_MCLK1_EN
	}
	for _, s := range steps {
		if err := ispMask(fd, s.off, s.mask, s.shift, s.val); err != nil {
			return fmt.Errorf("reg %#x: %w", s.off, err)
		}
	}
	return nil
}

func dumpSeninf(fd int, tag string) {
	fmt.Printf("SENINF %s:", tag)
	for _, off := range []uint32{regSeninfTopCtrl, regSeninf1Ctrl, regSeninf1Mux, regTG1PhCnt, regTG1SenCk} {
		v, err := ispRead(fd, off)
		if err != nil {
			fmt.Printf(" %#x=err(%v)", off, err)
			continue
		}
		fmt.Printf(" %#x=%08x", off, v)
	}
	fmt.Println()
}

func main() {
	drv := flag.Int("drv", 0, "sensor driver index in the kernel's list")
	group := flag.Int("mclk", 1, "master clock group: 1 = 48 MHz, 2 = 52 MHz")
	div := flag.Int("div", 1, "SENINF TG1 clock count: pad clock = group / (div+1); -1 skips the register setup")
	flag.Parse()

	// The ISP node first: opening it powers the camera block the sensor's clock comes from.
	isp, err := syscall.Open("/dev/camera-isp", syscall.O_RDWR, 0)
	if err != nil {
		fmt.Println("open /dev/camera-isp:", err)
		isp = -1
	} else {
		defer syscall.Close(isp)
		fmt.Println("opened /dev/camera-isp")
	}

	fd, err := syscall.Open("/dev/kd_camera_hw", syscall.O_RDWR, 0)
	if err != nil {
		fmt.Println("open /dev/kd_camera_hw:", err)
		os.Exit(1)
	}
	defer syscall.Close(fd)
	fmt.Println("opened /dev/kd_camera_hw")

	// SET_MCLK_PLL: the camtg clock mux (48 or 52 MHz group) the SENINF timing generator
	// divides down for the sensor. The struct on cronos is the default 12-byte layout
	// {u8 on; u32 freq; u8 TG}; the checkers 8-byte one is also accepted.
	var mclk [3]uint32
	mclk[0] = 1
	mclk[1] = uint32(*group)
	mclkSize := uint32(12)
	if err := ioctl(fd, ioc(3, nrSetMCLK, 12), unsafe.Pointer(&mclk[0])); err != nil {
		fmt.Println("SET_MCLK_PLL on (12 bytes):", err)
		mclkSize = 8
		if err := ioctl(fd, ioc(3, nrSetMCLK, 8), unsafe.Pointer(&mclk[0])); err != nil {
			fmt.Println("SET_MCLK_PLL on (8 bytes):", err)
			mclkSize = 0
		}
	}
	if mclkSize != 0 {
		fmt.Printf("SET_MCLK_PLL on ok (group %d, %d-byte struct)\n", *group, mclkSize)
		defer func() {
			mclk[0] = 0
			if err := ioctl(fd, ioc(3, nrSetMCLK, mclkSize), unsafe.Pointer(&mclk[0])); err == nil {
				fmt.Println("SET_MCLK_PLL off ok")
			}
		}()
	}

	// The sensor clock divider: what Android's libcamdrv writes and the kernel does not.
	if isp >= 0 {
		dumpSeninf(isp, "before")
		if *div >= 0 {
			if err := setMCLK1(isp, uint32(*div)); err != nil {
				fmt.Println("SENINF TG1 setup:", err)
			} else {
				fmt.Printf("SENINF TG1 set: pad clock = group / %d\n", *div+1)
			}
			dumpSeninf(isp, "after")
		}
	}

	// SENSOR_DRIVER_INDEX_STRUCT: two u32, the first for the main socket.
	var idx [2]uint32
	idx[0] = uint32(1<<dualMainShift) | uint32(*drv)
	if err := ioctl(fd, ioc(3, nrSetDriver, 8), unsafe.Pointer(&idx[0])); err != nil {
		fmt.Println("SET_DRIVER:", err)
		os.Exit(1)
	}
	fmt.Printf("SET_DRIVER ok: index %d -> %#x\n", *drv, idx[0])

	cur := uint32(1) // DUAL_CAMERA_MAIN_SENSOR
	if err := ioctl(fd, ioc(3, nrSetCurrent, 4), unsafe.Pointer(&cur)); err != nil {
		fmt.Println("SET_CURRENT_SENSOR:", err)
	}

	// T_OPEN powers the sensor and runs the driver's init sequence over I2C.
	if err := ioctl(fd, ioc(0, nrOpen, 0), nil); err != nil {
		fmt.Println("T_OPEN:", err)
		if isp >= 0 {
			dumpSeninf(isp, "after T_OPEN")
		}
		os.Exit(1)
	}
	fmt.Println("T_OPEN ok: sensor powered and initialised")
	if isp >= 0 {
		dumpSeninf(isp, "after T_OPEN")
	}

	// GETINFO2: {u32 SensorId; ptr info; ptr resolution}. SensorId is an input — 1 picks the
	// main socket's info (0 would hand back the empty sub-socket arrays). CHECK_IS_ALIVE is
	// not used here: it power-cycles the sensor on its own, which would undo T_OPEN.
	// GETRESOLUTION2 is left out too: its compat path allocates sizeof(pointer) bytes for a
	// 16-byte struct and scribbles over the caller's stack.
	info := make([]byte, 4096)
	res := make([]byte, 1024)
	var gi [3]uint32
	gi[0] = 1 // DUAL_CAMERA_MAIN_SENSOR
	gi[1] = uint32(uintptr(unsafe.Pointer(&info[0])))
	gi[2] = uint32(uintptr(unsafe.Pointer(&res[0])))
	if err := ioctl(fd, ioc(3, nrGetInfo2, 12), unsafe.Pointer(&gi[0])); err != nil {
		fmt.Println("GETINFO2:", err)
	} else {
		le := binary.LittleEndian
		fmt.Printf("GETINFO2 ok: preview %dx%d full %dx%d, mclk %d MHz, interface %d, format %d, mipi lanes %d+1\n",
			le.Uint16(info[0:]), le.Uint16(info[2:]), le.Uint16(info[4:]), le.Uint16(info[6:]),
			info[8], le.Uint32(info[32:]), le.Uint32(info[36:]), le.Uint32(info[40:]))
		names := []string{"preview", "full", "video", "hs video", "slim video", "custom1", "custom2"}
		for i, n := range names {
			fmt.Printf("  %-10s %4d x %4d\n", n, le.Uint16(res[i*4:]), le.Uint16(res[i*4+2:]))
		}
	}

	if err := ioctl(fd, ioc(0, nrClose, 0), nil); err != nil {
		fmt.Println("T_CLOSE:", err)
	} else {
		fmt.Println("T_CLOSE ok: sensor powered down")
	}
}
