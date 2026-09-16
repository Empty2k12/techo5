//go:build linux

// spotcam is a bench probe for the Echo Spot's GC0312 camera: power it on the sub socket, program
// the parallel sensor interface (SENINF4 -> mux 1 -> TG1) and the IMGO DMA, and dump raw frames.
// It is the Spot's counterpart of the Show's removed camframe; the daemon's hardware/camera is the
// place the working sequence ends up.
//
//	spotcam [-frames 5] [-out /tmp/spotcam] [-pad 4] [-pclkinv 0] [-hpol 1] [-vpol 1] [-tgfmt 0]
package main

import (
	"flag"
	"fmt"
	"os"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/unix"
)

const (
	sensMagic   = 'i'
	nrOpen      = 0
	nrControl   = 20
	nrClose     = 25
	nrSetDriver = 35
	nrSetMCLK   = 60
	nrSetCur    = 70
	nrFeature   = 15
	nrGetInfo2  = 65
	socketSub   = 2 // DUAL_CAMERA_SUB_SENSOR: the rook board powers the GC0312 as pin set 1

	ispMagic   = 'k'
	nrIspReset = 0

	ispBase, ispLen       = 0x15000000, 0x10000
	seninfBase, seninfLen = 0x15008000, 0x4000
	mipiBase, mipiLen     = 0x10217000, 0x3000

	ionHeapMultimedia = 10
	m4uPortIMGO       = 24
)

type ionAlloc struct {
	Len, Align, HeapIDMask, Flags uint32
	Handle                        int32
}
type ionFd struct{ Handle, Fd int32 }
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
	if _, _, e := syscall.Syscall(syscall.SYS_IOCTL, uintptr(fd), req, uintptr(arg)); e != 0 {
		return e
	}
	return nil
}

type mmio struct{ mem []byte }

func mapWindow(fd int, base, length int64) *mmio {
	mem, err := syscall.Mmap(fd, base, int(length), syscall.PROT_READ|syscall.PROT_WRITE, syscall.MAP_SHARED)
	must(err, "mmap %#x", base)
	return &mmio{mem: mem}
}
func (m *mmio) rd(off uint32) uint32     { return *(*uint32)(unsafe.Pointer(&m.mem[off])) }
func (m *mmio) wr(off, val uint32)       { *(*uint32)(unsafe.Pointer(&m.mem[off])) = val }
func (m *mmio) mask(off, clr, set uint32) { m.wr(off, m.rd(off)&^clr|set) }

func must(err error, f string, a ...any) {
	if err != nil {
		fmt.Fprintf(os.Stderr, "spotcam: "+f+": %v\n", append(a, err)...)
		os.Exit(1)
	}
}

func main() {
	frames := flag.Int("frames", 5, "frames to dump")
	out := flag.String("out", "/tmp/spotcam", "file prefix")
	pad := flag.Uint("pad", 4, "PAD2CAM_DATA_SEL: 4 = 8 bits on data 9..2, 3 = 7..0, 0 = 10 bits")
	pclkInv := flag.Uint("pclkinv", 0, "TG1 PADCLK_INV")
	hpol := flag.Uint("hpol", 1, "mux HSYNC_POL")
	vpol := flag.Uint("vpol", 1, "mux VSYNC_POL")
	tgfmt := flag.Uint("tgfmt", 1, "CAM_CTL_FMT_SEL TG1_FMT (0 RAW8, 1 RAW10)")
	w := flag.Uint("w", 640, "width")
	h := flag.Uint("h", 480, "height")
	topCtrl := flag.Int("topctrl", -1, "SENINF_TOP_CTRL value to write after the clock setup (-1 leaves it)")
	hsMask := flag.Uint("hsmask", 0, "mux HSYNC_MASK")
	wait := flag.Duration("wait", 4*time.Second, "how long to wait for frames")
	powerOnly := flag.Bool("power", false, "only power the sensor and read its id, then close")
	pattern := flag.Bool("pattern", false, "ask the sensor for its test pattern")
	flag.Parse()

	isp, err := syscall.Open("/dev/camera-isp", syscall.O_RDWR, 0)
	must(err, "open camera-isp")
	cam := mapWindow(isp, ispBase, ispLen)
	sen := mapWindow(isp, seninfBase, seninfLen)
	ana := mapWindow(isp, mipiBase, mipiLen)
	sens, err := syscall.Open("/dev/kd_camera_hw", syscall.O_RDWR, 0)
	must(err, "open kd_camera_hw")

	mclk := [3]uint32{1, 1, 0}
	must(ioctl(sens, ioc(3, sensMagic, nrSetMCLK, 12), unsafe.Pointer(&mclk[0])), "SET_MCLK_PLL")
	setMCLK1(sen, 1, uint32(*pclkInv))

	var buf []byte
	var mva uint32
	W, H := uint32(*w), uint32(*h)
	frameBytes := int(W * H)
	if !*powerOnly {
		buf, mva = allocION(frameBytes)
		m4u, err := syscall.Open("/proc/m4u", syscall.O_RDWR, 0)
		must(err, "open /proc/m4u")
		p := m4uPort{PortID: m4uPortIMGO, Virtuality: 1, Distance: 1}
		must(ioctl(m4u, ioc(1, 'g', 11, 4), unsafe.Pointer(&p)), "m4u config port")
		syscall.Close(m4u)
		fmt.Printf("ion buffer %d bytes at mva %#x\n", frameBytes, mva)
	}

	idx := [2]uint32{socketSub << 16, 0}
	must(ioctl(sens, ioc(3, sensMagic, nrSetDriver, 8), unsafe.Pointer(&idx[0])), "SET_DRIVER")
	cur := uint32(socketSub)
	ioctl(sens, ioc(3, sensMagic, nrSetCur, 4), unsafe.Pointer(&cur))
	must(ioctl(sens, ioc(0, sensMagic, nrOpen, 0), nil), "sensor open")
	fmt.Println("sensor open ok")
	defer func() {
		cam.mask(0x4414, 1, 0)
		ioctl(sens, ioc(0, sensMagic, nrClose, 0), nil)
		mclk[0] = 0
		ioctl(sens, ioc(3, sensMagic, nrSetMCLK, 12), unsafe.Pointer(&mclk[0]))
		fmt.Println("closed")
	}()
	if *powerOnly {
		return
	}

	// ISP pass 1: TG1 -> packer -> IMGO, one byte a pixel.
	ioctl(isp, ioc(0, ispMagic, nrIspReset, 0), nil)
	cam.wr(0x4020, 0)       // INT_EN
	cam.wr(0x4414, 0)       // VF off
	cam.wr(0x4150, 0x1FFFF) // CLK_EN
	cam.wr(0x4004, 1<<0|1<<12)
	cam.wr(0x4008, 0)
	cam.wr(0x400C, 1)
	cam.wr(0x4010, uint32(*tgfmt)<<16) // CAM_OUT_FMT 0: 8 bits
	cam.wr(0x4018, 0)
	cam.wr(0x401C, 0)
	cam.mask(0x4078, 1<<4, 1<<18)
	cam.wr(0x40F4, 0)
	cam.wr(0x4300, mva)
	cam.wr(0x4304, 0)
	cam.wr(0x4308, W-1)
	cam.wr(0x430C, H-1)
	cam.wr(0x4310, W)
	cam.wr(0x414C, W<<16|H)
	if cam.rd(0x4314) == 0 {
		cam.wr(0x4314, 0x80000040)
		cam.wr(0x4318, 0x00200020)
	}
	cam.wr(0x4410, 1<<0|1<<2)
	cam.wr(0x4414, 1<<12)
	cam.wr(0x4418, W<<16)
	cam.wr(0x441C, H<<16)
	cam.wr(0x4420, 0)

	// Parallel input: pads as GPI (setSeninf4Parallel), SENINF4 parallel source, mux 1 fed by it.
	for _, base := range []uint32{0x0000, 0x1000, 0x2000} {
		ana.mask(base+0x4C, 0, 0x1041041)
		ana.mask(base+0x50, 0, 0x1041041)
	}
	sen.mask(0x0D00, 0x7<<28|0xF<<12, 1|3<<12|uint32(*pad)<<28)
	m4 := sen.rd(0x0D20)
	sen.wr(0x0D20, m4|3)
	sen.wr(0x0D20, m4&^3)
	mux := sen.rd(0x0120)
	mux &^= 0xF<<12 | 1<<8 | 0x3<<28 | 0x3F<<22 | 0x3F<<16 | 1<<10 | 1<<9
	mux &^= 1 << 7
	mux |= 1<<31 | 3<<12 | 1<<28 | 0x1B<<22 | 0x1F<<16 | uint32(*hpol)<<10 | uint32(*vpol)<<9 | uint32(*hsMask)<<7
	sen.wr(0x0120, mux)
	sen.mask(0x0120, 0, 3)
	sen.mask(0x0120, 3, 0)
	sen.mask(0x0008, 0xF, 3) // TG1 <- SENINF4
	if *topCtrl >= 0 {
		sen.wr(0x0000, uint32(*topCtrl))
	}
	fmt.Printf("SENINF4_CTRL %#08x SENINF1_MUX %#08x TOP_MUX %#08x TOP_CTRL %#08x TG1_PH %#08x TG1_CK %#08x\n",
		sen.rd(0x0D00), sen.rd(0x0120), sen.rd(0x0008), sen.rd(0x0000), sen.rd(0x0200), sen.rd(0x0204))

	var window, config [256]byte
	ctl := [4]uint32{socketSub, 0, uint32(uintptr(unsafe.Pointer(&window[0]))), uint32(uintptr(unsafe.Pointer(&config[0])))}
	must(ioctl(sens, ioc(3, sensMagic, nrControl, 16), unsafe.Pointer(&ctl[0])), "sensor control")
	if *pattern {
		setFeature(sens, 3000+37, 1) // SENSOR_FEATURE_SET_TEST_PATTERN, checked below in the log
	}

	cam.mask(0x4414, 0, 1)
	last := cam.rd(0x444C) >> 16 & 0xFF
	got := 0
	deadline := time.Now().Add(*wait)
	for got < *frames && time.Now().Before(deadline) {
		cnt := cam.rd(0x444C) >> 16 & 0xFF
		if cnt == last {
			time.Sleep(2 * time.Millisecond)
			continue
		}
		last = cnt
		got++
		if got == 1 {
			continue
		}
		name := fmt.Sprintf("%s-%d.raw", *out, got)
		must(os.WriteFile(name, buf[:frameBytes], 0o644), "write")
		var sum, maxv int
		for _, b := range buf[:frameBytes] {
			sum += int(b)
			if int(b) > maxv {
				maxv = int(b)
			}
		}
		fmt.Printf("frame %d (tg count %d): mean %d max %d -> %s\n", got, cnt, sum/frameBytes, maxv, name)
	}
	fmt.Printf("TG_INTER_ST %#08x SENINF1_MUX_INTSTA %#08x SENINF4_CTRL %#08x SEN1MUX %#08x\n",
		cam.rd(0x444C), sen.rd(0x0128), sen.rd(0x0D00), sen.rd(0x0120))
	for off := uint32(0x012C); off <= 0x0148; off += 4 {
		fmt.Printf("mux1[%#x]=%#08x ", off, sen.rd(off))
	}
	fmt.Println()
	for off := uint32(0x0D2C); off <= 0x0D48; off += 4 {
		fmt.Printf("mux4[%#x]=%#08x ", off, sen.rd(off))
	}
	fmt.Println()
	if got == 0 {
		fmt.Println("no TG frame ticks")
		var sum, nz int
		for _, v := range buf[:frameBytes] {
			sum += int(v)
			if v != 0 {
				nz++
			}
		}
		fmt.Printf("buffer anyway: mean %d nonzero %d\n", sum/frameBytes, nz)
		os.WriteFile(*out+"-any.raw", buf[:frameBytes], 0o644)
	}
}

func setMCLK1(s *mmio, clkcnt, padInv uint32) {
	clkfPol := uint32(0)
	if clkcnt&1 == 0 {
		clkfPol = 1
	}
	clkfEdge := uint32(1)
	if clkcnt > 1 {
		clkfEdge = (clkcnt + 1) >> 1
	}
	s.mask(0x0200, 1<<31, 1<<31)
	s.mask(0x0000, 0xc00, 0x300)
	s.mask(0x0204, 0x3f|0x3f00|0x3f0000, clkfEdge|clkcnt<<16)
	s.mask(0x0200, 0x3|0x4|1<<6|1<<28, 1|clkfPol<<2|padInv<<6|1<<28)
	s.mask(0x0120, 1<<31, 1<<31)
	s.mask(0x0100, 1<<31, 1)
	s.mask(0x0200, 1<<29, 1<<29)
}

func setFeature(sens int, id uint32, v uint64) {
	var para [8]byte
	for i := range para {
		para[i] = byte(v >> (8 * i))
	}
	size := uint32(len(para))
	ctl := [4]uint32{socketSub, id, uint32(uintptr(unsafe.Pointer(&para[0]))), uint32(uintptr(unsafe.Pointer(&size)))}
	if err := ioctl(sens, ioc(3, sensMagic, nrFeature, 16), unsafe.Pointer(&ctl[0])); err != nil {
		fmt.Println("feature", id, err)
	}
}

func allocION(size int) ([]byte, uint32) {
	ion, err := syscall.Open("/dev/ion", syscall.O_RDWR, 0)
	must(err, "open ion")
	size = (size + 4095) &^ 4095
	a := ionAlloc{Len: uint32(size), Align: 4096, HeapIDMask: 1 << ionHeapMultimedia}
	must(ioctl(ion, ioc(3, 'I', 0, unsafe.Sizeof(a)), unsafe.Pointer(&a)), "ion alloc")
	share := ionFd{Handle: a.Handle}
	must(ioctl(ion, ioc(3, 'I', 4, unsafe.Sizeof(share)), unsafe.Pointer(&share)), "ion share")
	buf, err := unix.Mmap(int(share.Fd), 0, size, unix.PROT_READ|unix.PROT_WRITE, unix.MAP_SHARED)
	must(err, "mmap ion")
	cfg := ionMMData{MMCmd: 0, Config: ionMMConfig{Handle: a.Handle, ModuleID: m4uPortIMGO}}
	c := ionCustom{Cmd: 1, Arg: uint32(uintptr(unsafe.Pointer(&cfg)))}
	must(ioctl(ion, ioc(3, 'I', 6, unsafe.Sizeof(c)), unsafe.Pointer(&c)), "ion config")
	phys := ionSysData{SysCmd: 1, Phys: ionSysGetPhysP{Handle: a.Handle}}
	c = ionCustom{Cmd: 0, Arg: uint32(uintptr(unsafe.Pointer(&phys)))}
	must(ioctl(ion, ioc(3, 'I', 6, unsafe.Sizeof(c)), unsafe.Pointer(&c)), "ion phys")
	return buf, phys.Phys.PhyAddr
}
