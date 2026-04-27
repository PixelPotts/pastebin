package main

import (
	"bytes"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"log"
	"os"
	"os/exec"
	"strings"
	"sync/atomic"
)

const logFile = "/mnt/1tb-ssd/random/pastebin/debug.log"

// ClipEvent signals the UI to show or hide.
type ClipEvent int

const (
	EventShow ClipEvent = iota
	EventHide
)

var popupVisible atomic.Bool

func main() {
	f, err := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		fmt.Fprintf(os.Stderr, "cannot open log: %v\n", err)
		os.Exit(1)
	}
	log.SetOutput(f)
	log.SetFlags(log.Ltime | log.Lmicroseconds)
	log.Println("[main] pastebin starting...")
	log.Printf("[main] ANTHROPIC_API_KEY set: %v", os.Getenv("ANTHROPIC_API_KEY") != "")
	initClient("")
	log.Println("[main] claude client ready")
	events := make(chan ClipEvent, 10)
	go MonitorKeyboard(events)
	log.Println("[main] entering UI loop")
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

// ClipboardHasImage returns true if the clipboard contains an image.
func ClipboardHasImage() bool {
	targets, _ := exec.Command("xclip", "-selection", "clipboard", "-t", "TARGETS", "-o").Output()
	return strings.Contains(string(targets), "image/")
}

// ReadClipboardImageBytes returns raw image bytes from the clipboard.
func ReadClipboardImageBytes() []byte {
	for _, mime := range []string{"image/png", "image/jpeg", "image/bmp", "image/gif"} {
		out, err := exec.Command("xclip", "-selection", "clipboard", "-t", mime, "-o").Output()
		if err == nil && len(out) > 8 {
			return out
		}
	}
	return nil
}

// ReadClipboardImage returns a decoded image from the clipboard, or nil.
func ReadClipboardImage() image.Image {
	data := ReadClipboardImageBytes()
	if data == nil {
		return nil
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil
	}
	return img
}
