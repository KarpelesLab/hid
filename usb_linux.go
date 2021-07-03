package hid

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"syscall"
	"time"
	"unsafe"
)

type usbDevice struct {
	info Info

	epIn  int
	epOut int

	inputPacketSize  uint16
	outputPacketSize uint16

	path string
}

type usbHandle struct {
	dev *usbDevice
	f   *os.File
}

func (hid *usbDevice) Open() (Handle, error) {
	f, err := os.OpenFile(hid.path, os.O_RDWR, 0644)
	if err != nil {
		return nil, err
	}
	h := &usbHandle{dev: hid, f: f}
	if err = h.claim(); err != nil {
		f.Close()
		return nil, err
	}
	return h, nil
}

func (hid *usbHandle) Close() error {
	if hid.f != nil {
		hid.release()
		hid.f.Close()
		hid.f = nil
	}
	return nil
}

func (hid *usbDevice) Info() Info {
	return hid.info
}

func (hid *usbHandle) ioctl(n uint32, arg interface{}) (int, error) {
	b := new(bytes.Buffer)
	if err := binary.Write(b, binary.LittleEndian, arg); err != nil {
		return -1, err
	}
	r, _, err := syscall.Syscall6(syscall.SYS_IOCTL,
		uintptr(hid.f.Fd()), uintptr(n),
		uintptr(unsafe.Pointer(&(b.Bytes()[0]))), 0, 0, 0)
	return int(r), err
}

func (hid *usbHandle) claim() error {
	ifno := uint32(hid.dev.info.Interface)
	if r, errno := hid.ioctl(USBDEVFS_IOCTL, &usbfsIoctl{
		Interface: ifno,
		IoctlCode: USBDEVFS_DISCONNECT,
		Data:      0,
	}); r == -1 {
		// this typically means there was no driver on the usb device
		Logger.Println("driver disconnect failed:", r, errno)
	}

	if r, errno := hid.ioctl(USBDEVFS_CLAIM, &ifno); r == -1 {
		return errno
	}
	return nil
}

func (hid *usbHandle) release() error {
	ifno := uint32(hid.dev.info.Interface)
	if r, errno := hid.ioctl(USBDEVFS_RELEASE, &ifno); r == -1 {
		return errno
	}

	if r, errno := hid.ioctl(USBDEVFS_IOCTL, &usbfsIoctl{
		Interface: ifno,
		IoctlCode: USBDEVFS_CONNECT,
		Data:      0,
	}); r == -1 {
		Logger.Println("driver connect failed:", r, errno)
	}
	return nil
}

func (hid *usbHandle) Ctrl(rtype, req, val, index int, data []byte, t int) (int, error) {
	return hid.ctrl(rtype, req, val, index, data, t)
}

func (hid *usbHandle) ctrl(rtype, req, val, index int, data []byte, t int) (int, error) {
	s := usbfsCtrl{
		ReqType: uint8(rtype),
		Req:     uint8(req),
		Value:   uint16(val),
		Index:   uint16(index),
		Len:     uint16(len(data)),
		Timeout: uint32(t),
		Data:    slicePtr(data),
	}
	if r, err := hid.ioctl(USBDEVFS_CONTROL, &s); r == -1 {
		return -1, err
	} else {
		return r, nil
	}
}

func (hid *usbHandle) intr(ep int, data []byte, t int) (int, error) {
	if r, err := hid.ioctl(USBDEVFS_BULK, &usbfsBulk{
		Endpoint: uint32(ep),
		Len:      uint32(len(data)),
		Timeout:  uint32(t),
		Data:     slicePtr(data),
	}); r == -1 {
		return -1, err
	} else {
		return r, nil
	}
}

func (hid *usbHandle) ReadInputPacket(timeout time.Duration) ([]byte, error) {
	buf := make([]byte, hid.dev.inputPacketSize)
	n, err := hid.Read(buf, timeout)
	if err != nil {
		return nil, err
	}
	return buf[:n], nil
}

func (hid *usbHandle) Read(data []byte, timeout time.Duration) (int, error) {
	ms := timeout / (1 * time.Millisecond)
	return hid.intr(hid.dev.epIn, data, int(ms))
}

func (hid *usbHandle) Write(data []byte, timeout time.Duration) (int, error) {
	if hid.dev.epOut > 0 {
		ms := timeout / (1 * time.Millisecond)
		return hid.intr(hid.dev.epOut, data, int(ms))
	} else {
		return hid.ctrl(0x21, 0x09, 2<<8+0, int(hid.dev.info.Interface), data, len(data))
	}
}

func (hid *usbHandle) HIDReport() ([]byte, error) {
	buf := make([]byte, 256, 256)
	// In transfer, recepient interface, GetDescriptor, HidReport type
	n, err := hid.ctrl(0x81, 0x06, 0x22<<8+int(hid.dev.info.Interface), 0, buf, 1000)
	if err != nil {
		return nil, err
	} else {
		return buf[:n], nil
	}
}

func (hid *usbHandle) GetFeatureReport(report int) ([]byte, error) {
	return hid.GetReport((3 << 8) | report)
}

func (hid *usbHandle) GetReport(report int) ([]byte, error) {
	buf := make([]byte, 256)
	// 10100001, GET_REPORT, type*256+id, intf, len, data
	n, err := hid.ctrl(0xa1, 0x01, report, int(hid.dev.info.Interface), buf, 1000)
	if err != nil {
		return nil, err
	} else {
		return buf[:n], nil
	}
}

func (hid *usbHandle) SetReport(report int, data []byte) error {
	// 00100001, SET_REPORT, type*256+id, intf, len, data
	_, err := hid.ctrl(0x21, 0x09, report, int(hid.dev.info.Interface), data, 1000)
	return err
}

func (hid *usbHandle) SetFeatureReport(report int, data []byte) error {
	return hid.SetReport((3<<8)|report, data)
}

func (hid *usbHandle) SetOutputReport(report int, data []byte) error {
	return hid.SetReport((2<<8)|report, data)
}

//
// Enumeration
//

func cast(b []byte, to interface{}) error {
	r := bytes.NewBuffer(b)
	return binary.Read(r, binary.LittleEndian, to)
}

// typical /dev bus entry: /dev/bus/usb/006/003
var reDevBusDevice = regexp.MustCompile(`/dev/bus/usb/(\d+)/(\d+)`)

func walker(path string, cb func(Device)) error {
	if desc, err := ioutil.ReadFile(path); err != nil {
		return err
	} else {
		r := bytes.NewBuffer(desc)
		expected := map[byte]bool{
			UsbDescTypeDevice: true,
		}
		devDesc := deviceDesc{}
		var device *usbDevice
		for r.Len() > 0 {
			if length, err := r.ReadByte(); err != nil {
				return err
			} else if err := r.UnreadByte(); err != nil {
				return err
			} else {
				body := make([]byte, length, length)
				if n, err := r.Read(body); err != nil {
					return err
				} else if n != int(length) || length < 2 {
					return errors.New("short read")
				} else {
					if !expected[body[1]] {
						continue
					}
					switch body[1] {
					case UsbDescTypeDevice:
						expected[UsbDescTypeDevice] = false
						expected[UsbDescTypeConfig] = true
						if err := cast(body, &devDesc); err != nil {
							return err
						}
						//info := Info{
						//}
					case UsbDescTypeConfig:
						expected[UsbDescTypeInterface] = true
						expected[UsbDescTypeReport] = false
						expected[UsbDescTypeEndpoint] = false
						// Device left from the previous config
						if device != nil {
							cb(device)
							device = nil
						}
					case UsbDescTypeInterface:
						if device != nil {
							cb(device)
							device = nil
						}
						expected[UsbDescTypeEndpoint] = true
						expected[UsbDescTypeReport] = true
						i := &interfaceDesc{}
						if err := cast(body, i); err != nil {
							return err
						}
						if i.InterfaceClass == UsbHidClass {
							matches := reDevBusDevice.FindStringSubmatch(path)
							bus := 0
							dev := 0
							if len(matches) >= 3 {
								bus, _ = strconv.Atoi(matches[1])
								dev, _ = strconv.Atoi(matches[2])
							}
							device = &usbDevice{
								info: Info{
									Vendor:    devDesc.Vendor,
									Product:   devDesc.Product,
									Revision:  devDesc.Revision,
									SubClass:  i.InterfaceSubClass,
									Protocol:  i.InterfaceProtocol,
									Interface: i.Number,
									Bus:       bus,
									Device:    dev,
								},
								path: path,
							}
						}
					case UsbDescTypeEndpoint:
						if device != nil {
							if device.epIn != 0 && device.epOut != 0 {
								cb(device)
								device.epIn = 0
								device.epOut = 0
							}
							e := &endpointDesc{}
							if err := cast(body, e); err != nil {
								return err
							}
							if e.Address > 0x80 && device.epIn == 0 {
								device.epIn = int(e.Address)
								device.inputPacketSize = e.MaxPacketSize
							} else if e.Address < 0x80 && device.epOut == 0 {
								device.epOut = int(e.Address)
								device.outputPacketSize = e.MaxPacketSize
							}
						}
					}
				}
			}
		}
		if device != nil {
			cb(device)
		}
	}
	return nil
}

func UsbWalk(cb func(Device)) {
	filepath.Walk(DevBusUsb, func(f string, fi os.FileInfo, err error) error {
		if err != nil || fi.IsDir() {
			return nil
		}
		if err := walker(f, cb); err != nil {
			Logger.Println("UsbWalk: ", err)
		}
		return nil
	})
}
