package hid

import (
	"io/ioutil"
	"log"
	"time"
)

//
// General information about the HID device
//
type Info struct {
	Vendor   uint16
	Product  uint16
	Revision uint16

	SubClass uint8
	Protocol uint8

	Interface uint8
	Bus       int
	Device    int
}

//
// A common HID device interace
//
type Device interface {
	Open() (Handle, error)
	Info() Info
}

type Handle interface {
	Close() error
	HIDReport() ([]byte, error)
	SetFeatureReport(int, []byte) error
	GetFeatureReport(int) ([]byte, error)
	ReadInputPacket(timeout time.Duration) ([]byte, error)
	Read(buf []byte, ms time.Duration) (int, error)
	Write(data []byte, ms time.Duration) (int, error)
	Ctrl(rtype, req, val, index int, data []byte, t int) (int, error)
}

// Default Logger setting
//var Logger = log.New(log.Writer(), "hid", log.LstdFlags)
var Logger = log.New(ioutil.Discard, "hid", log.LstdFlags)
