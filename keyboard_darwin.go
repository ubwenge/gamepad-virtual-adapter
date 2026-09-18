//go:build darwin

package main

/*
#cgo LDFLAGS: -framework ApplicationServices
#include <ApplicationServices/ApplicationServices.h>

static int pdpKeyboardEventAccessGranted(void) {
	return CGPreflightPostEventAccess() ? 1 : 0;
}

static void pdpPostKeyboardEvent(int keyDown, unsigned short keyCode) {
	CGEventRef event = CGEventCreateKeyboardEvent(NULL, (CGKeyCode)keyCode, keyDown != 0);
	if (event != NULL) {
		CGEventPost(kCGHIDEventTap, event);
		CFRelease(event);
	}
}
*/
import "C"

type macKeyboard struct {
	*heldKeyState
}

func newMacKeyboard(keys []keyCode) *macKeyboard {
	keyboard := &macKeyboard{}
	keyboard.heldKeyState = newHeldKeyState(keyboard, keys...)
	return keyboard
}

func keyboardEventAccessGranted() bool {
	return C.pdpKeyboardEventAccessGranted() != 0
}

func (keyboard *macKeyboard) KeyDown(key keyCode) {
	C.pdpPostKeyboardEvent(1, C.ushort(key))
}

func (keyboard *macKeyboard) KeyUp(key keyCode) {
	C.pdpPostKeyboardEvent(0, C.ushort(key))
}
