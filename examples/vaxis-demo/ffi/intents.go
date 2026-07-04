package ffi

import "go.rockorager.dev/vaxis/ui"

// demoIntent is a string-typed intent. Ard cannot define a Go interface method
// on a string newtype, so the demo's custom intents live here; the build/handler
// logic stays in Ard.
type demoIntent string

func (i demoIntent) IntentType() ui.IntentType { return ui.IntentType(i) }

// Intent values, for Shortcuts key->intent maps.
func QuitIntent() ui.Intent           { return demoIntent(quitType) }
func NextPageIntent() ui.Intent       { return demoIntent(nextPageType) }
func PreviousPageIntent() ui.Intent   { return demoIntent(previousPageType) }
func CommandPaletteIntent() ui.Intent { return demoIntent(commandPaletteType) }

// Intent-type keys, for Actions intent->handler maps.
const (
	quitType           ui.IntentType = "demo.quit"
	nextPageType       ui.IntentType = "demo.next-page"
	previousPageType   ui.IntentType = "demo.previous-page"
	commandPaletteType ui.IntentType = "demo.command-palette"
)

func QuitIntentType() ui.IntentType           { return quitType }
func NextPageIntentType() ui.IntentType       { return nextPageType }
func PreviousPageIntentType() ui.IntentType   { return previousPageType }
func CommandPaletteIntentType() ui.IntentType { return commandPaletteType }
