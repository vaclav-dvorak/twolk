package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/vaclav-dvorak/twolk/internal/armory"
	"github.com/vaclav-dvorak/twolk/internal/config"
	"github.com/vaclav-dvorak/twolk/internal/game"
)

// Adding a character (i: realm, then name — stored as a name-only
// placeholder) and refreshing it (r: armory-API fetch or pasted export,
// the only way gear comes in). The refresh step has no text field, so
// a paste can only ever be the export.

// realmRow is one selectable line of the import wizard's first step.
// The server picks the game version: importing onto a row makes its
// module the active one.
type realmRow struct {
	game   *game.Game
	server *game.Server
	realm  string
}

// realmRows flattens every game module's server registry.
func (m Model) realmRows() []realmRow {
	var rows []realmRow
	for _, g := range m.games {
		for i := range g.Servers {
			s := &g.Servers[i]
			for _, r := range s.Realms {
				rows = append(rows, realmRow{game: g, server: s, realm: r})
			}
		}
	}
	return rows
}

func (m Model) importSelection() realmRow {
	rows := m.realmRows()
	return rows[min(max(m.impCursor, 0), len(rows)-1)]
}

// homeRow finds the active character's home in the realm registry —
// the row the refresh flow (r) operates on.
func (m Model) homeRow() (realmRow, int, bool) {
	i := m.cfg.ActiveChar
	if m.char == nil || i < 0 || i >= len(m.cfg.Chars) {
		return realmRow{}, 0, false
	}
	sc := m.cfg.Chars[i]
	for j, row := range m.realmRows() {
		if row.game.Slug == sc.Game && row.server.Slug == sc.Server &&
			strings.EqualFold(row.realm, sc.Realm) {
			return row, j, true
		}
	}
	return realmRow{}, 0, false
}

func (m Model) updateImport(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if msg.String() == "ctrl+c" {
		return m, tea.Quit
	}
	if m.impRefresh {
		return m.updateRefresh(msg)
	}

	// adding, step 0: where does the character play
	if m.impStep == 0 {
		switch msg.String() {
		case "esc":
			m.mode = modeNormal
			m.errMsg = ""
		case "up", "k":
			if m.impCursor > 0 {
				m.impCursor--
			}
		case "down", "j":
			if m.impCursor < len(m.realmRows())-1 {
				m.impCursor++
			}
		case "enter", " ":
			m.impStep = 1
			m.errMsg = ""
			m.impName.SetValue("")
			return m, m.impName.Focus()
		}
		return m, nil
	}

	// adding, step 1: the name field owns every key but enter/esc
	switch msg.Type {
	case tea.KeyEsc:
		m.impName.Blur()
		m.impStep = 0
		m.errMsg = ""
		return m, nil
	case tea.KeyEnter:
		return m.addCharacter()
	}
	var cmd tea.Cmd
	m.impName, cmd = m.impName.Update(msg)
	return m, cmd
}

// addCharacter stores the named character on the selected realm — a
// name-only placeholder until its first refresh brings the gear — and
// goes straight into that refresh. A name already stored on the realm
// is switched to instead, never overwritten.
func (m Model) addCharacter() (tea.Model, tea.Cmd) {
	sel := m.importSelection()
	name := strings.TrimSpace(m.impName.Value())
	if name == "" {
		m.errMsg = "type the character name"
		return m, nil
	}
	meta := config.StoredCharacter{Game: sel.game.Slug, Server: sel.server.Slug, Realm: sel.realm}
	idx := -1
	chars := m.storedChars()
	for i, sc := range m.cfg.Chars {
		if sc.Game == meta.Game && sc.Server == meta.Server && strings.EqualFold(sc.Realm, meta.Realm) &&
			chars[i] != nil && strings.EqualFold(chars[i].Name, name) {
			idx = i
			break
		}
	}
	if idx < 0 {
		raw, _ := json.Marshal(map[string]string{"name": name, "realm": sel.realm})
		if err := m.cfg.UpsertCharacter(raw, meta, -1); err != nil {
			m.errMsg = err.Error()
			return m, nil
		}
		idx = len(m.cfg.Chars) - 1
	}
	m.impName.Blur()
	cmd := m.activateChar(idx)
	m.errMsg = ""
	next, refresh := m.startRefresh()
	return next, tea.Batch(cmd, refresh)
}

// startRefresh opens the refresh step for the active character: its
// name and realm are fixed, so the modal has no text field and a paste
// can only be the export. Fetch servers start fetching at once.
func (m Model) startRefresh() (Model, tea.Cmd) {
	_, j, ok := m.homeRow()
	if !ok {
		return m, nil
	}
	m.mode = modeImport
	m.impRefresh = true
	m.impStep = 1
	m.impCursor = j
	m.impName.SetValue(m.char.Name)
	m.pasted, m.typedPaste = nil, false
	m.importNote = ""
	if m.importSelection().server.Import == game.ImportFetch {
		next, cmd := m.startFetch()
		return next.(Model), cmd
	}
	return m, nil
}

// updateRefresh is the refresh step: fetch servers fetch, paste servers
// take the export. Successive pastes append, so terminals that split a
// large paste into chunks still work.
func (m Model) updateRefresh(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.importSelection().server.Import == game.ImportFetch {
		switch msg.String() {
		case "esc":
			m.mode = modeNormal
			m.errMsg = ""
		case "enter":
			if !m.impFetching {
				return m.startFetch()
			}
		}
		return m, nil
	}
	if msg.Paste {
		m.pasted = append(m.pasted, exportText(msg.Runes)...)
		m.typedPaste = false
		m.errMsg, m.importNote = "", ""
		return m, nil
	}
	// Windows consoles have no bracketed paste: a paste arrives as one
	// keystroke per character. A "{" opens the export and every rune
	// after it is data, not a shortcut ("c" would clear, "y" copy).
	if startsExport(msg) || (m.typedPaste && isTyped(msg)) {
		return m.typedPasteKey(msg), nil
	}
	switch msg.String() {
	case "esc":
		if m.typedPaste {
			// drop a half-received export, stay on the step
			m.pasted, m.typedPaste = nil, false
			m.errMsg = ""
			return m, nil
		}
		m.mode = modeNormal
		m.pasted = nil
		m.errMsg = ""
	case "c":
		m.pasted, m.typedPaste = nil, false
		m.errMsg = ""
	case "enter":
		m.typedPaste = false // the keystroke paste is over either way
		if len(m.pasted) > 0 {
			return m.saveImport()
		}
	case "y":
		if ArmorySnippet == "" {
			m.errMsg = "snippet unavailable in this build — see tools/armory-export.js"
			return m, nil
		}
		if err := copyToClipboard(ArmorySnippet); err != nil {
			m.errMsg = err.Error()
			return m, nil
		}
		m.errMsg = ""
		m.importNote = "✓ snippet copied — paste it into the browser console on your armory page"
	}
	return m, nil
}

// isTyped reports a printable keystroke (a rune or a space). Alt is
// deliberately allowed: on layouts that type "{", "}", "[", "]" or "@"
// with AltGr (Czech, German, Polish...), Windows reports Ctrl+Right Alt,
// and bubbletea delivers the character as runes WITH Alt set — exactly
// the characters an export JSON is built from.
func isTyped(msg tea.KeyMsg) bool {
	return msg.Type == tea.KeyRunes || msg.Type == tea.KeySpace
}

// startsExport reports a typed "{" — the first character of an export
// JSON delivered as keystrokes.
func startsExport(msg tea.KeyMsg) bool {
	return isTyped(msg) && !msg.Paste && len(msg.Runes) > 0 && msg.Runes[0] == '{'
}

// typedPasteKey appends one keystroke of a character-by-character paste
// to the export buffer; a "{" starts a fresh one.
func (m Model) typedPasteKey(msg tea.KeyMsg) Model {
	if startsExport(msg) && !m.typedPaste {
		m.pasted, m.typedPaste = nil, false
	}
	m.typedPaste = true
	if msg.Type == tea.KeySpace {
		m.pasted = append(m.pasted, ' ')
	} else {
		m.pasted = append(m.pasted, exportText(msg.Runes)...)
	}
	m.errMsg, m.importNote = "", ""
	return m
}

// exportText keeps the printable runes of incoming export text. A
// keystroke paste on an AltGr layout interleaves the Ctrl and Right Alt
// key-downs of every "{", "[" or "@", and bubbletea's Windows reader
// delivers those as runes carrying the console's empty character, NUL —
// which broke the JSON. The export is one line, so no control
// character belongs in it.
func exportText(runes []rune) []byte {
	var b []byte
	for _, r := range runes {
		if r >= 0x20 && r != 0x7f {
			b = append(b, string(r)...)
		}
	}
	return b
}

// startFetch kicks off the armory-API fetch for the wizard's current
// realm and name (callers guarantee a non-empty name).
func (m Model) startFetch() (tea.Model, tea.Cmd) {
	sel := m.importSelection()
	m.impFetching = true
	m.errMsg = ""
	name, realm, fetch := strings.TrimSpace(m.impName.Value()), sel.realm, sel.server.Fetch
	return m, tea.Batch(m.spin.Tick, func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		raw, err := fetch(ctx, realm, name)
		return importFetchedMsg{raw: raw, err: err}
	})
}

// saveImport validates the pending export (pasted or fetched) against
// the character being refreshed and stores it in that character's
// place; its realm's game module becomes the active one.
func (m Model) saveImport() (tea.Model, tea.Cmd) {
	sel := m.importSelection()
	char, err := armory.Parse(m.pasted)
	if err != nil {
		m.errMsg = err.Error()
		return m, nil
	}
	if len(char.Equipment) == 0 {
		m.errMsg = "not an armory export (no equipment)"
		return m, nil
	}
	// the export names its character — catch the wrong armory page
	// before it overwrites someone else
	if m.char != nil && !strings.EqualFold(char.Name, m.char.Name) {
		m.errMsg = fmt.Sprintf("export is for %s, not %s", char.Name, m.char.Name)
		return m, nil
	}
	// endgame only: weights, caps and the candidate pool all assume
	// a max-level character, so anything else would just mislead
	if char.Level != sel.game.PlayerLevel {
		m.errMsg = fmt.Sprintf("%s is level %d — only level %d characters are supported",
			char.Name, char.Level, sel.game.PlayerLevel)
		return m, nil
	}
	if char.Realm != "" && !strings.EqualFold(char.Realm, sel.realm) {
		m.errMsg = fmt.Sprintf("export is from realm %s, but %s is selected", char.Realm, sel.realm)
		return m, nil
	}
	meta := config.StoredCharacter{Game: sel.game.Slug, Server: sel.server.Slug, Realm: sel.realm}
	if err := m.cfg.UpsertCharacter(m.pasted, meta, m.cfg.ActiveChar); err != nil {
		m.errMsg = err.Error()
		return m, nil
	}
	if err := m.cfg.Save(); err != nil {
		m.errMsg = err.Error()
		return m, nil
	}
	cmd := m.switchGame(sel.game)
	m.char = char
	m.cursor = 0
	m.gapsSeen = "" // a fresh import re-raises the gear warning if it still applies
	m.rebuildScorer()
	m.status = fmt.Sprintf("imported %s @ %s", char.Name, sel.realm)
	for _, eq := range char.Equipment {
		if eq.HasDetail {
			m.status += " (with gems & enchants)"
			break
		}
	}
	m.mode = modeNormal
	m.impRefresh = false
	m.pasted, m.typedPaste = nil, false
	m.errMsg = ""
	return m, cmd
}

// realmImported renders the add step's "on this realm" line: the
// characters already stored on the SELECTED realm (typing one of their
// names switches to it instead of adding a duplicate).
func (m Model) realmImported(sel realmRow) []string {
	chars := m.storedChars()
	var names []string
	for i, sc := range m.cfg.Chars {
		if sc.Game != sel.game.Slug || sc.Server != sel.server.Slug ||
			!strings.EqualFold(sc.Realm, sel.realm) || chars[i] == nil {
			continue
		}
		entry := lipgloss.NewStyle().Foreground(classColor(chars[i].Class)).Bold(true).
			Render(chars[i].Name)
		switch age := charAge(sc.ImportedAt); age {
		case "":
		case "just now":
			entry += dimStyle.Render(" · just now")
		default:
			entry += dimStyle.Render(" · " + age + " old")
		}
		names = append(names, entry)
	}
	if len(names) == 0 {
		return nil
	}
	return []string{dimStyle.Render("on this realm: ") + strings.Join(names, dimStyle.Render(", "))}
}

func (m Model) viewImport() string {
	// the modal chrome follows the SELECTED realm's game, not the active
	// one: moving the cursor between wotlk and cata rows re-tints title,
	// hotkeys and border live, and step 1 renders in the chosen realm's
	// colors even when importing across games
	accent := lipgloss.Color(m.importSelection().game.Accent)
	title := titleStyle.Foreground(accent)
	key := helpKey.Foreground(accent)
	header := title.Render("Add character")
	if m.impRefresh {
		header = title.Render("Refresh character")
	}

	sel := m.importSelection()
	home := dimStyle.Render(sel.server.Name+" — "+sel.realm) + dimStyle.Render("  ·  "+sel.game.Name)
	var content, footer string
	switch {
	case !m.impRefresh && m.impStep == 0:
		rows := m.realmRows()
		// one aligned column of homes, each tagged with its game's
		// flavor in the expansion's signature color
		homeW := 0
		for _, row := range rows {
			homeW = max(homeW, len(row.server.Name+" — "+row.realm))
		}
		lines := []string{dimStyle.Render("Where does the character play?"), ""}
		for i, row := range rows {
			accent := lipgloss.NewStyle().Foreground(lipgloss.Color(row.game.Accent))
			cursor, home := "  ", fmt.Sprintf("%-*s", homeW, row.server.Name+" — "+row.realm)
			if i == m.impCursor {
				cursor, home = accent.Render("▸ "), accent.Bold(true).Render(home)
			}
			lines = append(lines, cursor+home+"  "+accent.Render("("+row.game.Flavor+")"))
		}
		content = strings.Join(lines, "\n")
		footer = key.Render("enter") + helpDesc.Render(" continue  ") +
			key.Render("esc") + helpDesc.Render(" cancel")

	case !m.impRefresh:
		lines := []string{home, dimStyle.Render("character name: ") + m.impName.View(), ""}
		lines = append(lines, m.realmImported(sel)...)
		content = strings.Join(lines, "\n")
		footer = key.Render("enter") + helpDesc.Render(" save  ") +
			key.Render("esc") + helpDesc.Render(" back")

	case sel.server.Import == game.ImportFetch:
		lines := []string{home, dimStyle.Render("character: ") + m.char.Name, "",
			title.Render("fetch from the armory:"),
			dimStyle.Render(" enter pulls the character straight from " + sel.server.Name)}
		if m.impFetching {
			lines = append(lines, "", key.Render(m.spin.View()+"fetching "+m.char.Name+" @ "+sel.realm))
			footer = key.Render(m.spin.View()) + helpDesc.Render("fetching  ") +
				key.Render("esc") + helpDesc.Render(" cancel")
		} else {
			footer = key.Render("enter") + helpDesc.Render(" fetch  ") +
				key.Render("esc") + helpDesc.Render(" cancel")
		}
		content = strings.Join(lines, "\n")

	case len(m.pasted) == 0:
		// styled spans must not cross line breaks: lipgloss pads every
		// line of a multi-line Render to the block width, which would
		// push the inline key hint to the right
		lines := []string{home, dimStyle.Render("character: ") + m.char.Name, "",
			title.Render("export the character:"),
			dimStyle.Render(" 1. open your character's armory page in the browser"),
			dimStyle.Render("    (it passes the site's bot check)"),
			dimStyle.Render("    " + sel.server.PageURL(sel.realm, m.char.Name)),
			dimStyle.Render(" 2. press ") + key.Render("y") + dimStyle.Render(" here to copy the export snippet"),
			dimStyle.Render(" 3. open the browser devtools console (F12 / cmd+opt+J),"),
			dimStyle.Render("    paste the snippet, press enter — the export JSON"),
			dimStyle.Render("    is placed on your clipboard"),
			dimStyle.Render(" 4. come back here and paste (cmd/ctrl+v)")}
		content = strings.Join(lines, "\n")
		footer = key.Render("y") + helpDesc.Render(" copy browser snippet  ") +
			key.Render("esc") + helpDesc.Render(" cancel")

	default:
		validity := okStyle.Render("✓ valid JSON")
		if !json.Valid(m.pasted) {
			validity = errStyle.Render("✗ not valid JSON")
		}
		content = fmt.Sprintf("%d bytes pasted  %s\n\n%s",
			len(m.pasted), validity, dimStyle.Render(preview(m.pasted, 3, 60)))
		footer = key.Render("enter") + helpDesc.Render(" save  ") +
			key.Render("c") + helpDesc.Render(" clear paste  ") +
			key.Render("esc") + helpDesc.Render(" cancel")
	}
	if m.importNote != "" {
		footer = okStyle.Render(m.importNote) + "\n\n" + footer
	}
	if m.errMsg != "" {
		footer = errStyle.Render(m.errMsg) + "\n\n" + footer
	}
	modal := modalStyle.BorderForeground(accent).Width(min(m.width-4, 72)).Render(lipgloss.JoinVertical(lipgloss.Left,
		header,
		"",
		content,
		"",
		footer,
	))
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, modal)
}

// preview returns the first lines of raw, each truncated to width runes.
func preview(raw []byte, lines, width int) string {
	var out []string
	for _, line := range strings.SplitN(string(raw), "\n", lines+1) {
		if len(out) == lines {
			out = append(out, "…")
			break
		}
		r := []rune(line)
		if len(r) > width {
			line = string(r[:width]) + "…"
		}
		out = append(out, line)
	}
	return strings.Join(out, "\n")
}
