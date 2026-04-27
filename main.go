package main

import (
	"bytes"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"os"
	"os/exec"
	"strings"
	"sync/atomic"
)

// ClipEvent signals the UI to show or hide.
type ClipEvent int

const (
	EventShow ClipEvent = iota
	EventHide
)

var popupVisible atomic.Bool

func main() {
	fmt.Println("[main] pastebin starting...")
	fmt.Printf("[main] ANTHROPIC_API_KEY set: %v\n", os.Getenv("ANTHROPIC_API_KEY") != "")
	initClient("")
	fmt.Println("[main] claude client ready")
	events := make(chan ClipEvent, 10)
	go MonitorKeyboard(events)
	fmt.Println("[main] entering UI loop")
	RunUI(events)
}

// ReadClipboardText returns the current clipboard text.
func ReadClipboardText() string {
	if out, err := exec.Command("xclip", "-selection", "clipboard", "-o").Output(); err == nil {
		return string(out)
	}
	if out, err := exec.Command("xsel", "--clipboard", "--output").Output(); err == nil {
		return string(out)
	}
	if out, err := exec.Command("wl-paste").Output(); err == nil {
		return string(out)
	}
	return ""
}

// ReadClipboardImage returns a decoded image from the clipboard, or nil.
func ReadClipboardImage() image.Image {
	// Check if clipboard has image targets
	targets, err := exec.Command("xclip", "-selection", "clipboard", "-t", "TARGETS", "-o").Output()
	if err != nil || !strings.Contains(string(targets), "image/") {
		return nil
	}

	for _, mime := range []string{"image/png", "image/jpeg", "image/bmp", "image/gif"} {
		out, err := exec.Command("xclip", "-selection", "clipboard", "-t", mime, "-o").Output()
		if err == nil && len(out) > 8 {
			if img, _, err := image.Decode(bytes.NewReader(out)); err == nil {
				return img
			}
		}
	}
	return nil
}
