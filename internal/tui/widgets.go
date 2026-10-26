package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/vaclav-dvorak/twolk/internal/db"
	"github.com/vaclav-dvorak/twolk/internal/score"
)

// Shared building blocks of the full-screen views: the pinned help
// footer, the list + info-pane composition and the row/cell helpers
// the list screens have in common. Screen-specific rendering stays in
// the screens' own files; anything two screens repeat lands here.

// paneCols is the column layout every bordered overview pane shares
// (the compare pane derives its own widths from its header segments).
var paneCols = cmpCols{labelW: 18, lw: 12, gw: 8, epW: 9, rw: 12, hideSame: true}

// pinFooter pins the help line to the terminal's bottom row under body.
func (m Model) pinFooter(body, help string) string {
	gap := max(m.height-lipgloss.Height(body)-1, 1)
	return body + lipgloss.PlaceVertical(gap, lipgloss.Bottom, help)
}

// twoPane joins a left column of lines with a rendered right pane: the
// left takes everything the pane leaves free (minus the scrollbar
// column) and its lines truncate to that width.
func (m Model) twoPane(left []string, right string) string {
	leftW := m.width - lipgloss.Width(right) - 3
	for i := range left {
		left[i] = truncate(left[i], leftW)
	}
	return lipgloss.JoinHorizontal(lipgloss.Top,
		lipgloss.NewStyle().Width(leftW).Render(strings.Join(left, "\n")), "  ", right)
}

// listWithBar pins the shared scrollbar to a windowed cursor list
// (rows already built one column short): total rows, shown on screen,
// first visible index.
func (m Model) listWithBar(list string, total, shown, start int) string {
	return lipgloss.JoinHorizontal(lipgloss.Top,
		lipgloss.NewStyle().Width(m.width-1).Render(list),
		scrollbar(total, shown, start))
}

// rowStyler is the cursor-row highlighter: on the selected row every
// styled segment carries the selection background (a background bar
// instead of a cursor glyph).
func rowStyler(selected bool) func(lipgloss.Style) lipgloss.Style {
	if !selected {
		return func(s lipgloss.Style) lipgloss.Style { return s }
	}
	return func(s lipgloss.Style) lipgloss.Style { return s.Background(selBgColor) }
}

// displayName is an item's list name; heroic pieces carry the [H] tag.
func displayName(it *db.Item) string {
	if it.Heroic {
		return it.Name + " [H]"
	}
	return it.Name
}

// heroicCell renders a plain composed name for a list cell: the given
// (quality) style for the text with the heroic "[H]" tag in green
// wherever it sits — the gear screen's convention, shared by every
// list. w > 0 truncates and pads to that many cells first; sel carries
// the cursor-row styling.
func heroicCell(sel func(lipgloss.Style) lipgloss.Style, style lipgloss.Style, text string, w int) string {
	if w > 0 {
		text = fmt.Sprintf("%-*s", w, truncate(text, w))
	}
	if i := strings.Index(text, "[H]"); i >= 0 {
		return sel(style).Render(text[:i]) + sel(okStyle).Render("[H]") + sel(style).Render(text[i+3:])
	}
	return sel(style).Render(text)
}

// itemLabel renders an item inline exactly as the gear list does: the
// name in its quality color, a green " [H]" on heroic pieces, then the
// tier tag in the tier color (" T12"). Every non-column mention of an
// item (quick tips, compare, item card) goes through here so an item
// reads the same everywhere; table screens give the tier its own
// column in the same style.
func (m Model) itemLabel(it *db.Item, sel func(lipgloss.Style) lipgloss.Style) string {
	s := sel(qualityStyles[it.Quality]).Render(it.Name)
	if it.Heroic {
		s += sel(okStyle).Render(" [H]")
	}
	if t := m.tierLabel(it); t != "" {
		s += sel(tierStyle).Render(" " + t)
	}
	return s
}

// scrollInfoPane applies one pane-scroll key to an info-pane offset,
// clamped to the window the current layout gives the pane.
func (m Model) scrollInfoPane(key string, off, listWant int, pane string) int {
	total := lipgloss.Height(pane)
	_, paneVis := m.listPaneSplit(lipgloss.Height(m.viewHeader()), listWant, total)
	step, _ := paneScrollStep(key, paneVis)
	return clampOff(off+step, total, paneVis)
}

// viewCaps summarizes gear totals against the profile's stat caps, e.g.
// "Hit 241/263 · ArP 266/1400". Over-cap totals are highlighted: those
// points are wasted.
func (m Model) viewCaps(set []score.Worn) string {
	return m.viewCapsTotals(m.scorer.TotalsWorn(set))
}

// viewCapsTotals is viewCaps on stat totals a caller computed itself
// (the BiS pane prices its target set on the assumed reforge plan, not
// on worn instances).
func (m Model) viewCapsTotals(tot db.Stats) string {
	var parts []string
	shortLabels := map[db.Stat]string{db.ArmorPenetration: "ArP"}
	tot = m.scorer.Derived(tot) // a balance druid's hit includes converted spirit
	shown := map[db.Stat]bool{}
	for _, c := range m.scorer.Profile().Caps {
		if shown[c.Stat] {
			continue // stepped caps: the row names the first threshold
		}
		shown[c.Stat] = true
		v := tot[c.Stat]
		label := c.Stat.String()
		for _, p := range ratingPairs { // unified ratings: "Hit", not "Melee Hit"
			if c.Stat == p.a || c.Stat == p.b {
				label = p.name
			}
		}
		if s, ok := shortLabels[c.Stat]; ok {
			label = s
		}
		text := fmt.Sprintf("%s %.0f/%.0f", label, v, c.Cap)
		if c.Stat == db.Expertise {
			// gear carries expertise RATING; the in-game sheet shows
			// floored POINTS (8.1975 rating each, dodge cap 26)
			text += fmt.Sprintf(" (%d/%d)",
				int(v/m.game.Rules.RatingPerExpertise), int(c.Cap/m.game.Rules.RatingPerExpertise))
		}
		if c.Stat == db.SpellHit && m.raidDebuffOn() {
			// the cap is the SOFT one (raid debuff assumed); the hard
			// cap in parentheses is what to stack without Misery/IFF
			text += fmt.Sprintf(" (%.0f)", c.Cap+m.raidDebuffRating())
		}
		switch {
		// soft caps (SoftWeight > 0) never warn: surplus is devalued,
		// not wasted — reaching the cap is simply "done"
		case v > c.Cap && c.SoftWeight == 0:
			text = warnStyle.Render(text + " over")
		case v >= c.Cap:
			text = okStyle.Render(text)
		default:
			text = dimStyle.Render(text)
		}
		parts = append(parts, text)
	}
	// the totals include the assumed food/flask (see consumes.go); say
	// so, or a reader checks them against the character sheet and finds
	// 40 rating that is not on the gear. The buffs are NAMED next to
	// every pane that shows these totals.
	if m.scorer.Baseline() != (db.Stats{}) {
		parts = append(parts, dimStyle.Render("buffed"))
	}
	return strings.Join(parts, dimStyle.Render(" · "))
}

// helpLine renders a footer hint row from key/description pairs.
func helpLine(pairs ...string) string {
	var b strings.Builder
	for i := 0; i+1 < len(pairs); i += 2 {
		if i > 0 {
			b.WriteString("  ")
		}
		b.WriteString(helpKey.Render(pairs[i]))
		b.WriteString(helpDesc.Render(" " + pairs[i+1]))
	}
	return b.String()
}
