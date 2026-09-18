package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/google/gousb"
)

func main() {
	configPath := flag.String("config", "", "path to mapper configuration TOML")
	flag.Parse()

	config := defaultMapperConfig()
	if *configPath != "" {
		var err error
		config, err = loadMapperConfig(*configPath)
		if err != nil {
			log.Fatalf("Invalid config: %v", err)
		}
	}

	ctx := gousb.NewContext()
	defer ctx.Close()

	fmt.Printf("Searching for controller %04X:%04X...\n", config.vendorID, config.productID)
	dev, err := ctx.OpenDeviceWithVIDPID(gousb.ID(config.vendorID), gousb.ID(config.productID))
	if err != nil {
		log.Fatalf("Failed to open device: %v. (Did you use sudo?)", err)
	}
	if dev == nil {
		log.Fatal("Controller not found. Check your physical USB connection.")
	}
	defer dev.Close()

	dev.SetAutoDetach(true)
	intf, done, err := dev.DefaultInterface()
	if err != nil {
		log.Fatalf("Failed to claim default interface: %v", err)
	}
	defer done()

	epIn, err := intf.InEndpoint(1)
	if err != nil {
		log.Fatalf("Failed to open IN Endpoint 0x81: %v", err)
	}

	keyboard := newMacKeyboard(config.keys)
	if !keyboardEventAccessGranted() {
		log.Println("Warning: macOS may block synthesized keyboard events until Accessibility/Input Monitoring access is granted to this application.")
	}
	defer keyboard.ReleaseAll()

	stop := make(chan struct{})
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(signals)
	go func() {
		<-signals
		close(stop)
	}()

	fmt.Println("Controller mapper online.")
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
