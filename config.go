package main

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/BurntSushi/toml"
)

const maxInputReportBytes = 32

var knownSignalNames = []string{
	"A", "B", "X", "Y",
	"DPadUp", "DPadDown", "DPadLeft", "DPadRight",
	"Start", "Back",
	"LeftShoulder", "RightShoulder",
	"LeftThumb", "RightThumb",
	"LeftThumbUp", "LeftThumbDown", "LeftThumbLeft", "LeftThumbRight",
	"RightThumbUp", "RightThumbDown", "RightThumbLeft", "RightThumbRight",
}

var faceSignalNames = []string{"A", "B", "X", "Y"}
var dpadSignalNames = []string{"DPadUp", "DPadDown", "DPadLeft", "DPadRight"}
var systemSignalNames = []string{"Start", "Back"}
var shoulderSignalNames = []string{"LeftShoulder", "RightShoulder"}
var stickSignalNames = []string{
	"LeftThumb", "RightThumb",
	"LeftThumbUp", "LeftThumbDown", "LeftThumbLeft", "LeftThumbRight",
	"RightThumbUp", "RightThumbDown", "RightThumbLeft", "RightThumbRight",
}

var knownSignals = func() map[string]bool {
	result := make(map[string]bool, len(knownSignalNames))
	for _, name := range knownSignalNames {
		result[name] = true
	}
	return result
}()

type fileConfig struct {
	VendorID  uint64            `toml:"vendor_id"`
	ProductID uint64            `toml:"product_id"`
	Signals   fileSignals       `toml:"signals"`
	Keys      map[string]string `toml:"keys"`
}

type fileSignals struct {
	Face      map[string]string `toml:"Face"`
	DPad      map[string]string `toml:"DPad"`
	System    map[string]string `toml:"System"`
	Shoulders map[string]string `toml:"Shoulders"`
	Sticks    map[string]string `toml:"Sticks"`
}

// deviceConfig identifies the USB controller to open. It is intentionally
// separate from mapperConfig so monitor mode can be used before any signals or
// keyboard bindings have been discovered.
type deviceConfig struct {
	vendorID  uint16
	productID uint16
}

// mapperConfig is the fully validated runtime configuration.
type mapperConfig struct {
	vendorID       uint16
	productID      uint16
	rules          []signalRule
	keys           []keyCode
	minReportBytes int
}

type signalRule struct {
	name           string
	offset         int
	match          []byte
	mask           bool
	exclusiveGroup string
	key            keyCode
}

func defaultMapperConfig() mapperConfig {
	config, err := buildMapperConfig(fileConfig{
		VendorID:  0x0E6F,
		ProductID: 0x0401,
		Signals: fileSignals{
			Face: map[string]string{
				"A": "00 14 00 [10] 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00",
				"B": "00 14 00 [20] 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00",
				"X": "00 14 00 [40] 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00",
				"Y": "00 14 00 [80] 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00",
			},
			Sticks: map[string]string{
				"LeftThumbLeft":  "00 14 00 00 00 00 [00 80] 00 00 00 00 00 00 00 00 00 00 00 00",
				"LeftThumbRight": "00 14 00 00 00 00 [FF 7F] 00 00 00 00 00 00 00 00 00 00 00 00",
				"LeftThumbUp":    "00 14 00 00 00 00 00 00 [FF 7F] 00 00 00 00 00 00 00 00 00 00",
				"LeftThumbDown":  "00 14 00 00 00 00 00 00 [00 80] 00 00 00 00 00 00 00 00 00 00",
			},
		},
		Keys: map[string]string{
			"LeftThumbUp": "W", "LeftThumbDown": "S", "LeftThumbLeft": "A", "LeftThumbRight": "D",
			"A": "Space", "B": "Escape", "X": "E", "Y": "Q",
		},
	})
	if err != nil {
		panic(fmt.Sprintf("invalid built-in mapper configuration: %v", err))
	}
	return config
}

func defaultDeviceConfig() deviceConfig {
	return deviceConfig{vendorID: 0x0E6F, productID: 0x0401}
}

// loadMonitorConfig reads only the controller IDs. Mapping fields are ignored
// so an ID-only configuration can bootstrap report capture.
func loadMonitorConfig(path string) (deviceConfig, error) {
	var raw struct {
		VendorID  uint64 `toml:"vendor_id"`
		ProductID uint64 `toml:"product_id"`
	}
	if _, err := toml.DecodeFile(path, &raw); err != nil {
		return deviceConfig{}, fmt.Errorf("read config %q: %w", path, err)
	}
	return buildDeviceConfig(raw.VendorID, raw.ProductID)
}

func loadMapperConfig(path string) (mapperConfig, error) {
	var raw fileConfig
	metadata, err := toml.DecodeFile(path, &raw)
	if err != nil {
		return mapperConfig{}, fmt.Errorf("read config %q: %w", path, err)
	}
	if undecoded := metadata.Undecoded(); len(undecoded) != 0 {
		fields := make([]string, len(undecoded))
		for i, key := range undecoded {
			fields[i] = key.String()
		}
		sort.Strings(fields)
		return mapperConfig{}, fmt.Errorf("unknown configuration field(s): %s", strings.Join(fields, ", "))
	}
	return buildMapperConfig(raw)
}

func buildMapperConfig(raw fileConfig) (mapperConfig, error) {
	device, err := buildDeviceConfig(raw.VendorID, raw.ProductID)
	if err != nil {
		return mapperConfig{}, err
	}
	signals, err := collectSignals(raw.Signals)
	if err != nil {
		return mapperConfig{}, err
	}
	if len(signals) == 0 {
		return mapperConfig{}, fmt.Errorf("signals must not be empty")
	}
	if len(raw.Keys) == 0 {
		return mapperConfig{}, fmt.Errorf("keys must not be empty")
	}

	boundKeys := make(map[keyCode]string, len(raw.Keys))
	for name, keyName := range raw.Keys {
		if !knownSignals[name] {
			return mapperConfig{}, fmt.Errorf("key binding references unknown signal %q", name)
		}
		if _, exists := signals[name]; !exists {
			return mapperConfig{}, fmt.Errorf("key binding references undefined signal %q", name)
		}
		key, ok := keyCodeForName(keyName)
		if !ok {
			return mapperConfig{}, fmt.Errorf("signal %q uses unknown key %q", name, keyName)
		}
		if prior, exists := boundKeys[key]; exists {
			return mapperConfig{}, fmt.Errorf("signals %q and %q use duplicate key binding %q", prior, name, keyName)
		}
		boundKeys[key] = name
	}
	for name := range signals {
		if _, exists := raw.Keys[name]; !exists {
			return mapperConfig{}, fmt.Errorf("signal %q has no key binding", name)
		}
	}

	config := mapperConfig{vendorID: device.vendorID, productID: device.productID}
	for _, name := range knownSignalNames {
		signal, exists := signals[name]
		if !exists {
			continue
		}
		offset, match, err := parseReportRule(signal.report)
		if err != nil {
			return mapperConfig{}, fmt.Errorf("signal %q: %w", name, err)
		}
		key, ok := keyCodeForName(raw.Keys[name])
		if !ok {
			return mapperConfig{}, fmt.Errorf("signal %q uses unknown key %q", name, raw.Keys[name])
		}
		config.rules = append(config.rules, signalRule{
			name: name, offset: offset, match: match, mask: len(match) == 1,
			exclusiveGroup: signal.exclusiveGroup, key: key,
		})
		if end := offset + len(match); end > config.minReportBytes {
			config.minReportBytes = end
		}
		if !containsKey(config.keys, key) {
			config.keys = append(config.keys, key)
		}
	}
	return config, nil
}

func buildDeviceConfig(vendorID, productID uint64) (deviceConfig, error) {
	if vendorID == 0 || vendorID > 0xFFFF {
		return deviceConfig{}, fmt.Errorf("vendor_id must be a non-zero 16-bit value")
	}
	if productID == 0 || productID > 0xFFFF {
		return deviceConfig{}, fmt.Errorf("product_id must be a non-zero 16-bit value")
	}
	return deviceConfig{vendorID: uint16(vendorID), productID: uint16(productID)}, nil
}

type configuredSignal struct {
	report         string
	exclusiveGroup string
}

func collectSignals(groups fileSignals) (map[string]configuredSignal, error) {
	signals := make(map[string]configuredSignal, len(groups.Face)+len(groups.DPad)+len(groups.System)+len(groups.Shoulders)+len(groups.Sticks))
	if err := addSignalGroup(signals, "Face", groups.Face, faceSignalNames, ""); err != nil {
		return nil, err
	}
	if err := addSignalGroup(signals, "DPad", groups.DPad, dpadSignalNames, ""); err != nil {
		return nil, err
	}
	if err := addSignalGroup(signals, "System", groups.System, systemSignalNames, ""); err != nil {
		return nil, err
	}
	if err := addSignalGroup(signals, "Shoulders", groups.Shoulders, shoulderSignalNames, ""); err != nil {
		return nil, err
	}
	if err := addSignalGroup(signals, "Sticks", groups.Sticks, stickSignalNames, ""); err != nil {
		return nil, err
	}
	for _, name := range []string{"LeftThumbUp", "LeftThumbDown", "LeftThumbLeft", "LeftThumbRight"} {
		if signal, exists := signals[name]; exists {
			signal.exclusiveGroup = "left-stick-directions"
			signals[name] = signal
		}
	}
	for _, name := range []string{"RightThumbUp", "RightThumbDown", "RightThumbLeft", "RightThumbRight"} {
		if signal, exists := signals[name]; exists {
			signal.exclusiveGroup = "right-stick-directions"
			signals[name] = signal
		}
	}
	return signals, nil
}

func addSignalGroup(signals map[string]configuredSignal, group string, reports map[string]string, allowed []string, exclusiveGroup string) error {
	allowedNames := make(map[string]bool, len(allowed))
	for _, name := range allowed {
		allowedNames[name] = true
	}
	for name, report := range reports {
		if !allowedNames[name] {
			return fmt.Errorf("signal %q is not valid in signals.%s", name, group)
		}
		if _, exists := signals[name]; exists {
			return fmt.Errorf("signal %q is configured more than once", name)
		}
		signals[name] = configuredSignal{report: report, exclusiveGroup: exclusiveGroup}
	}
	return nil
}

// parseReportRule returns the bracketed span's byte offset and values. Context
// bytes are syntactically validated but intentionally never matched.
func parseReportRule(report string) (int, []byte, error) {
	tokens := strings.Fields(report)
	if len(tokens) == 0 {
		return 0, nil, fmt.Errorf("report is empty")
	}
	if len(tokens) > maxInputReportBytes {
		return 0, nil, fmt.Errorf("report has %d bytes; maximum is %d", len(tokens), maxInputReportBytes)
	}

	inSpan := false
	foundSpan := false
	spanOffset := 0
	match := []byte(nil)
	byteOffset := 0
	for _, token := range tokens {
		if strings.HasPrefix(token, "[") {
			if inSpan || foundSpan {
				return 0, nil, fmt.Errorf("report must contain exactly one bracketed byte span")
			}
			inSpan = true
			foundSpan = true
			spanOffset = byteOffset
			token = strings.TrimPrefix(token, "[")
		} else if strings.Contains(token, "[") {
			return 0, nil, fmt.Errorf("invalid bracket placement")
		}

		endsSpan := strings.HasSuffix(token, "]")
		if endsSpan {
			if !inSpan {
				return 0, nil, fmt.Errorf("invalid bracket placement")
			}
			token = strings.TrimSuffix(token, "]")
		} else if strings.Contains(token, "]") {
			return 0, nil, fmt.Errorf("invalid bracket placement")
		}
		if token == "" {
			return 0, nil, fmt.Errorf("bracketed byte span must not be empty")
		}
		if len(token) != 2 {
			return 0, nil, fmt.Errorf("%q is not a two-digit hexadecimal byte", token)
		}
		value, err := strconv.ParseUint(token, 16, 8)
		if err != nil {
			return 0, nil, fmt.Errorf("%q is not a two-digit hexadecimal byte", token)
		}
		if inSpan {
			match = append(match, byte(value))
		}
		byteOffset++
		if endsSpan {
			inSpan = false
		}
	}
	if inSpan || !foundSpan || len(match) == 0 {
		return 0, nil, fmt.Errorf("report must contain exactly one non-empty bracketed byte span")
	}
	return spanOffset, match, nil
}

func containsKey(keys []keyCode, wanted keyCode) bool {
	for _, key := range keys {
		if key == wanted {
			return true
		}
	}
	return false
}

func (config mapperConfig) desiredKeys(report []byte) (map[keyCode]bool, bool) {
	if len(report) < config.minReportBytes {
		return nil, false
	}
	active := make([]bool, len(config.rules))
	groups := make(map[string]int)
	for i, rule := range config.rules {
		if rule.matches(report) {
			active[i] = true
			if rule.exclusiveGroup != "" {
				groups[rule.exclusiveGroup]++
			}
		}
	}
	desired := make(map[keyCode]bool, len(config.keys))
	for i, rule := range config.rules {
		if active[i] && (rule.exclusiveGroup == "" || groups[rule.exclusiveGroup] == 1) {
			desired[rule.key] = true
		}
	}
	return desired, true
}

func (rule signalRule) matches(report []byte) bool {
	if rule.offset+len(rule.match) > len(report) {
		return false
	}
	if rule.mask {
		return report[rule.offset]&rule.match[0] != 0
	}
	return string(report[rule.offset:rule.offset+len(rule.match)]) == string(rule.match)
}

func (config mapperConfig) activeSignalNames(report []byte) ([]string, bool) {
	if len(report) < config.minReportBytes {
		return nil, false
	}
	active := make([]bool, len(config.rules))
	groups := make(map[string]int)
	for i, rule := range config.rules {
		if rule.matches(report) {
			active[i] = true
			if rule.exclusiveGroup != "" {
				groups[rule.exclusiveGroup]++
			}
		}
	}
	names := make([]string, 0, len(config.rules))
	for i, rule := range config.rules {
		if active[i] && (rule.exclusiveGroup == "" || groups[rule.exclusiveGroup] == 1) {
			names = append(names, rule.name)
		}
	}
	return names, true
}
