package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/google/gousb"
)

func main() {
	configPath := flag.String("config", "", "path to mapper configuration TOML")
	monitor := flag.Bool("monitor", false, "print raw USB input reports instead of mapping keys")
	debug := flag.Bool("debug", false, "log USB interface-selection diagnostics")
	flag.Parse()

	if *monitor {
		device := defaultDeviceConfig()
		if *configPath != "" {
			var err error
			device, err = loadMonitorConfig(*configPath)
			if err != nil {
				log.Fatalf("Invalid monitor config: %v", err)
			}
		}
		runController(device, true, mapperConfig{}, *debug)
		return
	}

	config := defaultMapperConfig()
	if *configPath != "" {
		var err error
		config, err = loadMapperConfig(*configPath)
		if err != nil {
			log.Fatalf("Invalid config: %v", err)
		}
	}
	runController(deviceConfig{vendorID: config.vendorID, productID: config.productID}, false, config, *debug)
}

func runController(device deviceConfig, monitor bool, config mapperConfig, debug bool) {

	ctx := gousb.NewContext()
	defer ctx.Close()

	status := os.Stdout
	if monitor {
		// Keep stdout exclusively for reports so it can be redirected to a log.
		status = os.Stderr
	}
	fmt.Fprintf(status, "Searching for controller %04X:%04X...\n", device.vendorID, device.productID)
	dev, err := ctx.OpenDeviceWithVIDPID(gousb.ID(device.vendorID), gousb.ID(device.productID))
	if err != nil {
		log.Fatalf("Failed to open device: %v. (Did you use sudo?)", err)
	}
	if dev == nil {
		log.Fatal("Controller not found. Check your physical USB connection.")
	}
	defer dev.Close()

	_, epIn, release, err := claimControllerInterface(dev, debug)
	if err != nil {
		log.Fatalf("Failed to claim controller interface: %v", err)
	}
	defer release()

	stop := make(chan struct{})
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(signals)
	go func() {
		<-signals
		close(stop)
	}()

	if monitor {
		fmt.Fprintln(status, "Controller monitor online.")
		runReportMode(true, epIn, config, stop, nil)
		return
	}

	fmt.Fprintln(status, "Controller mapper online.")
	runReportMode(false, epIn, config, stop, func(keys []keyCode) mapperKeyboard {
		keyboard := newMacKeyboard(keys)
		if !keyboardEventAccessGranted() {
			log.Println("Warning: macOS may block synthesized keyboard events until Accessibility/Input Monitoring access is granted to this application.")
		}
		return keyboard
	})
}

// claimControllerInterface avoids DefaultInterface because this device can
// report an active configuration ID that is absent from its descriptors.
func claimControllerInterface(dev *gousb.Device, debug bool) (*gousb.Interface, *gousb.InEndpoint, func(), error) {
	if err := dev.SetAutoDetach(true); err != nil {
		return nil, nil, nil, fmt.Errorf("enable kernel driver auto-detach: %w", err)
	}

	var failures []string
	for _, configID := range configurationIDs(dev) {
		if debug {
			log.Printf("Trying configuration %d", configID)
		}
		cfg, err := dev.Config(configID)
		if err != nil {
			failures = append(failures, fmt.Sprintf("configuration %d: %v", configID, err))
			if debug {
				log.Printf("Configuration %d failed: %v", configID, err)
			}
			continue
		}

		intf, err := cfg.Interface(0, 0)
		if err != nil {
			cfg.Close()
			failures = append(failures, fmt.Sprintf("configuration %d interface 0: %v", configID, err))
			if debug {
				log.Printf("Configuration %d interface 0/0 failed: %v", configID, err)
			}
			continue
		}

		epIn, err := intf.InEndpoint(1)
		if err != nil {
			intf.Close()
			cfg.Close()
			failures = append(failures, fmt.Sprintf("configuration %d endpoint 0x81: %v", configID, err))
			if debug {
				log.Printf("Configuration %d endpoint 0x81 failed: %v", configID, err)
			}
			continue
		}

		if debug {
			log.Printf("Selected configuration %d, interface 0/0, endpoint 0x81", configID)
		}
		return intf, epIn, func() {
			intf.Close()
			cfg.Close()
		}, nil
	}

	if debug {
		logAvailableInterfaces(dev)
	}
	if len(failures) == 0 {
		return nil, nil, nil, fmt.Errorf("device has no configurations")
	}
	return nil, nil, nil, fmt.Errorf("no usable configuration: %s", strings.Join(failures, "; "))
}

func configurationIDs(dev *gousb.Device) []int {
	ids := make([]int, 0, len(dev.Desc.Configs))
	for configID := range dev.Desc.Configs {
		ids = append(ids, configID)
	}
	sort.Ints(ids)
	return ids
}

func logAvailableInterfaces(dev *gousb.Device) {
	for _, configNumber := range configurationIDs(dev) {
		configDesc := dev.Desc.Configs[configNumber]
		interfaceNumbers := make([]int, len(configDesc.Interfaces))
		for i, interfaceDesc := range configDesc.Interfaces {
			interfaceNumbers[i] = interfaceDesc.Number
		}
		sort.Ints(interfaceNumbers)
		log.Printf("Available interfaces in configuration %d: %v", configNumber, interfaceNumbers)
	}
}

type mapperKeyboard interface {
	keySynchronizer
	ReleaseAll()
}

// runReportMode dispatches after USB setup. Its factory keeps keyboard setup
// completely out of monitor mode.
func runReportMode(monitor bool, epIn *gousb.InEndpoint, config mapperConfig, stop <-chan struct{}, newKeyboard func([]keyCode) mapperKeyboard) {
	if monitor {
		liveMonitor(epIn, stop)
		return
	}
	keyboard := newKeyboard(config.keys)
	defer keyboard.ReleaseAll()
	liveReport(epIn, keyboard, config, stop)
}

func readReport(epIn *gousb.InEndpoint, stop <-chan struct{}) ([]byte, error) {
	buffer := make([]byte, maxInputReportBytes)
	for {
		select {
		case <-stop:
			return nil, context.Canceled
		default:
		}

		ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
		n, err := epIn.ReadContext(ctx, buffer)
		cancel()
		if err != nil || n == 0 {
			continue
		}
		return append([]byte(nil), buffer[:n]...), nil
	}
}

func liveReport(epIn *gousb.InEndpoint, keyboard keySynchronizer, config mapperConfig, stop <-chan struct{}) {
	lastActive := make(map[string]bool, len(config.rules))
	for {
		select {
		case <-stop:
			return
		default:
		}

		report, err := readReport(epIn, stop)
		if err != nil {
			continue
		}
		desired, valid := config.desiredKeys(report)
		if !valid {
			// A truncated report must not release a held key or alter diagnostics.
			continue
		}
		activeNames, _ := config.activeSignalNames(report)
		active := make(map[string]bool, len(activeNames))
		for _, name := range activeNames {
			active[name] = true
			if !lastActive[name] {
				fmt.Println("Pressed:", name)
			}
		}
		lastActive = active
		keyboard.Sync(desired)
	}
}

func liveMonitor(epIn *gousb.InEndpoint, stop <-chan struct{}) {
	for {
		select {
		case <-stop:
			return
		default:
		}
		report, err := readReport(epIn, stop)
		if err != nil {
			continue
		}
		fmt.Println(formatReport(report))
	}
}

func formatReport(report []byte) string {
	values := make([]string, len(report))
	for i, value := range report {
		values[i] = fmt.Sprintf("%02X", value)
	}
	return strings.Join(values, " ")
}
