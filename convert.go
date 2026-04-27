package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"image/png"
	"log"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	xdraw "golang.org/x/image/draw"
	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

const (
	maxOCRDim    = 512
	maxImgW      = 1200
	maxImgH      = 2000
	renderFont   = 14.0
	imgMargin    = 24
	charsPerLine = 80
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

// ── text → image ──

func TextToImage(text string) ([]byte, error) {
	face := loadMonoFont()
	lines := wrapLines(text, charsPerLine)

	m := face.Metrics()
	lineH := m.Height.Ceil() + m.Height.Ceil()/4 // ~1.25x spacing

	maxW := 0
	for _, l := range lines {
		if w := font.MeasureString(face, l).Ceil(); w > maxW {
			maxW = w
		}
	}

	imgW := min(maxW+imgMargin*2, maxImgW)
	imgH := min(len(lines)*lineH+imgMargin*2, maxImgH)

	img := image.NewRGBA(image.Rect(0, 0, imgW, imgH))
	draw.Draw(img, img.Bounds(), &image.Uniform{color.NRGBA{30, 30, 30, 255}}, image.Point{}, draw.Src)

	d := &font.Drawer{
		Dst:  img,
		Src:  image.NewUniform(color.NRGBA{212, 212, 212, 255}),
		Face: face,
	}
	baseline := imgMargin + m.Ascent.Ceil()
	for i, l := range lines {
		y := baseline + i*lineH
		if y > imgH-imgMargin {
			break
		}
		d.Dot = fixed.P(imgMargin, y)
		d.DrawString(l)
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
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

func loadMonoFont() font.Face {
	for _, p := range []string{
		"/usr/share/fonts/truetype/dejavu/DejaVuSansMono.ttf",
		"/usr/share/fonts/truetype/liberation/LiberationMono-Regular.ttf",
		"/usr/share/fonts/truetype/ubuntu/UbuntuMono-R.ttf",
		"/usr/share/fonts/truetype/noto/NotoSansMono-Regular.ttf",
	} {
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		parsed, err := opentype.Parse(data)
		if err != nil {
			continue
		}
		face, err := opentype.NewFace(parsed, &opentype.FaceOptions{Size: renderFont, DPI: 96})
		if err != nil {
			continue
		}
		return face
	}
	return basicfont.Face7x13
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
