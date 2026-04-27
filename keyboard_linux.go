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
		log.Printf("[kbd] FAIL open %s: %v\n", path, err)
		return
	}
	log.Printf("[kbd] reading %s\n", path)
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
	log.Printf("[kbd] found keyboards: %v\n", keyboards)
	if len(keyboards) == 0 {
		log.Fatal("no keyboard devices found — ensure you are in the 'input' group")
	}

	keyCh := make(chan rawKeyEvent, 256)
	for _, path := range keyboards {
		go readDevice(path, keyCh)
	}
	log.Println("[kbd] monitoring started — waiting for key events...")

	var (
		ctrlHeld   bool
		lastCtrlC  time.Time
		lastCPress time.Time // dedup across multiple devices
		holdTimer  *time.Timer
		closeTimer *time.Timer
	)

	for ev := range keyCh {
		switch ev.code {
		case keyLeftCtrl, keyRightCtrl:
			prev := ctrlHeld
			ctrlHeld = ev.value != valRelease
			if ctrlHeld != prev {
				log.Printf("[kbd] ctrl %s\n", map[bool]string{true: "DOWN", false: "UP"}[ctrlHeld])
			}
			if !ctrlHeld {
				if holdTimer != nil {
					holdTimer.Stop()
					holdTimer = nil
				}
				if closeTimer != nil {
					closeTimer.Stop()
					closeTimer = nil
				}
			}

		case keyC:
			if !ctrlHeld {
				continue
			}
			if ev.value == valRelease {
				// Released before timer fired — cancel
				if holdTimer != nil {
					holdTimer.Stop()
					holdTimer = nil
				}
				if closeTimer != nil {
					closeTimer.Stop()
					closeTimer = nil
				}
				continue
			}
			// Press event — deduplicate across devices
			now := time.Now()
			if now.Sub(lastCPress) < dedupMs*time.Millisecond {
				continue
			}
			lastCPress = now

			log.Printf("[kbd] Ctrl+C press (popup=%v, lastCtrlC=%v ago)\n",
				popupVisible.Load(), func() string {
					if lastCtrlC.IsZero() {
						return "never"
					}
					return now.Sub(lastCtrlC).String()
				}())

			if popupVisible.Load() {
				log.Println("[kbd] -> starting 800ms close timer")
				if closeTimer != nil {
					closeTimer.Stop()
				}
				closeTimer = time.AfterFunc(doublePressMs*time.Millisecond, func() {
					if popupVisible.Load() {
						log.Println("[kbd] -> close timer fired, hiding popup")
						events <- EventHide
					}
				})
				lastCtrlC = time.Time{}
			} else if !lastCtrlC.IsZero() && now.Sub(lastCtrlC) < doublePressMs*time.Millisecond {
				log.Println("[kbd] -> DOUBLE PRESS detected, showing popup")
				events <- EventShow
				lastCtrlC = time.Time{}
				if holdTimer != nil {
					holdTimer.Stop()
					holdTimer = nil
				}
			} else {
				log.Println("[kbd] -> first press, waiting for double/hold...")
				lastCtrlC = now
				if holdTimer != nil {
					holdTimer.Stop()
				}
				holdTimer = time.AfterFunc(longPressMs*time.Millisecond, func() {
					if !popupVisible.Load() {
						log.Println("[kbd] -> hold timer fired, showing popup")
						events <- EventShow
					}
				})
			}

		case keyEscape:
			if ev.value == valPress && popupVisible.Load() {
				log.Println("[kbd] ESC -> hiding popup")
				events <- EventHide
			}
		}
	}
}
