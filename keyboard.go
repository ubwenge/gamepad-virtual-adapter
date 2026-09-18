package main

// keyCode values are macOS virtual key codes for the configured output keys.
type keyCode uint16

const (
	keyA           keyCode = 0
	keyS           keyCode = 1
	keyD           keyCode = 2
	keyF           keyCode = 3
	keyH           keyCode = 4
	keyG           keyCode = 5
	keyZ           keyCode = 6
	keyX           keyCode = 7
	keyC           keyCode = 8
	keyV           keyCode = 9
	keyB           keyCode = 11
	keyQ           keyCode = 12
	keyW           keyCode = 13
	keyE           keyCode = 14
	keyR           keyCode = 15
	keyY           keyCode = 16
	keyT           keyCode = 17
	key1           keyCode = 18
	key2           keyCode = 19
	key3           keyCode = 20
	key4           keyCode = 21
	key6           keyCode = 22
	key5           keyCode = 23
	key9           keyCode = 25
	key7           keyCode = 26
	key8           keyCode = 28
	key0           keyCode = 29
	keyO           keyCode = 31
	keyU           keyCode = 32
	keyI           keyCode = 34
	keyP           keyCode = 35
	keyEnter       keyCode = 36
	keyL           keyCode = 37
	keyJ           keyCode = 38
	keyK           keyCode = 40
	keyN           keyCode = 45
	keyM           keyCode = 46
	keyTab         keyCode = 48
	keySpace       keyCode = 49
	keyBackspace   keyCode = 51
	keyEscape      keyCode = 53
	keyLeftCommand keyCode = 55
	keyLeftShift   keyCode = 56
	keyLeftOption  keyCode = 58
	keyLeftControl keyCode = 59
	keyArrowLeft   keyCode = 123
	keyArrowRight  keyCode = 124
	keyArrowDown   keyCode = 125
	keyArrowUp     keyCode = 126
)

var mappedKeys = []keyCode{
	keyW, keyA, keyS, keyD,
	keySpace, keyEscape, keyE, keyQ,
}

type keyboardSink interface {
	KeyDown(keyCode)
	KeyUp(keyCode)
}

type keySynchronizer interface {
	Sync(map[keyCode]bool)
}

// heldKeyState sends events only for changes in the desired keyboard state.
type heldKeyState struct {
	sink keyboardSink
	held map[keyCode]bool
	keys []keyCode
}

func newHeldKeyState(sink keyboardSink, keys ...keyCode) *heldKeyState {
	if len(keys) == 0 {
		keys = mappedKeys
	}
	return &heldKeyState{
		sink: sink,
		held: make(map[keyCode]bool, len(keys)),
		keys: append([]keyCode(nil), keys...),
	}
}

func (state *heldKeyState) Sync(desired map[keyCode]bool) {
	for _, key := range state.keys {
		if state.held[key] && !desired[key] {
			state.sink.KeyUp(key)
			delete(state.held, key)
		}
	}
	for _, key := range state.keys {
		if desired[key] && !state.held[key] {
			state.sink.KeyDown(key)
			state.held[key] = true
		}
	}
}

var keyNames = map[string]keyCode{
	"A": keyA, "B": keyB, "C": keyC, "D": keyD, "E": keyE, "F": keyF,
	"G": keyG, "H": keyH, "I": keyI, "J": keyJ, "K": keyK, "L": keyL,
	"M": keyM, "N": keyN, "O": keyO, "P": keyP, "Q": keyQ, "R": keyR,
	"S": keyS, "T": keyT, "U": keyU, "V": keyV, "W": keyW, "X": keyX,
	"Y": keyY, "Z": keyZ,
	"0": key0, "1": key1, "2": key2, "3": key3, "4": key4,
	"5": key5, "6": key6, "7": key7, "8": key8, "9": key9,
	"Space": keySpace, "Escape": keyEscape, "Enter": keyEnter, "Tab": keyTab,
	"Backspace": keyBackspace,
	"ArrowUp":   keyArrowUp, "ArrowDown": keyArrowDown,
	"ArrowLeft": keyArrowLeft, "ArrowRight": keyArrowRight,
	"LeftShift": keyLeftShift, "LeftControl": keyLeftControl,
	"LeftOption": keyLeftOption, "LeftCommand": keyLeftCommand,
}

func keyCodeForName(name string) (keyCode, bool) {
	key, ok := keyNames[name]
	return key, ok
}

func (state *heldKeyState) ReleaseAll() {
	state.Sync(nil)
}
