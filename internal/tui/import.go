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

// The import wizard (i) and refresh flow (r): realm select, optional
// character name, paste or armory-API fetch.

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

	// step 0: where does the character play
	if m.impStep == 0 {
		switch msg.String() {
		case "esc":
			m.mode = modeNormal
			m.pasted, m.typedPaste = nil, false
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
			// a new character is coming: empty name, ready to type
			m.impStep = 1
			m.errMsg = ""
			m.impName.SetValue("")
			return m, m.impName.Focus()
		}
		return m, nil
	}

	// step 1. While the name field is focused it owns every key —
	// including paste (cmd+v pastes the name) and cursor movement;
	// enter/esc/tab hand control back to the modal.
	paste := m.importSelection().server.Import == game.ImportPaste
	if m.impName.Focused() {
		if paste && startsExport(msg) {
			// the export arrived while the name was still being edited
			// (no name contains "{")
			m.impName.Blur()
			return m.typedPasteKey(msg), nil
		}
		switch msg.Type {
		case tea.KeyEnter, tea.KeyEsc, tea.KeyTab:
			m.impName.Blur()
			return m, nil
		}
		var cmd tea.Cmd
		m.impName, cmd = m.impName.Update(msg)
		return m, cmd
	}
	// blurred: paste lands in the export buffer; successive pastes
	// append so terminals that split large pastes into chunks still
	// work. (Fetch servers have no paste step — route to the name.)
	if msg.Paste {
		if m.importSelection().server.Import == game.ImportFetch {
			cmd := m.impName.Focus()
			m.impName, _ = m.impName.Update(msg)
			return m, cmd
		}
		m.pasted = append(m.pasted, []byte(string(msg.Runes))...)
		m.typedPaste = false
		m.errMsg = ""
		return m, nil
	}
	if m.importSelection().server.Import == game.ImportFetch {
		return m.updateImportFetch(msg)
	}
	// Windows consoles have no bracketed paste: a paste arrives as one
	// keystroke per character. A "{" opens the export and every rune
	// after it is data, not a shortcut — otherwise the "n" of "name"
	// focuses the name field and the rest of the export lands in it.
	if paste && (startsExport(msg) || (m.typedPaste && isTyped(msg))) {
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
		if m.impRefresh {
			m.mode = modeNormal
		}
		m.impStep = 0
		m.pasted, m.typedPaste = nil, false
		m.errMsg = ""
	case "n":
		if m.impRefresh {
			return m, nil // refresh is bound to the active character
		}
		return m, m.impName.Focus()
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
		m.pasted = append(m.pasted, string(msg.Runes)...)
	}
	m.errMsg = ""
	return m
}

// updateImportFetch is step 1 for ImportFetch servers: no paste, no
// snippet — a named character is pulled straight from the armory API.
func (m Model) updateImportFetch(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		if m.impRefresh {
			m.mode = modeNormal
		}
		m.impStep = 0
		m.errMsg = ""
	case "n":
		if m.impRefresh {
			return m, nil // refresh is bound to the active character
		}
		return m, m.impName.Focus()
	case "enter":
		if m.impFetching {
			return m, nil
		}
		if strings.TrimSpace(m.impName.Value()) == "" {
			m.errMsg = "type the character name first"
			return m, m.impName.Focus()
		}
		return m.startFetch()
	}
	return m, nil
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
// the wizard selection and stores it tagged with where the character
// plays; the selected server's game module becomes the active one.
func (m Model) saveImport() (tea.Model, tea.Cmd) {
	sel := m.importSelection()
	char, err := armory.Parse(m.pasted)
	if err != nil {
		m.errMsg = err.Error()
		return m, nil
	}
	// endgame only: weights, caps and the candidate pool all assume
	// a max-level character, so anything else would just mislead
	if char.Level != sel.game.PlayerLevel {
		m.errMsg = fmt.Sprintf("%s is level %d — only level %d characters are supported",
			char.Name, char.Level, sel.game.PlayerLevel)
		return m, nil
	}
	// the export knows its realm — catch a mis-selected wizard row
	// before the character is filed under the wrong home
	if char.Realm != "" && !strings.EqualFold(char.Realm, sel.realm) {
		m.errMsg = fmt.Sprintf("export is from realm %s, but %s is selected", char.Realm, sel.realm)
		return m, nil
	}
	meta := config.StoredCharacter{Game: sel.game.Slug, Server: sel.server.Slug, Realm: sel.realm}
	// re-importing the same character replaces its entry; a new
	// name (or the same name elsewhere) is added as another character
	idx := -1
	for i, sc := range m.cfg.Chars {
		if sc.Game != meta.Game || sc.Server != meta.Server || sc.Realm != meta.Realm {
			continue
		}
		if old, err := armory.Parse(sc.Raw()); err == nil && old.Name == char.Name {
			idx = i
			break
		}
	}
	if err := m.cfg.UpsertCharacter(m.pasted, meta, idx); err != nil {
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

// realmImported renders the wizard's "on this realm" block: the
// characters stored for the SELECTED realm are the only ones a save
// can update, so they are what step 1 mentions — the active character
// is irrelevant when it lives elsewhere (importing onto a foreign
// realm is a fresh import and shows nothing).
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
	return []string{
		dimStyle.Render("on this realm: ") + strings.Join(names, dimStyle.Render(", ")),
		dimStyle.Render("re-importing one of these names updates it — any other"),
		dimStyle.Render("name is added as a new character"),
		"",
	}
}

func (m Model) viewImport() string {
	// the modal chrome follows the SELECTED realm's game, not the active
	// one: moving the cursor between wotlk and cata rows re-tints title,
	// hotkeys and border live, and step 1 renders in the chosen realm's
	// colors even when importing across games
	accent := lipgloss.Color(m.importSelection().game.Accent)
	title := titleStyle.Foreground(accent)
	key := helpKey.Foreground(accent)
	header := title.Render("Import character")
	if m.impRefresh {
		header = title.Render("Refresh character")
	}

	var content, footer string
	switch {
	case m.impStep == 0:
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

	case m.importSelection().server.Import == game.ImportFetch:
		sel := m.importSelection()
		nameLine := dimStyle.Render("character name: ")
		switch {
		case m.impName.Focused():
			nameLine += m.impName.View()
		case m.impName.Value() == "":
			nameLine += dimStyle.Render("— none — press ") + key.Render("n")
		default:
			nameLine += m.impName.Value()
		}
		lines := []string{
			dimStyle.Render(sel.server.Name+" — "+sel.realm) + dimStyle.Render("  ·  "+sel.game.Name),
			nameLine,
			"",
		}
		if !m.impRefresh { // a refresh IS one of these — nothing to say
			lines = append(lines, m.realmImported(sel)...)
		}
		lines = append(lines,
			title.Render("fetch from the armory:"),
			dimStyle.Render(" enter pulls the character straight from "+sel.server.Name))
		if m.impFetching {
			lines = append(lines, "", key.Render(m.spin.View()+"fetching "+m.impName.Value()+" @ "+sel.realm))
		}
		content = strings.Join(lines, "\n")

	case len(m.pasted) == 0:
		sel := m.importSelection()
		nameLine := dimStyle.Render("character name: ")
		switch {
		case m.impName.Focused():
			nameLine += m.impName.View()
		case m.impName.Value() == "":
			nameLine += dimStyle.Render("— none —")
		default:
			nameLine += m.impName.Value()
		}
		lines := []string{
			dimStyle.Render(sel.server.Name+" — "+sel.realm) + dimStyle.Render("  ·  "+sel.game.Name),
			nameLine,
			"",
		}
		if !m.impRefresh { // a refresh IS one of these — nothing to say
			lines = append(lines, m.realmImported(sel)...)
		}
		// styled spans must not cross line breaks: lipgloss pads every
		// line of a multi-line Render to the block width, which would
		// push the inline key hint to the right
		lines = append(lines,
			title.Render("export the character:"),
			dimStyle.Render(" 1. open your character's armory page in the browser"),
			dimStyle.Render("    (it passes the site's bot check)"))
		if m.impName.Value() != "" {
			lines = append(lines, dimStyle.Render("    "+sel.server.PageURL(sel.realm, m.impName.Value())))
		} else {
			lines = append(lines, dimStyle.Render("    (")+key.Render("n")+
				dimStyle.Render(" type the name to get the exact link)"))
		}
		lines = append(lines,
			dimStyle.Render(" 2. press ")+key.Render("y")+dimStyle.Render(" here to copy the export snippet"),
			dimStyle.Render(" 3. open the browser devtools console (F12 / cmd+opt+J),"),
			dimStyle.Render("    paste the snippet, press enter — the export JSON"),
			dimStyle.Render("    is placed on your clipboard"),
			dimStyle.Render(" 4. come back here and paste (cmd/ctrl+v)"))
		content = strings.Join(lines, "\n")

	default:
		validity := okStyle.Render("✓ valid JSON")
		if !json.Valid(m.pasted) {
			validity = errStyle.Render("✗ not valid JSON")
		}
		content = fmt.Sprintf("%d bytes pasted  %s\n\n%s",
			len(m.pasted), validity, dimStyle.Render(preview(m.pasted, 3, 60)))
	}

	if m.impStep == 1 {
		fetch := m.importSelection().server.Import == game.ImportFetch
		// import goes BACK to the realm list; refresh has no realm list
		back := key.Render("esc") + helpDesc.Render(" back")
		nameKey := key.Render("n") + helpDesc.Render(" name  ")
		if m.impRefresh {
			back = key.Render("esc") + helpDesc.Render(" cancel")
			nameKey = ""
		}
		switch {
		case m.impFetching:
			footer = key.Render(m.spin.View()) + helpDesc.Render("fetching  ") + back
		case m.impName.Focused() && fetch:
			footer = key.Render("enter") + helpDesc.Render(" done typing")
		case m.impName.Focused():
			footer = key.Render("enter") + helpDesc.Render(" done typing  ") +
				key.Render("paste") + helpDesc.Render(" export")
		case fetch:
			footer = key.Render("enter") + helpDesc.Render(" fetch  ") + nameKey + back
		case len(m.pasted) > 0:
			footer = key.Render("enter") + helpDesc.Render(" save  ") +
				key.Render("c") + helpDesc.Render(" clear paste  ") + back
		default:
			footer = nameKey +
				key.Render("y") + helpDesc.Render(" copy browser snippet  ") + back
		}
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
