package ffi

import (
	"time"
	"unicode"

	"git.sr.ht/~rockorager/vaxis"
)

func New(title string) (*vaxis.Vaxis, error) {
	vx, err := vaxis.New(vaxis.Options{DisableKittyKeyboard: true})
	if err != nil {
		return nil, err
	}
	if title != "" {
		vx.SetTitle(title)
	}
	vx.HideCursor()
	drainStartupEvents(vx)
	return vx, nil
}

func Close(term *vaxis.Vaxis) error {
	if term == nil {
		return nil
	}
	term.Close()
	return nil
}

func Clear(term *vaxis.Vaxis) {
	if term == nil {
		return
	}
	term.Window().Clear()
}

func DrawText(term *vaxis.Vaxis, x int, y int, text string) {
	if term == nil {
		return
	}
	root := term.Window()
	width, _ := root.Size()
	if x < 0 || y < 0 || x >= width {
		return
	}
	root.New(x, y, width-x, 1).Print(vaxis.Segment{Text: text})
}

func Render(term *vaxis.Vaxis) error {
	if term == nil {
		return nil
	}
	term.Render()
	return nil
}

func ReadKey(term *vaxis.Vaxis) (string, error) {
	if term == nil {
		return "q", nil
	}
	for ev := range term.Events() {
		switch ev := ev.(type) {
		case vaxis.Key:
			if ev.EventType != vaxis.EventPress {
				continue
			}
			if key := appKeyFromVaxisKey(ev); key != "" {
				return key, nil
			}
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
			case "BackSpace":
				return "backspace", nil
			}
		case vaxis.Resize, vaxis.Redraw:
			return "redraw", nil
		case vaxis.QuitEvent:
			return "q", nil
		}
	}
	return "q", nil
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
