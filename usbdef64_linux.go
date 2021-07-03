// +build amd64 arm64 arm64be ppc64 ppc64le mips64 mips64le s390x sparc64

package hid

import (
	"unsafe"
)

const (
	USBDEVFS_IOCTL     = 0xc0105512
	USBDEVFS_BULK      = 0xc0185502
	USBDEVFS_CONTROL   = 0xc0185500
	USBDEVFS_SUBMITURB = 0x8038550a
	USBDEVFS_REAPURB   = 0x4008550c

	USBDEVFS_URB_TYPE_ISO       = 0
	USBDEVFS_URB_TYPE_INTERRUPT = 1
	USBDEVFS_URB_TYPE_CONTROL   = 2
	USBDEVFS_URB_TYPE_BULK      = 3
)

type usbfsIoctl struct {
	Interface uint32
	IoctlCode uint32
	Data      uint64
}

type usbfsCtrl struct {
	ReqType uint8
	Req     uint8
	Value   uint16
	Index   uint16
	Len     uint16
	Timeout uint32
	_       uint32 // padding
	Data    uint64 // FIXME
}

type usbfsBulk struct {
	Endpoint uint32
	Len      uint32
	Timeout  uint32
	_        uint32 // padding
	Data     uint64
}

type usbfsUrb struct {
	Type         uint8
	Endpoint     uint8
	_            uint16 // padding
	Status       int32
	Flags        uint32
	_            uint32
	Buffer       uint64 // ptr
	BufferLen    uint32
	ActualLen    uint32
	StartFrame   uint32
	NumberOfPkts uint32 // stream_id for bulk
	ErrorCount   int32
	SigNr        uint32 // signal on completion, or 0 for no signal
	UserContext  uint64 // ptr

	// struct usbdevfs_iso_packet_desc
	FrameLength       uint32
	FrameActualLength uint32
	FrameStatus       uint32
}

func slicePtr(b []byte) uint64 {
	return uint64(uintptr(unsafe.Pointer(&b[0])))
}
