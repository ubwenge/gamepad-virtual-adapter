package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/google/gousb"
)

const (
	VendorID  = 0x0E6F // PDP Vendor ID
	ProductID = 0x0401 // Clone Controller Product ID

	buttonByteIndex     = 3
	joystickXByteIndex  = 6
	joystickYByteIndex  = 8
	requiredReportBytes = joystickYByteIndex + 2
)

type button struct {
	name string
	mask byte
}

var buttons = []button{
	{name: "A", mask: 0x10},
	{name: "B", mask: 0x20},
	{name: "X", mask: 0x40},
	{name: "Y", mask: 0x80},
}

func main() {
	ctx := gousb.NewContext()
	defer ctx.Close()

	fmt.Println("Searching for PDP Xbox 360 Clone Controller...")
	dev, err := ctx.OpenDeviceWithVIDPID(gousb.ID(VendorID), gousb.ID(ProductID))
	if err != nil {
		log.Fatalf("Failed to open device: %v. (Did you use sudo?)", err)
	}
	if dev == nil {
		log.Fatal("Controller not found. Check your physical USB connection.")
	}
	defer dev.Close()

	dev.SetAutoDetach(true)
	intf, done, err := dev.DefaultInterface()
	if err != nil {
		log.Fatalf("Failed to claim default interface: %v", err)
	}
	defer done()

	epIn, err := intf.InEndpoint(1)
	if err != nil {
		log.Fatalf("Failed to open IN Endpoint 0x81: %v", err)
	}

	fmt.Println("Controller mapper online. Press A, B, X, Y, or move the joystick to an endpoint.")
	liveReport(epIn)
}

func readReport(epIn *gousb.InEndpoint) ([]byte, error) {
	buffer := make([]byte, 32)
	for {
		ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
		n, err := epIn.ReadContext(ctx, buffer)
		cancel()
		if err != nil || n == 0 {
			continue
		}
		return append([]byte(nil), buffer[:n]...), nil
	}
}

func liveReport(epIn *gousb.InEndpoint) {
	var lastButtonByte byte
	lastJoystickDirection := ""

	for {
		report, err := readReport(epIn)
		if err != nil {
			continue
		}

		if len(report) > buttonByteIndex {
			for _, name := range buttonPresses(report[buttonByteIndex], lastButtonByte) {
				fmt.Println("Pressed:", name)
			}
			lastButtonByte = report[buttonByteIndex]
		}

		direction := joystickDirection(report)
		if direction == "" {
			// Non-endpoint, partial, diagonal, and neutral joystick states are silent.
			lastJoystickDirection = ""
			continue
		}
		if direction != lastJoystickDirection {
			fmt.Println("Joystick:", direction)
			lastJoystickDirection = direction
		}
	}
}

func buttonPresses(current, previous byte) []string {
	presses := make([]string, 0, len(buttons))
	for _, button := range buttons {
		if current&button.mask != 0 && previous&button.mask == 0 {
			presses = append(presses, button.name)
		}
	}
	return presses
}

func joystickDirection(report []byte) string {
	if len(report) < requiredReportBytes {
		return ""
	}

	left := report[joystickXByteIndex] == 0x00 && report[joystickXByteIndex+1] == 0x80
	right := report[joystickXByteIndex] == 0xFF && report[joystickXByteIndex+1] == 0x7F
	forward := report[joystickYByteIndex] == 0xFF && report[joystickYByteIndex+1] == 0x7F
	backward := report[joystickYByteIndex] == 0x00 && report[joystickYByteIndex+1] == 0x80

	directions := 0
	name := ""
	for _, candidate := range []struct {
		name   string
		active bool
	}{
		{name: "Left", active: left},
		{name: "Right", active: right},
		{name: "Forward", active: forward},
		{name: "Backward", active: backward},
	} {
		if candidate.active {
			directions++
			name = candidate.name
		}
	}
	if directions != 1 {
		return ""
	}
	return name
}
