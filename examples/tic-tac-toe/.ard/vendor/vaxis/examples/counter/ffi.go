package ffi

import (
	"time"
	"unicode"

	"git.sr.ht/~rockorager/vaxis"
)

func mustVaxis(term any) *vaxis.Vaxis {
	return term.(*vaxis.Vaxis)
}

func TuiOpen() (*vaxis.Vaxis, error) {
	vx, err := vaxis.New(vaxis.Options{DisableKittyKeyboard: true})
	if err != nil {
		return nil, err
	}
	vx.SetTitle("Ard Vaxis Counter")
	vx.HideCursor()
	drainStartupEvents(vx)
	return vx, nil
}

func TuiClose(term any) error {
	mustVaxis(term).Close()
	return nil
}

func TuiClear(term any) {
	mustVaxis(term).Window().Clear()
}

func TuiDrawText(term any, x int, y int, text string) {
	root := mustVaxis(term).Window()
	width, _ := root.Size()
	if x < 0 || y < 0 || x >= width {
		return
	}
	root.New(x, y, width-x, 1).Print(vaxis.Segment{Text: text})
}

func TuiFlush(term any) error {
	mustVaxis(term).Render()
	return nil
}

func drainStartupEvents(vx *vaxis.Vaxis) {
	quiet := time.NewTimer(100 * time.Millisecond)
	defer quiet.Stop()
	for {
		select {
		case <-vx.Events():
			if !quiet.Stop() {
				<-quiet.C
			}
			quiet.Reset(100 * time.Millisecond)
		case <-quiet.C:
			return
		}
	}
}

func TuiReadKey(term any) (string, error) {
	for ev := range mustVaxis(term).Events() {
		switch ev := ev.(type) {
		case vaxis.Key:
			if ev.EventType != vaxis.EventPress {
				continue
			}
			if key := appKeyFromVaxisKey(ev); key != "" {
				return key, nil
			}
			// Fall back to non-printable/special-key handling.
			switch ev.String() {
			case "Ctrl+c", "Esc":
				return "q", nil
			case "Up":
				return "up", nil
			case "Down":
				return "down", nil
			case "Left":
				return "left", nil
			case "Right":
				return "right", nil
			case "Enter", "Space":
				return "select", nil
			}
		case vaxis.Resize, vaxis.Redraw:
			return "redraw", nil
		case vaxis.QuitEvent:
			return "q", nil
		}
	}
	return "q", nil
}

func appKeyFromVaxisKey(key vaxis.Key) string {
	// Do not use BaseLayoutCode here: it describes the physical PC-101 key and
	// can turn a typed app command like "r" into a movement key on alternate
	// layouts/protocols. App commands should follow the produced text/keycode.
	for _, candidate := range []rune{firstRune(key.Text), key.Keycode, key.ShiftedCode} {
		if candidate == 0 || candidate > unicode.MaxRune || !unicode.IsPrint(candidate) {
			continue
		}
		switch candidate {
		case 'q', 'Q':
			return "q"
		case 'r', 'R':
			return "r"
		case 'k', 'K':
			return "up"
		case 'j', 'J':
			return "down"
		case 'h', 'H':
			return "left"
		case 'l', 'L':
			return "right"
		case ' ':
			return "select"
		default:
			return string(candidate)
		}
	}
	return ""
}

func firstRune(text string) rune {
	for _, r := range text {
		return r
	}
	return 0
}
