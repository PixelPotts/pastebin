package main

import (
	"bytes"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"log"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

func RunUI(events <-chan ClipEvent) {
	a := app.NewWithID("dev.pastebin.viewer")
	a.Settings().SetTheme(theme.DarkTheme())

	w := a.NewWindow("pastebin")
	w.Resize(fyne.NewSize(800, 520))
	w.CenterOnScreen()

	w.SetCloseIntercept(func() {
		w.Hide()
		popupVisible.Store(false)
	})
	w.Canvas().SetOnTypedKey(func(ke *fyne.KeyEvent) {
		if ke.Name == fyne.KeyEscape {
			w.Hide()
			popupVisible.Store(false)
		}
	})

	go func() {
		for ev := range events {
			switch ev {
			case EventShow:
				log.Println("[ui] EventShow received")
				showClipboard(w)
				popupVisible.Store(true)
			case EventHide:
				log.Println("[ui] EventHide received")
				w.Hide()
				popupVisible.Store(false)
			}
		}
	}()

	a.Run()
}

func showClipboard(w fyne.Window) {
	isImage := ClipboardHasImage()

	// --- mutable state for this session ---
	var text string
	var imgData []byte

	// --- status bar ---
	status := widget.NewLabel("  loading...")
	status.TextStyle = fyne.TextStyle{Italic: true}

	// --- content area (swappable) ---
	contentBox := container.NewMax()

	setContent := func(obj fyne.CanvasObject) {
		contentBox.Objects = []fyne.CanvasObject{obj}
		contentBox.Refresh()
	}

	// --- buttons (wired up below) ---
	copyBtn := widget.NewButton("Copy", nil)
	toImgBtn := widget.NewButton("To Image", nil)
	ocrBtn := widget.NewButton("OCR", nil)
	copyBtn.Disable()
	toImgBtn.Disable()
	ocrBtn.Disable()

	btnPanel := container.NewVBox(copyBtn, toImgBtn, ocrBtn)

	// --- button handlers ---
	copyBtn.OnTapped = func() {
		if text == "" {
			return
		}
		WriteClipboardText(text)
		status.SetText("  copied!")
	}

	toImgBtn.OnTapped = func() {
		if text == "" {
			return
		}
		status.SetText("  rendering image...")
		go func() {
			data, err := TextToImage(text)
			if err != nil {
				status.SetText("  render error: " + err.Error())
				return
			}
			imgData = data
			WriteClipboardImage(data)
			setContent(buildContent(text, imgData))
			ocrBtn.Enable()
			status.SetText("  image copied to clipboard")
		}()
	}

	ocrBtn.OnTapped = func() {
		if len(imgData) == 0 {
			return
		}
		status.SetText("  extracting text...")
		skel, stopS := MakeSkeleton(false, 300)
		if len(imgData) > 0 {
			setContent(container.NewHSplit(makeImageWidget(imgData), skel))
		} else {
			setContent(skel)
		}
		go func() {
			text = ImageToText(imgData)
			stopS()
			setContent(buildContent(text, imgData))
			copyBtn.Enable()
			toImgBtn.Enable()
			status.SetText("  text extracted")
		}()
	}

	// --- skeleton loader ---
	skeleton, stopSkel := MakeSkeleton(isImage, 500)
	setContent(skeleton)

	layout := container.NewBorder(nil, status, nil, btnPanel, contentBox)
	w.SetContent(layout)
	w.Show()
	w.RequestFocus()

	// --- load clipboard content ---
	go func() {
		if isImage {
			imgData = ReadClipboardImageBytes()
			stopSkel()
			if imgData != nil {
				setContent(makeImageWidget(imgData))
				ocrBtn.Enable()
				status.SetText("  image loaded")
			} else {
				status.SetText("  clipboard empty")
			}
			return
		}

		raw := ReadClipboardText()
		stopSkel()
		if raw == "" {
			setContent(makeTextWidget("(clipboard is empty)"))
			status.SetText("  done")
			return
		}

		// Show raw text immediately
		label := widget.NewLabel(raw)
		label.Wrapping = fyne.TextWrapWord
		label.TextStyle = fyne.TextStyle{Monospace: true}
		setContent(container.NewVScroll(label))
		copyBtn.Enable()
		toImgBtn.Enable()
		status.SetText("  reformatting...")

		log.Printf("[ui] sending %d bytes to reformat", len(raw))
		text = ReformatText(raw)
		log.Printf("[ui] reformat done, %d bytes", len(text))
		label.SetText(text)
		status.SetText("  done")
	}()
}

// --- helpers ---

func buildContent(text string, imgData []byte) fyne.CanvasObject {
	hasText := text != ""
	hasImage := len(imgData) > 0
	if hasText && hasImage {
		split := container.NewHSplit(makeImageWidget(imgData), makeTextWidget(text))
		split.SetOffset(0.4)
		return split
	}
	if hasImage {
		return makeImageWidget(imgData)
	}
	return makeTextWidget(text)
}

func makeTextWidget(text string) fyne.CanvasObject {
	label := widget.NewLabel(text)
	label.Wrapping = fyne.TextWrapWord
	label.TextStyle = fyne.TextStyle{Monospace: true}
	return container.NewVScroll(label)
}

func makeImageWidget(data []byte) fyne.CanvasObject {
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return widget.NewLabel("(image decode error)")
	}
	fImg := canvas.NewImageFromImage(img)
	fImg.FillMode = canvas.ImageFillContain
	fImg.SetMinSize(fyne.NewSize(300, 250))
	return fImg
}
