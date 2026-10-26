package tui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/vaclav-dvorak/twolk/internal/armory"
)

// charAge renders how old a stored import is ("3h", "2d"); "" when the
// entry predates the timestamp.
func charAge(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 48*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	}
}

// storedChars parses every stored export; entries that no longer parse
// come back nil and render as broken.
func (m Model) storedChars() []*armory.Character {
	raws := m.cfg.Characters()
	out := make([]*armory.Character, len(raws))
	for i, raw := range raws {
		if raw == nil {
			continue
		}
		out[i], _ = armory.Parse(raw)
	}
	return out
}

// activateChar switches to the stored character at index i. The
// character's game slug selects the module; the returned command (nil
// when none is needed) loads that module's item database.
func (m *Model) activateChar(i int) tea.Cmd {
	m.cfg.SetActive(i)
	if err := m.cfg.Save(); err != nil {
		m.status = "saving failed: " + err.Error()
	}
	var cmd tea.Cmd
	if i >= 0 && i < len(m.cfg.Chars) {
		if g := m.moduleBySlug(m.cfg.Chars[i].Game); g != nil {
			cmd = m.switchGame(g)
		}
	}
	// the filter is realm-local: reload even when the game module stays
	// (switchGame no-ops on same-game realm switches)
	m.loadFilter()
	if raw, err := m.cfg.Character(); err == nil && raw != nil {
		m.char, _ = armory.Parse(raw)
	} else {
		m.char = nil
	}
	m.cursor = 0
	m.gapsSeen = "" // another character warns about its own gear
	m.rebuildScorer()
	if m.char != nil {
		m.status = fmt.Sprintf("switched to %s @ %s", m.char.Name, m.char.Realm)
	}
	return cmd
}

func (m Model) updateChars(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	n := len(m.cfg.Characters())
	switch msg.String() {
	case "esc", "c":
		m.mode = modeNormal
		return m, nil
	case "ctrl+c":
		return m, tea.Quit
	case "up", "k":
		if m.cCursor > 0 {
			m.cCursor--
		}
		return m, nil
	case "down", "j":
		if m.cCursor < n-1 {
			m.cCursor++
		}
		return m, nil
	case "enter":
		cmd := m.activateChar(m.cCursor)
		m.mode = modeNormal
		return m, cmd
	case "d":
		if n == 0 {
			return m, nil
		}
		m.cfg.RemoveCharacter(m.cCursor)
		if err := m.cfg.Save(); err != nil {
			m.status = "saving failed: " + err.Error()
		}
		if m.cCursor >= len(m.cfg.Characters()) {
			m.cCursor = max(len(m.cfg.Characters())-1, 0)
		}
		// deleting may have shifted the active slot: reload it
		cmd := m.activateChar(m.cfg.ActiveChar)
		if len(m.cfg.Characters()) == 0 {
			m.char = nil
			m.scorer = nil
			m.mode = modeNormal
		}
		return m, cmd
	}
	return m, nil
}

func (m Model) viewChars() string {
	header := titleStyle.Render("Characters")
	chars := m.storedChars()

	var lines []string
	for i, c := range chars {
		prefix := "  "
		if i == m.cCursor {
			prefix = helpKey.Render("▸ ")
		}
		active := dimStyle.Render("○ ")
		if i == m.cfg.ActiveChar {
			active = okStyle.Render("● ")
		}
		if c == nil {
			lines = append(lines, prefix+active+errStyle.Render("(unreadable entry)"))
			continue
		}
		nameStyle := lipgloss.NewStyle().Foreground(classColor(c.Class)).Bold(true)
		desc := fmt.Sprintf("  %d %s ", c.Level, c.Race)
		spec := c.ClassName
		if s := c.Spec(); s != "" {
			spec = s + " " + c.ClassName
		}
		line := prefix + active +
			nameStyle.Render(fmt.Sprintf("%-24s", truncate(c.Name+" @ "+c.Realm, 24))) +
			dimStyle.Render(desc) + nameStyle.Render(spec)
		if age := charAge(m.cfg.Chars[i].ImportedAt); age != "" {
			line += dimStyle.Render("  " + age)
		}
		lines = append(lines, line)
	}
	if len(lines) == 0 {
		lines = append(lines, dimStyle.Render("no characters — import one with i"))
	}

	footer := helpLine("enter", "switch", "d", "delete", "i", "import new (from main screen)", "esc", "close")
	modal := modalStyle.Render(lipgloss.JoinVertical(lipgloss.Left,
		header, "", strings.Join(lines, "\n"), "", footer))
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, modal)
}
