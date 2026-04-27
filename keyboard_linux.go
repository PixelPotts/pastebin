//go:build linux

package main

import (
	"encoding/binary"
	"io"
	"log"
	"os"
	"strings"
	"time"
)

const (
	evKey      uint16 = 0x01
	valPress   int32  = 1
	valRelease int32  = 0

	keyLeftCtrl  uint16 = 29
	keyRightCtrl uint16 = 97
	keyC         uint16 = 46
	keyEscape    uint16 = 1

	inputEventSize  = 24 // sizeof(struct input_event) on 64-bit
	doublePressMs   = 800
	longPressMs     = 500
	dedupMs         = 50
)

type rawKeyEvent struct {
	code  uint16
	value int32
}

// findKeyboards returns /dev/input/event* paths for keyboard devices.
func findKeyboards() []string {
	data, err := os.ReadFile("/proc/bus/input/devices")
	if err != nil {
		return nil
	}

	var devices []string
	for _, block := range strings.Split(string(data), "\n\n") {
		if !strings.Contains(block, "kbd") {
			continue
		}
		// Skip devices that are only buttons (sleep, power)
		if strings.Contains(block, "Button") {
			continue
		}
		for _, line := range strings.Split(block, "\n") {
			if !strings.HasPrefix(line, "H: Handlers=") {
				continue
			}
			for _, field := range strings.Fields(line[len("H: Handlers="):]) {
				if strings.HasPrefix(field, "event") {
					devices = append(devices, "/dev/input/"+field)
				}
			}
		}
	}
	return devices
}

func readDevice(path string, ch chan<- rawKeyEvent) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()

	buf := make([]byte, inputEventSize)
	for {
		if _, err := io.ReadFull(f, buf); err != nil {
			return
		}
		typ := binary.LittleEndian.Uint16(buf[16:18])
		if typ != evKey {
			continue
		}
		code := binary.LittleEndian.Uint16(buf[18:20])
		val := int32(binary.LittleEndian.Uint32(buf[20:24]))
		ch <- rawKeyEvent{code, val}
	}
}

// MonitorKeyboard watches for Ctrl+C double-press / long-press and ESC.
func MonitorKeyboard(events chan<- ClipEvent) {
	keyboards := findKeyboards()
	if len(keyboards) == 0 {
		log.Fatal("no keyboard devices found — ensure you are in the 'input' group")
	}

	keyCh := make(chan rawKeyEvent, 256)
	for _, path := range keyboards {
		go readDevice(path, keyCh)
	}

	var (
		ctrlHeld      bool
		lastCtrlC     time.Time
		lastCPress    time.Time // dedup across multiple devices
		holdTimer     *time.Timer
	)

	for ev := range keyCh {
		switch ev.code {
		case keyLeftCtrl, keyRightCtrl:
			ctrlHeld = ev.value != valRelease
			// If ctrl released while hold timer active, cancel it
			if !ctrlHeld && holdTimer != nil {
				holdTimer.Stop()
				holdTimer = nil
			}

		case keyC:
			if !ctrlHeld || ev.value == valRelease {
				// On release, cancel hold timer
				if ev.value == valRelease && holdTimer != nil {
					holdTimer.Stop()
					holdTimer = nil
				}
				continue
			}
			// Deduplicate across multiple keyboard devices
			now := time.Now()
			if now.Sub(lastCPress) < dedupMs*time.Millisecond {
				continue
			}
			lastCPress = now

			if popupVisible.Load() {
				// Ctrl+C while popup open → close
				events <- EventHide
				lastCtrlC = time.Time{}
			} else if !lastCtrlC.IsZero() && now.Sub(lastCtrlC) < doublePressMs*time.Millisecond {
				// Double press → show
				events <- EventShow
				lastCtrlC = time.Time{}
				if holdTimer != nil {
					holdTimer.Stop()
					holdTimer = nil
				}
			} else {
				// First press — start tracking
				lastCtrlC = now
				if holdTimer != nil {
					holdTimer.Stop()
				}
				holdTimer = time.AfterFunc(longPressMs*time.Millisecond, func() {
					if !popupVisible.Load() {
						events <- EventShow
					}
				})
			}

		case keyEscape:
			if ev.value == valPress && popupVisible.Load() {
				events <- EventHide
			}
		}
	}
}
