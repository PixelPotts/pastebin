package main

import (
	"fmt"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// RunUI starts the fyne application and listens for show/hide events.
// Must be called on the main goroutine.
func RunUI(events <-chan ClipEvent) {
	a := app.NewWithID("dev.pastebin.viewer")
	a.Settings().SetTheme(theme.DarkTheme())

	w := a.NewWindow("pastebin")
	w.Resize(fyne.NewSize(720, 480))
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

	// Don't show window on startup — stay hidden until triggered
	go func() {
		for ev := range events {
			switch ev {
			case EventShow:
				fmt.Println("[ui] EventShow received")
				showClipboard(w)
				popupVisible.Store(true)
			case EventHide:
				fmt.Println("[ui] EventHide received, hiding window")
				w.Hide()
				popupVisible.Store(false)
			}
		}
	}()

	a.Run()
}

func showClipboard(w fyne.Window) {
	// Try image first
	if img := ReadClipboardImage(); img != nil {
		fImg := canvas.NewImageFromImage(img)
		fImg.FillMode = canvas.ImageFillContain
		fImg.SetMinSize(fyne.NewSize(400, 300))
		w.SetContent(container.NewPadded(fImg))
		w.Show()
		w.RequestFocus()
		return
	}

	// Text — show raw immediately, then reformat in background
	raw := ReadClipboardText()
	if raw == "" {
		raw = "(clipboard is empty)"
	}

	label := widget.NewLabel(raw)
	label.Wrapping = fyne.TextWrapWord
	label.TextStyle = fyne.TextStyle{Monospace: true}

	status := widget.NewLabel("  reformatting...")
	status.TextStyle = fyne.TextStyle{Italic: true}

	scroll := container.NewVScroll(label)
	scroll.SetMinSize(fyne.NewSize(700, 430))

	w.SetContent(container.NewBorder(nil, status, nil, nil, scroll))
	w.Show()
	w.RequestFocus()

	// Reformat via Claude in background
	go func() {
		fmt.Printf("[ui] sending %d bytes to Claude for reformat...\n", len(raw))
		cleaned := ReformatText(raw)
		fmt.Printf("[ui] reformat done, got %d bytes back\n", len(cleaned))
		label.SetText(cleaned)
		status.SetText("  done")
	}()
}
