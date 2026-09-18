package main

// keyCode values are macOS virtual key codes for the configured output keys.
type keyCode uint16

const (
	keyA      keyCode = 0
	keyS      keyCode = 1
	keyD      keyCode = 2
	keyQ      keyCode = 12
	keyW      keyCode = 13
	keyE      keyCode = 14
	keySpace  keyCode = 49
	keyEscape keyCode = 53
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
}

func newHeldKeyState(sink keyboardSink) *heldKeyState {
	return &heldKeyState{
		sink: sink,
		held: make(map[keyCode]bool, len(mappedKeys)),
	}
}

func (state *heldKeyState) Sync(desired map[keyCode]bool) {
	for _, key := range mappedKeys {
		if state.held[key] && !desired[key] {
			state.sink.KeyUp(key)
			delete(state.held, key)
		}
	}
	for _, key := range mappedKeys {
		if desired[key] && !state.held[key] {
			state.sink.KeyDown(key)
			state.held[key] = true
		}
	}
}

func (state *heldKeyState) ReleaseAll() {
	state.Sync(nil)
}
