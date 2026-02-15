package device

import (
	"fmt"
	"image"
	"os"
	"time"

	"github.com/google/gousb"
)

const (
	// TODO: document source of these values
	ControlRequestType = uint8(gousb.ControlClass | gousb.ControlInterface)

	BacklightColourVal = uint16(0x307)

	SetupPacketRequest = uint8(9)

	SetupPacketIndex = uint16(0)

	// Width (columns) of LCD in pixels
	LCDWidth = 160

	// Height (rows) of LCD in pixels
	LCDHeight = 43

	// The size of the byte array to send when writing to the LCD
	LCDDataLength = 992

	// The index in the byte array where the image data begins
	LCDImageStartIdx = 32

	// Magic number that needs to be set as the first byte of the byte array to
	// send when writing to the LCD
	LCDMagicNumber = 3
)

func (d *G13Device) setBacklightColour(r, g, b uint8) error {
	// TODO: set context with timeout
	data := []byte{5, r, g, b, 0}
	n, err := d.dev.Control(ControlRequestType, SetupPacketRequest, BacklightColourVal, SetupPacketIndex, data)
	if err != nil {
		return fmt.Errorf("failed setting backlight colour %+v: %w", data, err)
	}
	if n != len(data) {
		return fmt.Errorf("sent %d bytes but wrote %d while setting backlight colour", len(data), n)
	}

	return nil
}

// SetBacklightColour sets the LCD and key backlight colour to the given r, g,
// b values and starts a background routine to keep setting the colour every
// second.
func (d *G13Device) SetBacklightColour(r, g, b uint8) error {
	// initialise the background colour and catch errors first before starting
	// the routine
	if err := d.setBacklightColour(r, g, b); err != nil {
		return err
	}

	colourFn := func() {
		if err := d.setBacklightColour(r, g, b); err != nil {
			fmt.Fprintln(os.Stderr, err.Error())
		}
	}

	d.routines.colour = newRoutine(colourFn, 1000*time.Millisecond)
	return nil
}

// ResetBacklightColour sets the background colour to 0, 0, 0 (turns the
// backlight off) and stops the backlight colour background routine.
func (d *G13Device) ResetBacklightColour() error {
	if d.routines.colour != nil {
		d.routines.colour.stop()
		d.routines.colour = nil
	}

	return d.setBacklightColour(uint8(0), uint8(0), uint8(0))
}

func (d *G13Device) SetLCD(img image.Image) error {
	bounds := img.Bounds()
	if bounds.Min.X != 0 || bounds.Min.Y != 0 {
		return fmt.Errorf("invalid image: bounds to not start at 0,0")
	}
	if bounds.Max.X != LCDWidth || bounds.Max.Y != LCDHeight {
		return fmt.Errorf("image data has incorrect size %dx%d: %dx%d required", bounds.Max.X, bounds.Max.Y, LCDWidth, LCDHeight)
	}
	data := imageToG13Bytes(img)

	n, err := d.oep.Write(data)
	if err != nil {
		return err
	}
	if n != len(data) {
		return fmt.Errorf("sent %d bytes but wrote %d while setting LCD", len(data), n)
	}

	return nil
}

func (d *G13Device) ResetLCD() error {
	blank := make([]uint8, LCDDataLength)
	blank[0] = 0x03
	n, err := d.oep.Write(blank)
	if err != nil {
		return err
	}
	if n != len(blank) {
		return fmt.Errorf("sent %d bytes but wrote %d while resetting LCD", len(blank), n)
	}
	return nil
}

func imageToG13Bytes(img image.Image) []uint8 {
	vbitmap := make([]uint8, LCDDataLength)
	vbitmap[0] = LCDMagicNumber // Required "magic number"

	// The bits for the LCD start at 32. Each byte represents a *column* of
	// pixels and the next byte represents the *column* to its right.
	// The layout (ignoring the 32-byte offset), looks like this:
	// 000.0 | 001.0 | 002.0
	// 000.1 | 001.1 | 002.1
	// 000.2 | 001.2 | 002.2
	// 000.3 | 001.3 | 002.3
	// 000.4 | 001.4 | 002.4
	// 000.5 | 001.5 | 002.5
	// 000.6 | 001.6 | 002.6
	// 000.7 | 001.7 | 002.7 ...
	// 160.0 | 161.0 | 162.0
	// 160.1 | 161.1 | 162.1
	// 160.2 | 161.2 | 162.2
	// 160.3 | 161.3 | 162.3
	// 160.4 | 161.4 | 162.4
	// 160.5 | 161.5 | 162.5
	// 160.6 | 161.6 | 162.6
	// 160.7 | 161.7 | 162.7 ...
	// ...
	// where each X.Y value in a cell is composed of:
	//   X: the index of the byte
	//   Y: the bit index of the byte

	// run through the image and for each position, find the appropriate bit in
	// the appropriate byte of the LCD to flip

	bounds := img.Bounds() // must be 160x43
	for y := range bounds.Max.Y {
		for x := range bounds.Max.X {
			r, g, b, _ := img.At(x, y).RGBA()
			// convert the image to monochrome by turning on any non-white
			// pixels
			if r+g+b < 255*3 {
				byteIdx := y/8*LCDWidth + x // index of the byte that represents the 8-pixel column we're in
				bitIdx := y % 8             // index of the bit (within the byte) to flip on

				onBit := uint8(1) << bitIdx
				vbitmap[byteIdx+LCDImageStartIdx] |= onBit
			}
		}
	}
	return vbitmap
}
