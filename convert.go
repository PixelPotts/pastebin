package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"image"
	"image/jpeg"
	_ "image/png"
	"log"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	xdraw "golang.org/x/image/draw"
)

const (
	maxOCRDim    = 512
	maxImgW      = 1200
	maxImgH      = 2000
	imgMargin    = 24
	charsPerLine = 80
	pangoFont    = "DejaVu Sans Mono 14"
)

// ── clipboard writers ──

func WriteClipboardText(text string) error {
	cmd := exec.Command("xclip", "-selection", "clipboard")
	cmd.Stdin = strings.NewReader(text)
	return cmd.Run()
}

func WriteClipboardImage(data []byte) error {
	cmd := exec.Command("xclip", "-selection", "clipboard", "-t", "image/png", "-i")
	cmd.Stdin = bytes.NewReader(data)
	return cmd.Run()
}

// ── text → image (via pango-view for emoji support) ──

func TextToImage(text string) ([]byte, error) {
	wrapped := strings.Join(wrapLines(text, charsPerLine), "\n")

	tmp, err := os.CreateTemp("", "paste-*.png")
	if err != nil {
		return nil, fmt.Errorf("temp file: %w", err)
	}
	tmpPath := tmp.Name()
	tmp.Close()
	defer os.Remove(tmpPath)

	cmd := exec.Command("pango-view",
		"--font="+pangoFont,
		"--background=#1e1e1e",
		"--foreground=#d4d4d4",
		fmt.Sprintf("--margin=%d", imgMargin),
		"-q", "-o", tmpPath,
		"/dev/stdin",
	)
	cmd.Stdin = strings.NewReader(wrapped)

	if out, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("pango-view: %w: %s", err, out)
	}

	data, err := os.ReadFile(tmpPath)
	if err != nil {
		return nil, fmt.Errorf("read rendered image: %w", err)
	}
	return data, nil
}

func wrapLines(text string, maxChars int) []string {
	var out []string
	for _, para := range strings.Split(text, "\n") {
		if para == "" {
			out = append(out, "")
			continue
		}
		var line string
		for _, w := range strings.Fields(para) {
			if line == "" {
				line = w
			} else if len(line)+1+len(w) > maxChars {
				out = append(out, line)
				line = w
			} else {
				line += " " + w
			}
		}
		if line != "" {
			out = append(out, line)
		}
	}
	return out
}

// ── image → text (OCR via Claude vision) ──

func ImageToText(imgData []byte) string {
	if !clientReady {
		return "(no API key configured)"
	}
	src, _, err := image.Decode(bytes.NewReader(imgData))
	if err != nil {
		log.Printf("[ocr] decode error: %v", err)
		return "(failed to decode image)"
	}

	src = resizeMax(src, maxOCRDim)

	var buf bytes.Buffer
	jpeg.Encode(&buf, src, &jpeg.Options{Quality: 80})
	b64 := base64.StdEncoding.EncodeToString(buf.Bytes())

	log.Printf("[ocr] sending %dx%d (%d B b64) to Claude",
		src.Bounds().Dx(), src.Bounds().Dy(), len(b64))

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	resp, err := client.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     "claude-haiku-4-5",
		MaxTokens: 4096,
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(
				anthropic.NewImageBlockBase64("image/jpeg", b64),
				anthropic.NewTextBlock("Transcribe all text visible in this image exactly as written. Preserve formatting and structure. Return only the text."),
			),
		},
	})
	if err != nil {
		log.Printf("[ocr] API error: %v", err)
		return "(OCR failed: " + err.Error() + ")"
	}
	if len(resp.Content) > 0 {
		log.Printf("[ocr] success, %d chars", len(resp.Content[0].Text))
		return resp.Content[0].Text
	}
	return "(no text found)"
}

func resizeMax(src image.Image, maxDim int) image.Image {
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	if w <= maxDim && h <= maxDim {
		return src
	}
	scale := float64(maxDim) / float64(max(w, h))
	nw, nh := int(float64(w)*scale), int(float64(h)*scale)
	dst := image.NewRGBA(image.Rect(0, 0, nw, nh))
	xdraw.BiLinear.Scale(dst, dst.Bounds(), src, b, xdraw.Over, nil)
	return dst
}
