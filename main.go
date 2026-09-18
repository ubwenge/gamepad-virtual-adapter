package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/google/gousb"
)

func main() {
	configPath := flag.String("config", "", "path to mapper configuration TOML")
	monitor := flag.Bool("monitor", false, "print raw USB input reports instead of mapping keys")
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
		runController(device, true, mapperConfig{})
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
	runController(deviceConfig{vendorID: config.vendorID, productID: config.productID}, false, config)
}

func runController(device deviceConfig, monitor bool, config mapperConfig) {

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

	intf, release, err := claimControllerInterface(dev)
	if err != nil {
		log.Fatalf("Failed to claim controller interface: %v", err)
	}
	defer release()

	epIn, err := intf.InEndpoint(1)
	if err != nil {
		log.Fatalf("Failed to open IN Endpoint 0x81: %v", err)
	}

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

// claimControllerInterface avoids DefaultInterface because this device reports
// configuration ID 1 even though libusb reports its active configuration as 0.
func claimControllerInterface(dev *gousb.Device) (*gousb.Interface, func(), error) {
	if err := dev.SetAutoDetach(true); err != nil {
		return nil, nil, fmt.Errorf("enable kernel driver auto-detach: %w", err)
	}

	cfg, err := dev.Config(1)
	if err != nil {
		logAvailableInterfaces(dev)
		return nil, nil, fmt.Errorf("claim configuration 1: %w", err)
	}

	intf, err := cfg.Interface(0, 0)
	if err != nil {
		cfg.Close()
		logAvailableInterfaces(dev)
		return nil, nil, fmt.Errorf("claim interface 0 in configuration 1: %w", err)
	}

	return intf, func() {
		intf.Close()
		cfg.Close()
	}, nil
}

func logAvailableInterfaces(dev *gousb.Device) {
	for configNumber, configDesc := range dev.Desc.Configs {
		interfaceNumbers := make([]int, len(configDesc.Interfaces))
		for i, interfaceDesc := range configDesc.Interfaces {
			interfaceNumbers[i] = interfaceDesc.Number
		}
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
