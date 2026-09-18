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

func TestDesiredKeys(t *testing.T) {
	report := func(buttons, x0, x1, y0, y1 byte) []byte {
		result := make([]byte, requiredReportBytes)
		result[buttonByteIndex] = buttons
		result[joystickXByteIndex], result[joystickXByteIndex+1] = x0, x1
		result[joystickYByteIndex], result[joystickYByteIndex+1] = y0, y1
		return result
	}

	tests := []struct {
		name   string
		report []byte
		want   map[keyCode]bool
		valid  bool
	}{
		{name: "all buttons", report: report(0xF0, 0x12, 0x34, 0x56, 0x78), want: map[keyCode]bool{keySpace: true, keyEscape: true, keyE: true, keyQ: true}, valid: true},
		{name: "left", report: report(0, 0x00, 0x80, 0x12, 0x34), want: map[keyCode]bool{keyA: true}, valid: true},
		{name: "right", report: report(0, 0xFF, 0x7F, 0x12, 0x34), want: map[keyCode]bool{keyD: true}, valid: true},
		{name: "forward", report: report(0, 0x12, 0x34, 0xFF, 0x7F), want: map[keyCode]bool{keyW: true}, valid: true},
		{name: "backward", report: report(0, 0x12, 0x34, 0x00, 0x80), want: map[keyCode]bool{keyS: true}, valid: true},
		{name: "diagonal ignores stick", report: report(0, 0x00, 0x80, 0xFF, 0x7F), want: map[keyCode]bool{}, valid: true},
		{name: "short report", report: make([]byte, requiredReportBytes-1), valid: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, valid := desiredKeys(test.report)
			if valid != test.valid || !reflect.DeepEqual(got, test.want) {
				t.Errorf("desiredKeys() = (%v, %v), want (%v, %v)", got, valid, test.want, test.valid)
			}
		})
	}
}

type fakeKeyboard struct {
	events []string
}

func (keyboard *fakeKeyboard) KeyDown(key keyCode) {
	keyboard.events = append(keyboard.events, "down:"+keyName(key))
}

func (keyboard *fakeKeyboard) KeyUp(key keyCode) {
	keyboard.events = append(keyboard.events, "up:"+keyName(key))
}

func keyName(key keyCode) string {
	return map[keyCode]string{
		keyW: "W", keyA: "A", keyS: "S", keyD: "D",
		keySpace: "Space", keyEscape: "Escape", keyE: "E", keyQ: "Q",
	}[key]
}

func TestHeldKeyState(t *testing.T) {
	fake := &fakeKeyboard{}
	state := newHeldKeyState(fake)

	state.Sync(map[keyCode]bool{keyW: true, keySpace: true})
	state.Sync(map[keyCode]bool{keyW: true, keySpace: true})
	state.Sync(map[keyCode]bool{keyD: true})
	state.ReleaseAll()

	want := []string{
		"down:W", "down:Space",
		"up:W", "up:Space", "down:D",
		"up:D",
	}
	if !reflect.DeepEqual(fake.events, want) {
		t.Errorf("events = %v, want %v", fake.events, want)
	}
}
