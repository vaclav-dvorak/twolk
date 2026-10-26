package tui

import (
	"fmt"
	"math"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/vaclav-dvorak/twolk/internal/db"
	"github.com/vaclav-dvorak/twolk/internal/score"
)

// The talent modal (t): cap-reducing talents, assumption fallbacks
// for plain imports and the raid-debuff toggle.

// talentMaxReachable bounds a cap talent's assumable rank by the
// armory export's per-tree point sums: the export doesn't say WHICH
// talents were taken, but a rank the tree total can't unlock is
// provably not among them.
func (m Model) talentMaxReachable(t score.CapTalent) int {
	if len(m.char.Talents) == 0 {
		return t.Max
	}
	return t.MaxReachable(m.game.Rules.TreePoints(m.char.Class, t.Tree, m.char.Talents[0].Points))
}

// talentAssumptions reports whether the talent-assumption modal has a
// point: the class has cap-affecting talents in this version AND the
// import doesn't already carry exact ranks (API-fetched cata imports
// do — facts leave nothing to assume, so the modal and its hotkey
// disappear entirely). The raid-debuff toggle keeps the modal alive
// regardless: raid composition is always the user's call.
func (m Model) talentAssumptions() bool {
	if m.char == nil {
		return false
	}
	if m.raidDebuffApplies() {
		return true
	}
	if m.char.TalentRanks != nil {
		return false
	}
	return len(m.game.Rules.CapTalentsFor(m.char.Class)) > 0
}

// talentPoints returns the assumed points in a cap talent from the
// per-talent config, clamped to what the character can reach. The
// clamp is applied at read time on purpose: the config key is shared
// by every character of the class, so a stored value may be fine for
// one character and unreachable for another.
func (m Model) talentPoints(t score.CapTalent) int {
	if m.char == nil {
		return 0
	}
	// API-fetched imports carry the actual talents: facts beat
	// assumptions, and the modal shows them read-only
	if r := t.DetectRank(m.char.TalentRanks); r >= 0 {
		return r
	}
	if v, ok := m.cfg.TalentPoints[m.game.Slug+"/"+m.char.Class.Slug()+"/"+score.Slug(t.Name)]; ok {
		return min(max(v, 0), m.talentMaxReachable(t))
	}
	return 0
}

// raidDebuffApplies reports whether the raid-debuff toggle exists for
// the current character: the version has the spell-hit raid debuff
// (Misery / Improved Faerie Fire) and the build is spell-hit-capped —
// the debuff is raid composition, not character data, so it is the one
// row of the talent modal that is a user call even with exact imported
// talents.
func (m Model) raidDebuffApplies() bool {
	p := m.baseProfile()
	return p != nil && p.HitCap != nil && p.HitCap.Stat == db.SpellHit &&
		m.game.Rules.SpellHitDebuffPct > 0
}

// raidDebuffOn reports whether the user marked the raid as providing
// the spell-hit debuff (stored per game — the raid is the same across
// a game's characters).
func (m Model) raidDebuffOn() bool {
	return m.raidDebuffApplies() && m.cfg.RaidDebuff[m.game.Slug]
}

// raidDebuffRating is the debuff's gear-rating equivalent.
func (m Model) raidDebuffRating() float64 {
	return m.game.Rules.SpellHitDebuffPct * m.game.Rules.SpellHitRatingPerPct
}

// capReductions sums, per capped stat, the gear-rating equivalents the
// character gets outside gear: assumed talent points, racials (Draenei
// Heroic Presence helps melee, ranged and spell hit alike), and the
// raid's spell-hit debuff when toggled on.
func (m Model) capReductions() map[db.Stat]float64 {
	red := map[db.Stat]float64{}
	for _, t := range m.game.Rules.CapTalentsFor(m.char.Class) {
		red[t.Stat] += float64(m.talentPoints(t)) * t.RatingPerPoint
	}
	if r := score.RacialHitPct(m.char.Race); r > 0 {
		red[db.MeleeHit] += r * m.game.Rules.MeleeHitRatingPerPct
		red[db.SpellHit] += r * m.game.Rules.SpellHitRatingPerPct
	}
	if m.raidDebuffOn() {
		red[db.SpellHit] += m.raidDebuffRating()
	}
	return red
}

func (m Model) updateTalents(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	talents := m.game.Rules.CapTalentsFor(m.char.Class)
	rows := len(talents)
	if m.raidDebuffApplies() {
		rows++ // the raid-debuff toggle sits below the talents
	}
	switch msg.String() {
	case "esc", "enter", "t":
		if err := m.cfg.Save(); err != nil {
			m.status = "saving talents failed: " + err.Error()
		}
		m.rebuildScorer()
		m.mode = modeNormal
		return m, nil
	case "ctrl+c":
		return m, tea.Quit
	case "up", "k":
		if m.tCursor > 0 {
			m.tCursor--
		}
		return m, nil
	case "down", "j":
		if m.tCursor < rows-1 {
			m.tCursor++
		}
		return m, nil
	case "left", "right":
		if rows == 0 {
			return m, nil
		}
		if m.tCursor >= len(talents) {
			if m.cfg.RaidDebuff == nil {
				m.cfg.RaidDebuff = map[string]bool{}
			}
			m.cfg.RaidDebuff[m.game.Slug] = msg.String() == "right"
			return m, nil
		}
		t := talents[min(m.tCursor, len(talents)-1)]
		pts := m.talentPoints(t)
		if msg.String() == "right" {
			pts = min(pts+1, m.talentMaxReachable(t))
		} else {
			pts = max(pts-1, 0)
		}
		if m.cfg.TalentPoints == nil {
			m.cfg.TalentPoints = map[string]int{}
		}
		m.cfg.TalentPoints[m.game.Slug+"/"+m.char.Class.Slug()+"/"+score.Slug(t.Name)] = pts
		return m, nil
	}
	return m, nil
}

func (m Model) viewTalents() string {
	header := titleStyle.Render("Talent assumptions — "+m.char.ClassName) +
		dimStyle.Render("  (talents that help reach stat caps)")
	talents := m.game.Rules.CapTalentsFor(m.char.Class)
	const debuffName, debuffTree = "Misery / Imp. Faerie Fire", "raid"

	// column widths from the widest entry, so the bars align vertically
	nameW, treeW, barW := 0, 0, 1
	for _, t := range talents {
		nameW = max(nameW, len(t.Name))
		treeW = max(treeW, len(t.Tree)+2) // parentheses
		barW = max(barW, t.Max)
	}
	if m.raidDebuffApplies() {
		nameW = max(nameW, len(debuffName))
		treeW = max(treeW, len(debuffTree)+2)
	}

	var lines []string
	for i, t := range talents {
		pts := m.talentPoints(t)
		// ranks the character's tree points can't unlock render as
		// locked (·) and are not editable — the armory export doesn't
		// say which talents were taken, but it does say which can't be
		reach := m.talentMaxReachable(t)
		bar := okStyle.Render(strings.Repeat("▰", pts)) +
			dimStyle.Render(strings.Repeat("▱", reach-pts)+strings.Repeat("·", t.Max-reach)) +
			strings.Repeat(" ", barW-t.Max)
		prefix := "  "
		if i == m.tCursor {
			prefix = helpKey.Render("▸ ")
		}
		name := fmt.Sprintf("%-*s", nameW, t.Name)
		if reach == 0 {
			name = dimStyle.Render(name)
		}
		line := fmt.Sprintf("%s%s %s  %s %d/%d (+%s each)",
			prefix, name,
			dimStyle.Render(fmt.Sprintf("%-*s", treeW, "("+t.Tree+")")),
			bar, pts, t.Max, t.Per)
		if reach == 0 {
			line += dimStyle.Render("  unreachable")
		}
		lines = append(lines, line)
	}
	// the raid-debuff toggle, drawn as a one-point talent: it lowers the
	// spell hit cap the same way, but the raid provides it — the tree
	// label carries that
	if m.raidDebuffApplies() {
		on := 0
		if m.cfg.RaidDebuff[m.game.Slug] {
			on = 1
		}
		bar := okStyle.Render(strings.Repeat("▰", on)) +
			dimStyle.Render(strings.Repeat("▱", 1-on)) +
			strings.Repeat(" ", barW-1)
		prefix := "  "
		if m.tCursor == len(talents) {
			prefix = helpKey.Render("▸ ")
		}
		lines = append(lines, fmt.Sprintf("%s%-*s %s  %s %d/1 (+%g%% spell hit)",
			prefix, nameW, debuffName,
			dimStyle.Render(fmt.Sprintf("%-*s", treeW, "("+debuffTree+")")),
			bar, on, m.game.Rules.SpellHitDebuffPct))
	}
	if len(lines) == 0 {
		lines = append(lines, dimStyle.Render(m.char.ClassName+" has no cap-affecting talents"))
	}

	var notes []string
	if r := score.RacialHitPct(m.char.Race); r > 0 {
		notes = append(notes, okStyle.Render(fmt.Sprintf("+%g%% hit", r))+
			dimStyle.Render(" from "+m.char.Race+" racial (applied automatically)"))
	}
	if base := m.baseProfile(); base != nil {
		var capLine []string
		for _, c := range base.EffectiveCaps(m.capReductions()) {
			label := c.Stat.String()
			for _, p := range ratingPairs {
				if c.Stat == p.a || c.Stat == p.b {
					label = p.name
				}
			}
			text := fmt.Sprintf("%s %.0f", label, math.Ceil(c.Cap))
			if c.Stat == db.SpellHit && m.raidDebuffOn() {
				// soft cap with the raid debuff (hard cap without)
				text += fmt.Sprintf(" (%.0f)", math.Ceil(c.Cap+m.raidDebuffRating()))
			}
			capLine = append(capLine, text)
		}
		if len(capLine) > 0 {
			notes = append(notes, dimStyle.Render("effective caps: "+strings.Join(capLine, " · ")))
		}
	}

	footer := helpLine("◂/▸", "points", "esc", "done")
	modal := modalStyle.Render(lipgloss.JoinVertical(lipgloss.Left,
		header, "", strings.Join(lines, "\n"), "", strings.Join(notes, "\n"), "", footer))
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, modal)
}
