// Package device provides the [Device] interface for interacting with the G13
// gameboard.
//
// Decoding logic adapted from https://github.com/khampf/g13 which was
// originally forked and adapted from https://github.com/ecraven/g13/.
package device

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	"os"
	"time"

	"github.com/google/gousb"
)

const (
	g13VendorID  = 0x046d
	g13ProductID = 0xc21c
)

type Device interface {
	Close()
	ReadBytes() ([]byte, error)
	ReadInput() (uint64, error)
	SetBacklightColour(r, g, b uint8) error
	SetLCD(image.Image) error
	ResetLCD() error
	SetTimeout(time.Duration) error
}

var ErrReadTimeout = errors.New("timed out reading from device")

type G13Device struct {
	ctx  *gousb.Context
	dev  *gousb.Device
	cfg  *gousb.Config
	intf *gousb.Interface
	iep  *gousb.InEndpoint
	oep  *gousb.OutEndpoint

	timeout time.Duration
}

// New returns an initialised [G13Device] for a connected G13 gameboard. It
// contains an initialised [gousb.InEndpoint] which is used by
// [G13Device.ReadBytes] and [G13Device.ReadInput] for reading button presses.
func New() (Device, error) {
	ctx := gousb.NewContext()

	d := G13Device{}
	var dev *gousb.Device
	for dev == nil {
		var err error
		dev, err = ctx.OpenDeviceWithVIDPID(g13VendorID, g13ProductID)
		if err != nil {
			d.Close()
			return nil, fmt.Errorf("failed to open device: %w", err)
		}

		if dev == nil {
			fmt.Fprintf(os.Stderr, "device not found: waiting for device\n")
			time.Sleep(3 * time.Second)
		}
	}

	d.dev = dev
	cfg, err := dev.Config(1)
	if err != nil {
		d.Close()
		return nil, fmt.Errorf("failed to initialise config: %w", err)
	}
	d.cfg = cfg

	if err := dev.SetAutoDetach(true); err != nil {
		d.Close()
		return nil, fmt.Errorf("failed to enable automatic kernel driver detachment: %w", err)
	}

	intf, err := cfg.Interface(0, 0)
	if err != nil {
		d.Close()
		return nil, fmt.Errorf("failed to select interface 0: %w", err)
	}
	d.intf = intf

	ep, err := intf.InEndpoint(1)
	if err != nil {
		d.Close()
		return nil, fmt.Errorf("failed to initialise input endpoint: %w", err)
	}

	// Probably unnecessary, but good to be sure
	ep.Desc.TransferType = gousb.TransferTypeInterrupt
	d.iep = ep

	op, err := intf.OutEndpoint(2)
	if err != nil {
		d.Close()
		return nil, fmt.Errorf("failed to initialise output endpoint: %w", err)
	}
	d.oep = op

	// Set default timeout to 100 ms. Feels the best empirically.
	d.timeout = 100 * time.Millisecond
	return &d, nil
}

func (d *G13Device) Close() {
	if d == nil {
		return
	}

	if d.dev != nil {
		if err := d.ResetBacklightColour(); err != nil {
			fmt.Fprintf(os.Stderr, "error resetting backlight during shutdown: %s\n", err)
		}
		if err := d.ResetLCD(); err != nil {
			fmt.Fprintf(os.Stderr, "error resetting LCD during shutdown: %s\n", err)
		}
	}

	if d.ctx != nil {
		defer func() {
			if err := d.ctx.Close(); err != nil {
				fmt.Fprintf(os.Stderr, "error closing USB context during shutdown: %s\n", err)
			}
			d.ctx = nil
		}()
	}

	if d.dev != nil {
		defer func() {
			if err := d.dev.Close(); err != nil {
				fmt.Fprintf(os.Stderr, "error closing USB device during shutdown: %s\n", err)
			}
			d.dev = nil
		}()
	}

	if d.cfg != nil {
		defer func() {
			if err := d.cfg.Close(); err != nil {
				fmt.Fprintf(os.Stderr, "error closing USB config during shutdown: %s\n", err)
			}
			d.cfg = nil
		}()
	}

	if d.intf != nil {
		defer func() {
			d.intf.Close()
			d.intf = nil
		}()
	}
}

func (d *G13Device) ReadInput() (uint64, error) {
	buf, err := d.ReadBytes()
	if err != nil {
		return 0, err
	}
	return binary.LittleEndian.Uint64(buf), nil
}

// ReadBytes reads a byte array from the device. The size is the maximum
// supported. Returns a [ErrReadTimeout] if the read times out. Timeout can
// be set using [G13Device.SetTimeout].
func (d *G13Device) ReadBytes() ([]byte, error) {
	if d.iep == nil {
		return nil, fmt.Errorf("tried to read bytes from a closed device")
	}

	ctx, cancel := context.WithTimeout(context.Background(), d.timeout)
	defer cancel()

	buf := make([]byte, 1*d.iep.Desc.MaxPacketSize)
	if _, err := d.iep.ReadContext(ctx, buf); err != nil {
		if errors.Is(err, gousb.TransferCancelled) {
			return nil, ErrReadTimeout
		}
		return nil, fmt.Errorf("failed reading from device: %w", err)
	}

	return buf, nil
}

// SetTimeout sets the timeout for reads from the device.
func (d *G13Device) SetTimeout(dt time.Duration) error {
	if d == nil {
		return fmt.Errorf("tried to set timeout on nil device")
	}

	d.timeout = dt
	return nil
}
