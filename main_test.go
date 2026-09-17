package main

import (
	"reflect"
	"testing"
)

func TestButtonPresses(t *testing.T) {
	tests := []struct {
		name     string
		current  byte
		previous byte
		want     []string
	}{
		{name: "A", current: 0x10, want: []string{"A"}},
		{name: "B", current: 0x20, want: []string{"B"}},
		{name: "X", current: 0x40, want: []string{"X"}},
		{name: "Y", current: 0x80, want: []string{"Y"}},
		{name: "simultaneous", current: 0x90, want: []string{"A", "Y"}},
		{name: "held", current: 0x10, previous: 0x10, want: []string{}},
		{name: "release", previous: 0x10, want: []string{}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := buttonPresses(test.current, test.previous); !reflect.DeepEqual(got, test.want) {
				t.Errorf("buttonPresses(%02X, %02X) = %v, want %v", test.current, test.previous, got, test.want)
			}
		})
	}
}

func TestJoystickDirection(t *testing.T) {
	report := func(x0, x1, y0, y1 byte) []byte {
		result := make([]byte, requiredReportBytes)
		result[joystickXByteIndex], result[joystickXByteIndex+1] = x0, x1
		result[joystickYByteIndex], result[joystickYByteIndex+1] = y0, y1
		return result
	}

	tests := []struct {
		name   string
		report []byte
		want   string
	}{
		{name: "left", report: report(0x00, 0x80, 0x12, 0x34), want: "Left"},
		{name: "right", report: report(0xFF, 0x7F, 0x12, 0x34), want: "Right"},
		{name: "forward", report: report(0x12, 0x34, 0xFF, 0x7F), want: "Forward"},
		{name: "backward", report: report(0x12, 0x34, 0x00, 0x80), want: "Backward"},
		{name: "non-endpoint", report: report(0x12, 0x34, 0x56, 0x78)},
		{name: "diagonal", report: report(0x00, 0x80, 0xFF, 0x7F)},
		{name: "short report", report: make([]byte, requiredReportBytes-1)},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := joystickDirection(test.report); got != test.want {
				t.Errorf("joystickDirection() = %q, want %q", got, test.want)
			}
		})
	}
}
