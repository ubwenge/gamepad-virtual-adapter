package main

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/BurntSushi/toml"
)

func TestSampleConfigMatchesCurrentControllerReports(t *testing.T) {
	config, err := loadMapperConfig("config.toml")
	if err != nil {
		t.Fatal(err)
	}
	if config.vendorID != 0x0E6F || config.productID != 0x0401 || config.minReportBytes != 10 {
		t.Fatalf("config = %#v, want PDP IDs and minimum report length 10", config)
	}

	report := func(button, x0, x1, y0, y1 byte) []byte {
		result := make([]byte, 19)
		result[3] = button
		result[6], result[7] = x0, x1
		result[8], result[9] = y0, y1
		return result
	}
	tests := []struct {
		name   string
		report []byte
		want   map[keyCode]bool
	}{
		{"buttons", report(0xF0, 0x12, 0x34, 0x56, 0x78), map[keyCode]bool{keySpace: true, keyEscape: true, keyE: true, keyQ: true}},
		{"left", report(0, 0, 0x80, 0x12, 0x34), map[keyCode]bool{keyA: true}},
		{"right", report(0, 0xFF, 0x7F, 0x12, 0x34), map[keyCode]bool{keyD: true}},
		{"forward", report(0, 0x12, 0x34, 0xFF, 0x7F), map[keyCode]bool{keyW: true}},
		{"backward", report(0, 0x12, 0x34, 0, 0x80), map[keyCode]bool{keyS: true}},
		{"diagonal", report(0, 0, 0x80, 0xFF, 0x7F), map[keyCode]bool{}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, valid := config.desiredKeys(test.report)
			if !valid || !reflect.DeepEqual(got, test.want) {
				t.Fatalf("desiredKeys() = (%v, %v), want (%v, true)", got, valid, test.want)
			}
		})
	}
}

func TestReportRuleParsing(t *testing.T) {
	offset, bytes, err := parseReportRule("AA BB [10] 00")
	if err != nil || offset != 2 || !reflect.DeepEqual(bytes, []byte{0x10}) {
		t.Fatalf("single-byte span = (%d, % X, %v)", offset, bytes, err)
	}
	offset, bytes, err = parseReportRule("AA BB CC [FF 7F] 00")
	if err != nil || offset != 3 || !reflect.DeepEqual(bytes, []byte{0xFF, 0x7F}) {
		t.Fatalf("multi-byte span = (%d, % X, %v)", offset, bytes, err)
	}

	for _, report := range []string{
		"", "00 1G [10]", "00 000 [10]", "00 10", "00 []", "00 [10", "00 10]", "[10] [20]", "[10] 00 [20]", "[10] 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00",
	} {
		if _, _, err := parseReportRule(report); err == nil {
			t.Errorf("parseReportRule(%q) succeeded", report)
		}
	}
}

func TestRuleMatchingUsesOnlyBracketedBytes(t *testing.T) {
	rule := signalRule{offset: 2, match: []byte{0x10}, mask: true}
	if !rule.matches([]byte{0x00, 0xFF, 0x90}) {
		t.Error("mask should match when its bit is set, despite different context")
	}
	if rule.matches([]byte{0x00, 0xFF, 0x20}) {
		t.Error("mask should not match when its bit is clear")
	}
	exact := signalRule{offset: 1, match: []byte{0xFF, 0x7F}}
	if !exact.matches([]byte{0xAA, 0xFF, 0x7F, 0xBB}) || exact.matches([]byte{0xAA, 0xFF, 0x80}) {
		t.Error("multi-byte span must be an exact consecutive match")
	}
}

func TestShortReportDoesNotProduceDesiredState(t *testing.T) {
	config := defaultMapperConfig()
	if got, valid := config.desiredKeys(make([]byte, config.minReportBytes-1)); valid || got != nil {
		t.Errorf("short report = (%v, %v), want (nil, false)", got, valid)
	}
}

func TestDuplicateKeyboardTargetsAreRejected(t *testing.T) {
	_, err := buildMapperConfig(fileConfig{
		VendorID: 1, ProductID: 2,
		Signals: fileSignals{Face: map[string]string{
			"A": "[01]", "B": "[02]",
		}},
		Keys: map[string]string{"A": "Space", "B": "Space"},
	})
	if err == nil {
		t.Fatal("buildMapperConfig accepted duplicate keyboard target")
	}
}

func TestConfigValidation(t *testing.T) {
	valid := `vendor_id = 1
product_id = 2
[signals.Face]
A = "[10]"
[keys]
A = "Space"
`
	tests := []struct {
		name     string
		contents string
	}{
		{"unknown field", strings.Replace(valid, "[signals.Face]", "extra = 1\n[signals.Face]", 1)},
		{"invalid id", strings.Replace(valid, "vendor_id = 1", "vendor_id = 0x10000", 1)},
		{"unknown signal", strings.Replace(valid, "A = \"[10]\"", "Z = \"[10]\"", 1)},
		{"misplaced signal", strings.Replace(valid, "[signals.Face]", "[signals.Sticks]", 1)},
		{"unknown signal group", strings.Replace(valid, "[signals.Face]", "[signals.Other]", 1)},
		{"legacy signal table", strings.Replace(valid, "[signals.Face]\nA = \"[10]\"", "[signals.A]\nreport = \"[10]\"", 1)},
		{"undefined binding", valid + "B = \"E\"\n"},
		{"missing binding", strings.Replace(valid, "A = \"Space\"", "", 1)},
		{"invalid key", strings.Replace(valid, "Space", "Return", 1)},
		{"legacy exclusive group", strings.Replace(valid, "A = \"[10]\"", "A = \"[10]\"\nexclusive_group = \"other\"", 1)},
		{"non-string report", strings.Replace(valid, "A = \"[10]\"", "A = 10", 1)},
		{"malformed toml", "vendor_id = [\n"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.toml")
			if err := os.WriteFile(path, []byte(test.contents), 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := loadMapperConfig(path); err == nil {
				t.Fatal("loadMapperConfig succeeded")
			}
		})
	}
}

func TestAllXInputSignalSectionsAreAccepted(t *testing.T) {
	raw := fileConfig{VendorID: 1, ProductID: 2, Keys: make(map[string]string)}
	groups := []struct {
		names   []string
		reports *map[string]string
	}{
		{faceSignalNames, &raw.Signals.Face},
		{dpadSignalNames, &raw.Signals.DPad},
		{systemSignalNames, &raw.Signals.System},
		{shoulderSignalNames, &raw.Signals.Shoulders},
		{stickSignalNames, &raw.Signals.Sticks},
	}
	outputNames := []string{
		"A", "B", "C", "D", "E", "F", "G", "H", "I", "J", "K", "L",
		"M", "N", "O", "P", "Q", "R", "S", "T", "U", "V",
	}
	index := 0
	for _, group := range groups {
		reports := make(map[string]string, len(group.names))
		*group.reports = reports
		for _, name := range group.names {
			reports[name] = fmt.Sprintf("[%02X]", index+1)
			raw.Keys[name] = outputNames[index]
			index++
		}
	}

	config, err := buildMapperConfig(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(config.rules) != len(knownSignalNames) {
		t.Fatalf("configured %d rules, want %d", len(config.rules), len(knownSignalNames))
	}
	for i, rule := range config.rules {
		if rule.name != knownSignalNames[i] {
			t.Fatalf("rule %d = %q, want %q", i, rule.name, knownSignalNames[i])
		}
	}
}

func TestDPadAndStickDirectionalBehavior(t *testing.T) {
	config, err := buildMapperConfig(fileConfig{
		VendorID: 1, ProductID: 2,
		Signals: fileSignals{
			DPad: map[string]string{
				"DPadUp": "[01]", "DPadRight": "[02]",
			},
			Sticks: map[string]string{
				"LeftThumb":     "00 [04]",
				"LeftThumbUp":   "00 00 [01 00]",
				"LeftThumbLeft": "00 00 00 00 [01 00]",
				"RightThumbUp":  "00 00 00 00 00 00 [01 00]",
			},
		},
		Keys: map[string]string{
			"DPadUp": "A", "DPadRight": "B", "LeftThumb": "C",
			"LeftThumbUp": "D", "LeftThumbLeft": "E", "RightThumbUp": "F",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	report := []byte{0x03, 0x04, 0x01, 0x00, 0x01, 0x00, 0x01, 0x00}
	got, valid := config.desiredKeys(report)
	want := map[keyCode]bool{keyA: true, keyB: true, keyC: true, keyF: true}
	if !valid || !reflect.DeepEqual(got, want) {
		t.Fatalf("desiredKeys() = (%v, %v), want (%v, true)", got, valid, want)
	}
}

func TestEverySupportedKeyboardName(t *testing.T) {
	names := []string{
		"A", "B", "C", "D", "E", "F", "G", "H", "I", "J", "K", "L", "M",
		"N", "O", "P", "Q", "R", "S", "T", "U", "V", "W", "X", "Y", "Z",
		"0", "1", "2", "3", "4", "5", "6", "7", "8", "9",
		"Space", "Escape", "Enter", "Tab", "Backspace", "ArrowUp", "ArrowDown",
		"ArrowLeft", "ArrowRight", "LeftShift", "LeftControl", "LeftOption", "LeftCommand",
	}
	if len(keyNames) != len(names) {
		t.Fatalf("keyNames has %d entries, want %d", len(keyNames), len(names))
	}
	for _, name := range names {
		t.Run(name, func(t *testing.T) {
			config, err := buildMapperConfig(fileConfig{
				VendorID: 1, ProductID: 2,
				Signals: fileSignals{Face: map[string]string{"A": "[01]"}},
				Keys:    map[string]string{"A": name},
			})
			if err != nil {
				t.Fatal(err)
			}
			if config.rules[0].key != keyNames[name] {
				t.Fatalf("key code = %d, want %d", config.rules[0].key, keyNames[name])
			}
		})
	}
	if _, ok := keyCodeForName("Return"); ok {
		t.Fatal("Return was accepted as a keyboard key")
	}
}

func TestTemplateIsParseableButIncomplete(t *testing.T) {
	var template map[string]any
	if _, err := toml.DecodeFile("config.toml.template", &template); err != nil {
		t.Fatalf("template is not valid TOML: %v", err)
	}
	if len(template) != 0 {
		t.Fatalf("template contains uncommented configuration: %v", template)
	}
	if _, err := loadMapperConfig("config.toml.template"); err == nil {
		t.Fatal("incomplete template passed mapper validation")
	}
}

type fakeKeyboard struct{ events []string }

func (keyboard *fakeKeyboard) KeyDown(key keyCode) {
	keyboard.events = append(keyboard.events, "down:"+keyName(key))
}

func (keyboard *fakeKeyboard) KeyUp(key keyCode) {
	keyboard.events = append(keyboard.events, "up:"+keyName(key))
}

func keyName(key keyCode) string {
	return map[keyCode]string{keyW: "W", keyA: "A", keyS: "S", keyD: "D", keySpace: "Space", keyEscape: "Escape", keyE: "E", keyQ: "Q"}[key]
}

func TestHeldKeyState(t *testing.T) {
	fake := &fakeKeyboard{}
	state := newHeldKeyState(fake)
	state.Sync(map[keyCode]bool{keyW: true, keySpace: true})
	state.Sync(map[keyCode]bool{keyW: true, keySpace: true})
	state.Sync(map[keyCode]bool{keyD: true})
	state.ReleaseAll()
	want := []string{"down:W", "down:Space", "up:W", "up:Space", "down:D", "up:D"}
	if !reflect.DeepEqual(fake.events, want) {
		t.Errorf("events = %v, want %v", fake.events, want)
	}
}
