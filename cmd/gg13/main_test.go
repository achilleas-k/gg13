package main

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/achilleas-k/gg13/internal/config"
	"github.com/achilleas-k/gg13/internal/device"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type testEvent struct {
	// action is press, down, or up
	action string

	// code is the key or button code
	code int
}

// TestKeyboard implements the [keyboard.Keyboard] interface and just records
// each method call as events for testing.
// To differentiate between events groups, use the [newEvent] function to
// start a new event array.
type TestKeyboard struct {
	events [][]testEvent
}

func (tk *TestKeyboard) Close() error {
	return nil
}

func (tk *TestKeyboard) insert(action string, code int) error {
	if tk == nil {
		return fmt.Errorf("keyboard not initialised")
	}
	eventIndex := len(tk.events) - 1
	currentEvent := tk.events[eventIndex]
	currentEvent = append(currentEvent, testEvent{action: action, code: code})
	tk.events[eventIndex] = currentEvent
	return nil
}

func (tk *TestKeyboard) KeyPress(k int) error {
	return tk.insert("press", k)
}

func (tk *TestKeyboard) KeyDown(k int) error {
	return tk.insert("down", k)
}

func (tk *TestKeyboard) KeyUp(k int) error {
	return tk.insert("up", k)
}

func (tk *TestKeyboard) newEvent() {
	tk.events = append(tk.events, []testEvent{})
}

func newTestKeyboard(t *testing.T) *TestKeyboard {
	t.Helper()

	tk := &TestKeyboard{
		events: [][]testEvent{},
	}
	return tk
}

func TestHandleKeyboard(t *testing.T) {
	testCases := map[string]struct {
		mapping        map[device.KeyBit]int
		inputs         []uint64
		expectedEvents [][]testEvent
	}{
		"single-key": {
			mapping: map[device.KeyBit]int{
				device.G1: 30, // 'a' key
			},
			inputs: []uint64{
				device.G1.Uint64(), // G1 pressed
				0,                  // all keys released
			},
			expectedEvents: [][]testEvent{
				{{action: "down", code: 30}},
				{{action: "up", code: 30}},
			},
		},
		"multi-keys": {
			mapping: map[device.KeyBit]int{
				device.G1: 30, // 'a'
				device.G2: 48, // 'b'
				device.G3: 46, // 'c'
			},
			inputs: []uint64{
				device.G1.Uint64(),                      // G1 only
				device.G1.Uint64() | device.G2.Uint64(), // G1 + G2
				device.G2.Uint64(),                      // G2 only
				device.G1.Uint64() | device.G2.Uint64() | device.G3.Uint64(), // all three
				0, // all released
			},
			expectedEvents: [][]testEvent{
				{ // G1 only
					{action: "down", code: 30},
					{action: "up", code: 48},
					{action: "up", code: 46},
				},

				{ // G1 + G2
					{action: "down", code: 30},
					{action: "down", code: 48},
					{action: "up", code: 46},
				},

				{ // G2 only
					{action: "up", code: 30},
					{action: "down", code: 48},
					{action: "up", code: 46},
				},

				{ // G1 + G2 + G3
					{action: "down", code: 30},
					{action: "down", code: 48},
					{action: "down", code: 46},
				},

				{ // all up
					{action: "up", code: 30},
					{action: "up", code: 48},
					{action: "up", code: 46},
				},
			},
		},
		"unmapped-events-only": {
			mapping: map[device.KeyBit]int{
				device.G1: 30, // only G1 mapped
			},
			inputs: []uint64{
				device.G20.Uint64(), // G2 pressed (not mapped)
				device.G17.Uint64(), // G3 pressed (not mapped)
				0,                   // released
			},
			expectedEvents: [][]testEvent{
				{{action: "up", code: 30}},
				{{action: "up", code: 30}},
				{{action: "up", code: 30}},
			},
		},
		"key-combos": {
			mapping: map[device.KeyBit]int{
				device.G1:  29, // left ctrl
				device.G2:  56, // left alt
				device.G10: 20, // T
			},
			inputs: []uint64{
				device.G1.Uint64(),                                            // Ctrl down
				device.G1.Uint64() | device.G2.Uint64(),                       // Ctrl+Alt down
				device.G1.Uint64() | device.G2.Uint64() | device.G10.Uint64(), // Ctrl+Alt+T
				device.G1.Uint64() | device.G2.Uint64(),                       // Release T
				0,                                                             // Release all
			},
			expectedEvents: [][]testEvent{
				{
					{action: "down", code: 29},
					{action: "up", code: 56},
					{action: "up", code: 20},
				},

				{
					{action: "down", code: 29},
					{action: "down", code: 56},
					{action: "up", code: 20},
				},

				{
					{action: "down", code: 29},
					{action: "down", code: 56},
					{action: "down", code: 20},
				},

				{
					{action: "down", code: 29},
					{action: "down", code: 56},
					{action: "up", code: 20},
				},

				{
					{action: "up", code: 29},
					{action: "up", code: 56},
					{action: "up", code: 20},
				},
			},
		},
		"empty-mapping": {
			mapping: map[device.KeyBit]int{},
			inputs: []uint64{
				device.G1.Uint64(),
				device.G2.Uint64() | device.G3.Uint64(),
				0,
			},
			expectedEvents: [][]testEvent{{}, {}, {}}, // three empty events
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			kb := newTestKeyboard(t)

			cfg := config.NewEmpty()
			for g, k := range tc.mapping {
				cfg.SetKey(g, k)
			}

			for _, input := range tc.inputs {
				kb.newEvent()
				handleKeyboard(input, cfg, kb)
			}

			assert := assert.New(t)
			// actions in each event aren't necessarily ordered, so check each
			// event separately
			assert.Len(kb.events, len(tc.expectedEvents))
			for idx := range tc.expectedEvents {
				assert.ElementsMatch(tc.expectedEvents[idx], kb.events[idx])
			}
		})
	}
}

type stickEvent struct {
	x float32
	y float32
}

type TestJoystick struct {
	events      []testEvent
	stickEvents []stickEvent
}

func newTestJoystick(t *testing.T) *TestJoystick {
	t.Helper()
	return &TestJoystick{
		events:      []testEvent{},
		stickEvents: []stickEvent{},
	}
}

func (tj *TestJoystick) Close() error {
	return nil
}

func (tj *TestJoystick) ButtonPress(b int) error {
	if tj == nil {
		return fmt.Errorf("joystick not initialised")
	}
	tj.events = append(tj.events, testEvent{action: "press", code: b})
	return nil
}

func (tj *TestJoystick) ButtonDown(b int) error {
	if tj == nil {
		return fmt.Errorf("joystick not initialised")
	}
	tj.events = append(tj.events, testEvent{action: "down", code: b})
	return nil
}

func (tj *TestJoystick) ButtonUp(b int) error {
	if tj == nil {
		return fmt.Errorf("joystick not initialised")
	}
	tj.events = append(tj.events, testEvent{action: "up", code: b})
	return nil
}

func (tj *TestJoystick) StickPosition(x, y float32) error {
	if tj == nil {
		return fmt.Errorf("joystick not initialised")
	}
	tj.stickEvents = append(tj.stickEvents, stickEvent{x: x, y: y})
	return nil
}

// createConfigWithJoystick creates a temporary config file with joystick mode enabled
func createConfigWithJoystick(t *testing.T) *config.G13Config {
	t.Helper()
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.json")
	cfgData := `{"mapping":{"stick":{"mode":"joystick"}}}`
	require.NoError(t, os.WriteFile(cfgPath, []byte(cfgData), 0o600))
	cfg, err := config.NewFromFile(cfgPath)
	require.NoError(t, err)
	return cfg
}

// encodeStickPosition encodes stick x,y coordinates into the input uint64
func encodeStickPosition(x, y uint8) uint64 {
	return (uint64(x) << 8) | (uint64(y) << 16)
}

func TestHandleJoystick(t *testing.T) {
	testCases := map[string]struct {
		inputs              []uint64
		expectedStickEvents []stickEvent
		useJoystickMode     bool
	}{
		"stick-off": {
			inputs: []uint64{
				encodeStickPosition(127, 127), // center
				encodeStickPosition(255, 255), // max right/down
				encodeStickPosition(0, 0),     // max left/up
			},
			expectedStickEvents: []stickEvent{},
			useJoystickMode:     false,
		},
		"centre": {
			inputs: []uint64{
				encodeStickPosition(127, 127),
			},
			expectedStickEvents: []stickEvent{
				{x: 0.0, y: 0.0}, // center is 0,0 in uinput coordinates
			},
			useJoystickMode: true,
		},
		"corners": {
			inputs: []uint64{
				encodeStickPosition(255, 255), // max right/down
				encodeStickPosition(0, 0),     // max left/up
				encodeStickPosition(255, 0),   // max right/up
				encodeStickPosition(0, 255),   // max left/down
			},
			expectedStickEvents: []stickEvent{
				{x: 1.0078740, y: 1.0078740}, // max right/down
				{x: -1.0, y: -1.0},           // max left/up
				{x: 1.0078740, y: -1.0},      // max right/up
				{x: -1.0, y: 1.0078740},      // max left/down
			},
			useJoystickMode: true,
		},
		"half-circle-down": {
			inputs: []uint64{
				encodeStickPosition(127, 127), // center
				encodeStickPosition(200, 127), // right
				encodeStickPosition(200, 200), // right-down
				encodeStickPosition(127, 200), // down
				encodeStickPosition(50, 200),  // left-down
				encodeStickPosition(50, 127),  // left
				encodeStickPosition(127, 127), // back to center
			},
			expectedStickEvents: []stickEvent{
				{x: 0.0, y: 0.0},
				{x: 0.5748032, y: 0.0},
				{x: 0.5748032, y: 0.5748032},
				{x: 0.0, y: 0.5748032},
				{x: -0.6062992, y: 0.5748032},
				{x: -0.6062992, y: 0.0},
				{x: 0.0, y: 0.0},
			},
			useJoystickMode: true,
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			js := newTestJoystick(t)

			var cfg *config.G13Config
			if tc.useJoystickMode {
				cfg = createConfigWithJoystick(t)
			} else {
				cfg = config.NewEmpty()
			}

			for _, input := range tc.inputs {
				handleJoystick(input, cfg, js)
			}

			assert.Equal(t, tc.expectedStickEvents, js.stickEvents)
		})
	}
}

func TestHandleInput(t *testing.T) {
	testCases := map[string]struct {
		keyMapping          map[device.KeyBit]int
		inputs              []uint64
		expectedKeyEvents   [][]testEvent
		expectedStickEvents []stickEvent
		useJoystickMode     bool
	}{
		"key-stick-combo": {
			keyMapping: map[device.KeyBit]int{
				device.G4:  30, // A
				device.G11: 48, // B
			},
			inputs: []uint64{
				// G4 pressed with stick at center
				device.G4.Uint64() | encodeStickPosition(127, 127),
				// G4 and G11 pressed with stick moved right
				device.G4.Uint64() | device.G11.Uint64() | encodeStickPosition(200, 127),
				// All released, stick back to center
				encodeStickPosition(127, 127),
			},
			expectedKeyEvents: [][]testEvent{
				{
					{action: "down", code: 30},
					{action: "up", code: 48},
				},
				{
					{action: "down", code: 30},
					{action: "down", code: 48},
				},
				{
					{action: "up", code: 30},
					{action: "up", code: 48},
				},
			},
			expectedStickEvents: []stickEvent{
				{x: 0.0, y: 0.0},
				{x: 0.5748032, y: 0.0},
				{x: 0.0, y: 0.0},
			},
			useJoystickMode: true,
		},
		"kb-only": {
			keyMapping: map[device.KeyBit]int{
				device.LEFT: 42,
			},
			inputs: []uint64{
				device.LEFT.Uint64() | encodeStickPosition(255, 255),
				0,
			},
			expectedKeyEvents: [][]testEvent{
				{
					{action: "down", code: 42},
				},
				{
					{action: "up", code: 42},
				},
			},
			expectedStickEvents: []stickEvent{},
			useJoystickMode:     false,
		},
		"js-only": {
			keyMapping: map[device.KeyBit]int{},
			inputs: []uint64{
				encodeStickPosition(127, 127),
				encodeStickPosition(255, 0),
				encodeStickPosition(0, 255),
			},
			expectedKeyEvents: [][]testEvent{{}, {}, {}},
			expectedStickEvents: []stickEvent{
				{x: 0.0, y: 0.0},
				{x: 1.0078740, y: -1.0},
				{x: -1.0, y: 1.0078740},
			},
			useJoystickMode: true,
		},
		"key-stick-combo-2": {
			keyMapping: map[device.KeyBit]int{
				device.G1: 30,
				device.G2: 48,
				device.G3: 46,
			},
			inputs: []uint64{
				device.G1.Uint64() | encodeStickPosition(100, 100),
				device.G2.Uint64() | encodeStickPosition(150, 150),
				device.G3.Uint64() | encodeStickPosition(200, 200),
				0 | encodeStickPosition(127, 127),
			},
			expectedKeyEvents: [][]testEvent{
				{
					{action: "down", code: 30},
					{action: "up", code: 48},
					{action: "up", code: 46},
				},
				{
					{action: "up", code: 30},
					{action: "down", code: 48},
					{action: "up", code: 46},
				},
				{
					{action: "up", code: 30},
					{action: "up", code: 48},
					{action: "down", code: 46},
				},
				{
					{action: "up", code: 30},
					{action: "up", code: 48},
					{action: "up", code: 46},
				},
			},
			expectedStickEvents: []stickEvent{
				{x: -0.21259843, y: -0.21259843},
				{x: 0.18110237, y: 0.18110237},
				{x: 0.5748032, y: 0.5748032},
				{x: 0.0, y: 0.0},
			},
			useJoystickMode: true,
		},
		"progressive-keypress": {
			keyMapping: map[device.KeyBit]int{
				device.G20: 29,
				device.G22: 56,
				device.G4:  20,
			},
			inputs: []uint64{
				device.G20.Uint64() | encodeStickPosition(127, 127),
				device.G20.Uint64() | device.G22.Uint64() | encodeStickPosition(200, 200),
				device.G20.Uint64() | device.G22.Uint64() | device.G4.Uint64() | encodeStickPosition(255, 255),
				encodeStickPosition(127, 127),
			},
			expectedKeyEvents: [][]testEvent{
				{
					{action: "down", code: 29},
					{action: "up", code: 56},
					{action: "up", code: 20},
				},
				{
					{action: "down", code: 29},
					{action: "down", code: 56},
					{action: "up", code: 20},
				},
				{
					{action: "down", code: 29},
					{action: "down", code: 56},
					{action: "down", code: 20},
				},
				{
					{action: "up", code: 29},
					{action: "up", code: 56},
					{action: "up", code: 20},
				},
			},
			expectedStickEvents: []stickEvent{
				{x: 0.0, y: 0.0},
				{x: 0.5748032, y: 0.5748032},
				{x: 1.0078740, y: 1.0078740},
				{x: 0.0, y: 0.0},
			},
			useJoystickMode: true,
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			kb := newTestKeyboard(t)
			js := newTestJoystick(t)

			var cfg *config.G13Config
			if tc.useJoystickMode {
				cfg = createConfigWithJoystick(t)
			} else {
				cfg = config.NewEmpty()
			}

			for gkey, kbkey := range tc.keyMapping {
				cfg.SetKey(gkey, kbkey)
			}

			for _, input := range tc.inputs {
				kb.newEvent()
				handleInput(input, cfg, kb, js)
			}

			assert := assert.New(t)
			// actions in each keyboard event aren't necessarily ordered, so
			// check each event separately
			assert.Len(kb.events, len(tc.expectedKeyEvents))
			for idx := range tc.expectedKeyEvents {
				assert.ElementsMatch(tc.expectedKeyEvents[idx], kb.events[idx])
			}

			assert.Equal(tc.expectedStickEvents, js.stickEvents)
		})
	}
}

func TestNoPanic(t *testing.T) {
	// Test that we don't panic when the keyboard or joystick return an error.
	// This test might change in the future if we change the input handlers to
	// return errors. Currently, they just print an error message and continue.
	// Inputs don't really matter.
	testCases := map[string]struct {
		kb *TestKeyboard
		js *TestJoystick
	}{
		"both-nil": {},
		"kb-nil": {
			js: newTestJoystick(t),
		},
		"js-nil": {
			kb: newTestKeyboard(t),
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			input := device.G20.Uint64() | device.G22.Uint64() | device.G4.Uint64() | encodeStickPosition(255, 255)
			cfg := config.NewEmpty()
			cfg.SetKey(device.G20, 29)
			if tc.kb != nil {
				tc.kb.newEvent()
			}
			assert.NotPanics(t, func() { handleInput(input, cfg, tc.kb, tc.js) })

		})
	}
}
