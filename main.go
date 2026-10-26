package main

import (
	_ "embed"
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/vaclav-dvorak/twolk/internal/config"
	"github.com/vaclav-dvorak/twolk/internal/tui"
)

// armorySnippet is the browser-console export helper, shipped inside the
// binary so the import modal can copy it to the clipboard (y).
//
//go:embed tools/armory-export.js
var armorySnippet string

func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, "twolk:", err)
		os.Exit(1)
	}
	tui.ArmorySnippet = armorySnippet
	p := tea.NewProgram(tui.New(cfg), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "twolk:", err)
		os.Exit(1)
	}
}
