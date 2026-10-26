package tui

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// The shared top chrome: logo + class art + character line, and the
// expansion stripe under it.

// viewHeader renders the top block shared by the main screen and the
// full-screen drill-downs (upgrades, gems): ASCII logo + class emblem +
// the character/score/caps line — so a drill-down reads as the same
// application, not a new window.
func (m Model) viewHeader() string {
	c := m.char
	nameStyle := lipgloss.NewStyle().Foreground(classColor(c.Class)).Bold(true)
	id := fmt.Sprintf("%s @ %s", c.Name, c.Realm)
	detail := fmt.Sprintf("%d %s ", c.Level, c.Race)
	spec := c.ClassName
	if s := c.Spec(); s != "" {
		spec = s + " " + c.ClassName
	}
	head := nameStyle.Render(id) + "  " + dimStyle.Render(detail) + nameStyle.Render(spec)
	switch {
	case len(c.Equipment) == 0:
		// added but never refreshed: a name and a realm, nothing else
		head = nameStyle.Render(id) + "  " + dimStyle.Render("no gear yet — press ") +
			helpKey.Render("r")
	case m.scorer != nil:
		set := wornOf(m.gearRows())
		head += "  " + titleStyle.Render(fmt.Sprintf("score %.0f", m.scorer.SetScoreWorn(set)))
		// the weights profile always matches the spec shown left of
		// the score — only deviations from it are worth flagging
		if len(m.cfg.Weights[m.scorer.Profile().Key()]) > 0 {
			head += dimStyle.Render(" (custom weights)")
		}
		head += "  " + m.viewCaps(set)
		if line := consumesLine(m.consFood, m.consFlask); line != "" {
			head += "\n" + line
		}
	case len(m.gaps) > 0:
		// the reason stays on screen after the warning modal is
		// dismissed — a missing score is otherwise unexplained
		head += "  " + errStyle.Render("scoring disabled") +
			dimStyle.Render(" — unscorable "+gapSlots(m.gaps))
	case m.game.DB != nil:
		head += "  " + dimStyle.Render(fmt.Sprintf("no scoring profile for %s/%s yet", c.ClassName, c.Spec()))
	}
	art := m.classArtView()
	leftBlock := lipgloss.JoinVertical(lipgloss.Left,
		titleStyle.Render(titleArt),
		dimStyle.Render("terminal "+m.game.Name+" gear planner"),
		"",
		head)
	pad := m.width - lipgloss.Width(leftBlock) - lipgloss.Width(art) - 1
	header := lipgloss.JoinVertical(lipgloss.Left,
		titleStyle.Render("twolk")+dimStyle.Render("  ·  terminal "+m.game.Name+" gear planner"), "", head)
	if art != "" && pad >= 2 {
		// ascii-art logo left, class emblem pinned to the right edge;
		// skipped when it wouldn't fit without wrapping
		header = lipgloss.JoinHorizontal(lipgloss.Top,
			leftBlock, strings.Repeat(" ", pad), art)
	}
	// breathing room between header art and the stripe — skipped on
	// tight terminals, where every row is spoken for
	if m.height >= 30 {
		return lipgloss.JoinVertical(lipgloss.Left, header, "", m.expansionStripe())
	}
	return lipgloss.JoinVertical(lipgloss.Left, header, m.expansionStripe())
}

// expansionStripe is the one-line banner under every header: the
// expansion name centered in its accent color, with shade glyphs
// (░▒▓█) densifying toward a solid accent bar at both screen edges
// while their color deepens from a dimmed accent to the full one.
// Two blends stack: glyph density lets the terminal's own background
// show through (so the stripe meets ANY color scheme), and the
// foreground fade adds the gradient depth — floored well above black,
// the darkest steps are the terminal's job, not ours.
func (m Model) expansionStripe() string {
	name := "  " + m.game.Name + "  "
	nameW := lipgloss.Width(name)
	center := lipgloss.NewStyle().Bold(true).
		Foreground(lipgloss.Color(m.game.Accent)).Render(name)
	left := (m.width - nameW) / 2
	right := m.width - left - nameW // absorbs the odd-width remainder
	if left < 1 || right < 1 {
		return center
	}
	shades := []rune{' ', '░', '▒', '▓', '█'}
	const colorSteps = 8
	fade := func(width int, edgeLeft bool) string {
		var out, run strings.Builder
		prev := ""
		flush := func() {
			if run.Len() > 0 {
				out.WriteString(lipgloss.NewStyle().
					Foreground(lipgloss.Color(prev)).Render(run.String()))
				run.Reset()
			}
		}
		for i := 0; i < width; i++ {
			d := float64(i+1) / float64(width) // distance from the name
			if edgeLeft {
				d = float64(width-i) / float64(width)
			}
			q := (math.Floor(d*colorSteps) + 1) / colorSteps
			col := fadedAccent(m.game.Accent, 0.35+0.65*q)
			if col != prev {
				flush()
				prev = col
			}
			run.WriteRune(shades[min(int(d*float64(len(shades))), len(shades)-1)])
		}
		flush()
		return out.String()
	}
	return fade(left, true) + center + fade(right, false)
}

// fadedAccent scales a #RRGGBB accent toward black: strength 1 is the
// full accent, 0 black.
func fadedAccent(hex string, strength float64) string {
	if len(hex) != 7 || hex[0] != '#' {
		return hex
	}
	v, err := strconv.ParseUint(hex[1:], 16, 32)
	if err != nil {
		return hex
	}
	s := min(max(strength, 0), 1)
	r := uint32(float64(v>>16&0xff) * s)
	g := uint32(float64(v>>8&0xff) * s)
	b := uint32(float64(v&0xff) * s)
	return fmt.Sprintf("#%02X%02X%02X", r, g, b)
}
