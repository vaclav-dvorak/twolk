package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"github.com/vaclav-dvorak/twolk/internal/db"
	"github.com/vaclav-dvorak/twolk/internal/score"
)

// Stat, effect, gem and enchant text shared by the detail panes.

// ratingPairs are stats the game presents as a single tooltip line but the
// upstream data carries per side: the 3.0-unified hit/crit/haste ratings
// (spell + melee twins) and attack power ("+X Attack Power" grants melee
// and ranged AP alike). Values are duplicated on virtually everything; the
// rare genuinely one-sided cases (a few enchants, ranged-AP-only weapons)
// keep their distinct labels.
var ratingPairs = []struct {
	a, b db.Stat
	name string
}{
	{db.SpellHit, db.MeleeHit, "Hit"},
	{db.SpellCrit, db.MeleeCrit, "Crit"},
	{db.SpellHaste, db.MeleeHaste, "Haste"},
	{db.AttackPower, db.RangedAttackPower, "Attack Power"},
}

// baseStats are the "white" tooltip stats the game lists at the top of an
// item; everything else renders as a green "Equip:" effect below them.
var baseStats = map[db.Stat]bool{
	db.Strength: true, db.Agility: true, db.Stamina: true,
	db.Intellect: true, db.Spirit: true, db.Armor: true,
	db.Health: true, db.Mana: true,
	db.ArcaneResistance: true, db.FireResistance: true,
	db.FrostResistance: true, db.NatureResistance: true,
	db.ShadowResistance: true,
}

// statGroups splits the stat lines the way the game tooltip does: base
// attributes first, secondary (equip-effect) stats as a second group.
// Groups without content are omitted.
func (m Model) statGroups(s db.Stats) [][]string {
	var groups [][]string
	for _, wantBase := range []bool{true, false} {
		want := wantBase
		g := m.statLines(s, func(st db.Stat) bool { return baseStats[st] == want })
		if len(g) > 0 {
			groups = append(groups, g)
		}
	}
	return groups
}

// statLabel names a single stat the way the game version shows it: a
// unified rating whose twin the version's data never sets renders as
// the plain name (cata "Hit", never "Melee Hit").
func (m Model) statLabel(st db.Stat) string {
	for _, p := range ratingPairs {
		if (st == p.a || st == p.b) && (!m.game.Rules.HasStat(p.a) || !m.game.Rules.HasStat(p.b)) {
			return p.name
		}
	}
	return st.String()
}

// statLines lists the non-zero stats of s in enum order, collapsing
// unified-rating pairs into the single line the game shows. include
// filters which stats to list; nil includes everything.
func (m Model) statLines(s db.Stats, include func(db.Stat) bool) []string {
	labels := map[db.Stat]string{} // "" marks the twin as consumed
	for _, p := range ratingPairs {
		switch {
		case s[p.a] != 0 && s[p.a] == s[p.b]:
			labels[p.a] = p.name
			labels[p.b] = ""
		// a version without the twin stat has nothing to distinguish:
		// the single side IS the unified rating (cata "Hit", never
		// "Melee Hit")
		case !m.game.Rules.HasStat(p.a):
			labels[p.b] = p.name
		case !m.game.Rules.HasStat(p.b):
			labels[p.a] = p.name
		}
	}
	var out []string
	for st := db.Stat(0); st < db.NumStats; st++ {
		v := s[st]
		if v == 0 || (include != nil && !include(st)) {
			continue
		}
		label, merged := labels[st]
		if merged && label == "" {
			continue
		}
		if !merged {
			label = st.String()
		}
		out = append(out, fmt.Sprintf("%+.0f %s", v, label))
	}
	return out
}

// effectLines renders one item effect as a headline plus a dim detail line.
func (m Model) effectLines(e *db.Effect) []string {
	bonus := strings.Join(m.statLines(e.Bonus, nil), ", ")
	switch e.Kind {
	case db.EffectOnUse:
		return []string{
			okStyle.Render("Use: ") + bonus,
			dimStyle.Render(fmt.Sprintf("  %s, %s cooldown", fmtDur(e.Duration), fmtDur(e.Cooldown))),
		}
	case db.EffectProc:
		var cond []string
		if e.ProcChance > 0 {
			cond = append(cond, fmt.Sprintf("%g%% chance", e.ProcChance*100))
		}
		if e.PPM > 0 {
			cond = append(cond, fmt.Sprintf("%g PPM", e.PPM))
		}
		if lbl := score.TriggerLabel(e); lbl != "" {
			cond = append(cond, "from "+lbl)
		}
		if strings.Contains(e.Outcome, "Crit") {
			cond = append(cond, "on crit")
		}
		if e.Cooldown > 0 {
			cond = append(cond, fmtDur(e.Cooldown)+" ICD")
		}
		detail := dimStyle.Render("  " + strings.Join(cond, ", "))
		if m.scorer != nil && !m.scorer.Profile().CanTrigger(e) {
			detail += errStyle.Render(" — unreachable")
		}
		return []string{
			okStyle.Render("Proc: ") + bonus + dimStyle.Render(" for "+fmtDur(e.Duration)),
			detail,
		}
	case db.EffectCurated:
		return []string{
			okStyle.Render("Proc (avg value): ") + bonus,
			dimStyle.Render("  curated estimate, not passive stats"),
		}
	}
	return nil
}

// fmtDur renders durations compactly: "50s", "1m30s", "2m" (not "2m0s").
func fmtDur(d time.Duration) string {
	s := d.String()
	if strings.HasSuffix(s, "m0s") {
		s = strings.TrimSuffix(s, "0s")
	}
	if strings.HasSuffix(s, "h0m") {
		s = strings.TrimSuffix(s, "0m")
	}
	return s
}

// gemColorStyle picks a terminal color resembling the socket color —
// muted shades on purpose: gem NAMES wear these across whole panes,
// and full-saturation entries (196/220/208) shout. Orange is also kept
// away from the cata accent so gems never read as chrome.
func gemColorStyle(c db.GemColor) lipgloss.Style {
	colors := map[db.GemColor]string{
		db.GemMeta: "246", db.GemRed: "167", db.GemBlue: "67",
		db.GemYellow: "179", db.GemGreen: "71", db.GemOrange: "172",
		db.GemPurple: "97", db.GemPrismatic: "109", db.GemCogwheel: "242",
	}
	if col, ok := colors[c]; ok {
		return lipgloss.NewStyle().Foreground(lipgloss.Color(col))
	}
	return dimStyle
}

// gemText renders a gem name bracketed and tinted with the gem's own
// color, so gems stand out in prose: "[ Delicate Inferno Ruby ]" in
// red, an Ember Topaz in orange.
func gemText(g *db.Gem) string {
	if g == nil {
		return ""
	}
	return gemColorStyle(g.Color).Render("[ " + g.Name + " ]")
}

// enchantText renders an enchant name bracketed and tinted the classic
// tooltip-enchant green: "[ Arcanum of Ramkahen ]".
func enchantText(name string) string {
	return enchantStyle.Render("[ " + name + " ]")
}
