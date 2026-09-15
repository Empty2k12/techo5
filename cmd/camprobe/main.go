//go:build linux

// camprobe talks to the Echo Show's camera sensor through the MediaTek imgsensor driver, the
// way Android's camera HAL starts: pick the driver, open it (which powers the sensor and runs
// its init table over I2C), ask what it is and what it can do, close it. No ISP, no frame —
// this is the first step of docs/porting-plan.md item 9, and it says whether the sensor is
// alive from Linux at all.
//
//	camprobe [-drv N]   # N = index into the kernel's sensor list (0 on cronos = OV02B10)
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
	nrGetRes2     = 10
	nrClose       = 25
	nrCheckAlive  = 30
	nrSetDriver   = 35
	nrGetInfo2    = 65
	nrSetCurrent  = 70
	dualMainShift = 16 // DUAL_CAMERA_MAIN_SENSOR = 1 in the upper half of the index
)

func ioc(dir, nr, size uint32) uintptr {
	return uintptr(dir<<30 | size<<16 | uint32(magic)<<8 | nr)
}

func ioctl(fd int, req uintptr, arg unsafe.Pointer) error {
	_, _, e := syscall.Syscall(syscall.SYS_IOCTL, uintptr(fd), req, uintptr(arg))
	if e != 0 {
		return e
	}
	return nil
}

func main() {
	drv := flag.Int("drv", 0, "sensor driver index in the kernel's list")
	flag.Parse()

	fd, err := syscall.Open("/dev/kd_camera_hw", syscall.O_RDWR, 0)
	if err != nil {
		fmt.Println("open /dev/kd_camera_hw:", err)
		os.Exit(1)
	}
	defer syscall.Close(fd)
	fmt.Println("opened /dev/kd_camera_hw")

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
		os.Exit(1)
	}
	fmt.Println("T_OPEN ok: sensor powered and initialised")

	if err := ioctl(fd, ioc(0, nrCheckAlive, 0), nil); err != nil {
		fmt.Println("CHECK_IS_ALIVE:", err)
	} else {
		fmt.Println("CHECK_IS_ALIVE ok")
	}

	// GETINFO2: {u32 SensorId; ptr info; ptr resolution} — generous buffers for the structs.
	info := make([]byte, 16384)
	res := make([]byte, 1024)
	var gi [3]uint32
	gi[1] = uint32(uintptr(unsafe.Pointer(&info[0])))
	gi[2] = uint32(uintptr(unsafe.Pointer(&res[0])))
	if err := ioctl(fd, ioc(3, nrGetInfo2, 12), unsafe.Pointer(&gi[0])); err != nil {
		fmt.Println("GETINFO2:", err)
	} else {
		fmt.Printf("GETINFO2 ok: SensorId %#x\n", gi[0])
		names := []string{"preview", "full", "video", "hs video", "slim video", "custom1", "custom2"}
		for i, n := range names {
			w := binary.LittleEndian.Uint16(res[i*4:])
			h := binary.LittleEndian.Uint16(res[i*4+2:])
			fmt.Printf("  %-10s %4d x %4d\n", n, w, h)
		}
	}

	// GETRESOLUTION2: two pointers to resolution structs, main first.
	var pr [2]uint32
	res2 := make([]byte, 1024)
	pr[0] = uint32(uintptr(unsafe.Pointer(&res2[0])))
	if err := ioctl(fd, ioc(3, nrGetRes2, 8), unsafe.Pointer(&pr[0])); err != nil {
		fmt.Println("GETRESOLUTION2:", err)
	} else {
		fmt.Printf("GETRESOLUTION2 ok: preview %d x %d, full %d x %d\n",
			binary.LittleEndian.Uint16(res2[0:]), binary.LittleEndian.Uint16(res2[2:]),
			binary.LittleEndian.Uint16(res2[4:]), binary.LittleEndian.Uint16(res2[6:]))
	}

	if err := ioctl(fd, ioc(0, nrClose, 0), nil); err != nil {
		fmt.Println("T_CLOSE:", err)
	} else {
		fmt.Println("T_CLOSE ok: sensor powered down")
	}
}
