// Package ffi: terminal glue. exec.Command's variadic arg and term.Option
// functional options are inexpressible in Ard, so they live here; the terminal
// widget itself and its OnEvent handler stay in Ard.
package ffi

import (
	"os"
	"os/exec"

	"go.rockorager.dev/vaxis/widgets/term"
)

// ShellCommand builds the interactive shell command for the terminal widget.
func ShellCommand() *exec.Cmd {
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/sh"
	}
	return exec.Command(shell, "-i")
}

// TerminalOptions returns the demo terminal's options.
func TerminalOptions() []term.Option {
	return []term.Option{term.WithKittyKeyboard(true)}
}
