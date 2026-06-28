// Package ffi adapts the vaxis terminal library to a small, game-shaped API.
package ffi

import "git.sr.ht/~rockorager/vaxis"

// New opens the terminal in the alternate screen with the given title.
func New(title string) (*vaxis.Vaxis, error) {
	vx, err := vaxis.New(vaxis.Options{DisableMouse: true})
	if err != nil {
		return nil, err
	}
	if title != "" {
		vx.SetTitle(title)
	}
	vx.HideCursor()
	return vx, nil
}

// DrawText writes text starting at (x, y), advancing by each grapheme's width.
func DrawText(vx *vaxis.Vaxis, x int, y int, text string) {
	if vx == nil || text == "" {
		return
	}
	win := vx.Window()
	col := x
	for _, ch := range vaxis.Characters(text) {
		win.SetCell(col, y, vaxis.Cell{Character: ch})
		col += ch.Width
	}
}

// ReadKey blocks for the next key press and returns a normalized name the game
// understands: "q", "r", "up"/"down"/"left"/"right", "select", "1".."9", or
// "redraw" on a resize.
func ReadKey(vx *vaxis.Vaxis) (string, error) {
	if vx == nil {
		return "q", nil
	}
	for ev := range vx.Events() {
		switch ev := ev.(type) {
		case vaxis.Key:
			// With the Kitty keyboard protocol a press emits both a press and a
			// release event; act only on the press so each keystroke applies once.
			if ev.EventType != vaxis.EventPress {
				continue
			}
			switch ev.String() {
			case "Ctrl+c", "Esc", "q":
				return "q", nil
			case "r":
				return "r", nil
			case "Up", "k":
				return "up", nil
			case "Down", "j":
				return "down", nil
			case "Left", "h":
				return "left", nil
			case "Right", "l":
				return "right", nil
			case "Enter", "Space":
				return "select", nil
			case "1", "2", "3", "4", "5", "6", "7", "8", "9":
				return ev.String(), nil
			}
		case vaxis.Resize:
			return "redraw", nil
		}
	}
	return "q", nil
}
