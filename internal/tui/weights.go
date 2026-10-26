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

// weightRow is one editable line of the weights editor. Unified ratings
// (hit/crit/haste, attack power) are single rows; edits write to the side
// the profile actually reads — see resolveSide.
type weightRow struct {
	key   string    // stable config key (overrides are persisted under it)
	label string    // display text, free to change
	sides []db.Stat // empty for the profile-level weapon DPS rows
}

// weightRows is the union of every version's editable rows; the editor
// shows the subset whose stats the active version's data can carry
// (see Model.weightRows).
var weightRows = []weightRow{
	{key: "strength", label: "Strength", sides: []db.Stat{db.Strength}},
	{key: "agility", label: "Agility", sides: []db.Stat{db.Agility}},
	{key: "stamina", label: "Stamina", sides: []db.Stat{db.Stamina}},
	{key: "intellect", label: "Intellect", sides: []db.Stat{db.Intellect}},
	{key: "spirit", label: "Spirit", sides: []db.Stat{db.Spirit}},
	{key: "attack_power", label: "Attack Power", sides: []db.Stat{db.AttackPower, db.RangedAttackPower}},
	{key: "spell_power", label: "Spell Power", sides: []db.Stat{db.SpellPower}},
	{key: "melee_dps", label: "Melee DPS"},
	{key: "ranged_dps", label: "Ranged DPS"},
	{key: "hit", label: "Hit", sides: []db.Stat{db.MeleeHit, db.SpellHit}},
	{key: "crit", label: "Crit", sides: []db.Stat{db.MeleeCrit, db.SpellCrit}},
	{key: "haste", label: "Haste", sides: []db.Stat{db.MeleeHaste, db.SpellHaste}},
	{key: "mastery", label: "Mastery", sides: []db.Stat{db.Mastery}},
	{key: "armor_penetration", label: "Armor Penetration", sides: []db.Stat{db.ArmorPenetration}},
	{key: "expertise", label: "Expertise", sides: []db.Stat{db.Expertise}},
	{key: "mp5", label: "Mana Regen (MP5)", sides: []db.Stat{db.MP5}},
	{key: "defense", label: "Defense", sides: []db.Stat{db.Defense}},
	{key: "dodge", label: "Dodge", sides: []db.Stat{db.Dodge}},
	{key: "parry", label: "Parry", sides: []db.Stat{db.Parry}},
	{key: "block_value", label: "Block Value", sides: []db.Stat{db.BlockValue}},
	{key: "armor", label: "Armor", sides: []db.Stat{db.Armor}},
}

// weightRows filters the editor rows to the stats the active game
// version carries; the profile-level weapon-DPS rows always show.
func (m Model) weightRows() []weightRow {
	rows := make([]weightRow, 0, len(weightRows))
	for _, r := range weightRows {
		if len(r.sides) == 0 || m.game.Rules.HasStat(r.sides...) {
			rows = append(rows, r)
		}
	}
	return rows
}

// resolveSide picks the stat slot a row reads/writes for a profile: the
// side the profile's defaults weight, falling back to the first listed.
// Writing one side only preserves the twin-stat discipline (items carry
// duplicated values; weighting both sides would double count).
func resolveSide(base *score.Profile, row weightRow) db.Stat {
	for _, s := range row.sides {
		if base.Weights[s] != 0 {
			return s
		}
	}
	return row.sides[0]
}

// baseProfile returns the unmodified registry profile for the character.
func (m Model) baseProfile() *score.Profile {
	if m.char == nil || m.game == nil {
		return nil
	}
	return m.game.FindProfile(m.char.Class, m.char.Spec(), m.char.TalentRanks)
}

// profileRole is the active profile's role ("" without one).
func (m Model) profileRole() string {
	if p := m.baseProfile(); p != nil {
		return p.Role
	}
	return ""
}

// defaultWeight returns a row's profile default: the stat weight, or
// the profile-level DPS-per-point weights for the weapon rows.
func defaultWeight(base *score.Profile, row weightRow) float64 {
	switch row.key {
	case "melee_dps":
		return base.WeaponDPSWeight
	case "ranged_dps":
		return base.RangedDPSWeight
	}
	return base.Weights[resolveSide(base, row)]
}

// weightValue returns the effective weight for a row: the user override
// if present, the profile default otherwise.
func (m Model) weightValue(base *score.Profile, row weightRow) (val float64, overridden bool) {
	if ov, ok := m.cfg.Weights[base.Key()][row.key]; ok {
		return ov, true
	}
	return defaultWeight(base, row), false
}

// setWeight stores an override, or removes it when the value returns to
// the profile default.
func (m *Model) setWeight(base *score.Profile, row weightRow, v float64) {
	v = math.Round(v*100) / 100
	def := defaultWeight(base, row)
	if m.cfg.Weights == nil {
		m.cfg.Weights = map[string]map[string]float64{}
	}
	if m.cfg.Weights[base.Key()] == nil {
		m.cfg.Weights[base.Key()] = map[string]float64{}
	}
	if v == def {
		delete(m.cfg.Weights[base.Key()], row.key)
	} else {
		m.cfg.Weights[base.Key()][row.key] = v
	}
}

func (m Model) updateWeights(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	base := m.baseProfile()
	if base == nil {
		m.mode = modeNormal
		return m, nil
	}
	switch msg.String() {
	case "esc", "enter", "w":
		if err := m.cfg.Save(); err != nil {
			m.status = "saving weights failed: " + err.Error()
		}
		m.mode = modeNormal
		return m, nil
	case "ctrl+c":
		return m, tea.Quit
	case "up", "k":
		if m.wCursor > 0 {
			m.wCursor--
		}
		return m, nil
	case "down", "j":
		if m.wCursor < len(m.weightRows())-1 {
			m.wCursor++
		}
		return m, nil
	case "left", "right", "shift+left", "shift+right":
		row := m.weightRows()[m.wCursor]
		v, _ := m.weightValue(base, row)
		step := 0.05
		if strings.HasPrefix(msg.String(), "shift+") {
			step = 0.5
		}
		if strings.HasSuffix(msg.String(), "left") {
			step = -step
		}
		m.setWeight(base, row, math.Max(v+step, 0))
		m.rebuildScorer() // live: gems, caps and scores react immediately
		return m, nil
	case "r":
		delete(m.cfg.Weights[base.Key()], m.weightRows()[m.wCursor].key)
		m.rebuildScorer()
		return m, nil
	case "d":
		delete(m.cfg.Weights, base.Key())
		m.rebuildScorer()
		return m, nil
	}
	return m, nil
}

func (m Model) viewWeights() string {
	base := m.baseProfile()
	header := titleStyle.Render("Stat weights — "+base.Name) +
		dimStyle.Render("  (per stat point, in the spec's sim-native scale)")

	var lines []string
	for i, row := range m.weightRows() {
		v, overridden := m.weightValue(base, row)
		prefix := "  "
		if i == m.wCursor {
			prefix = helpKey.Render("▸ ")
		}
		val := fmt.Sprintf("%6.2f", v)
		switch {
		case overridden:
			val = warnStyle.Render(val) +
				dimStyle.Render(fmt.Sprintf("  (default %.2f)", defaultWeight(base, row)))
		case v == 0:
			val = dimStyle.Render(val)
		default:
			val = okStyle.Render(val)
		}
		lines = append(lines, fmt.Sprintf("%s%-19s %s", prefix, row.label, val))
	}

	var notes []string
	if m.scorer != nil {
		notes = append(notes, dimStyle.Render("set score with these weights: ")+
			titleStyle.Render(fmt.Sprintf("%.0f", m.scorer.SetScoreWorn(wornOf(m.gearRows())))))
	}
	notes = append(notes, dimStyle.Render("overrides are saved per spec and marked yellow"))

	footer := helpLine("◂/▸", "±0.05", "shift+◂/▸", "±0.5", "r", "reset row", "d", "defaults", "esc", "done")
	modal := modalStyle.Render(lipgloss.JoinVertical(lipgloss.Left,
		header, "", strings.Join(lines, "\n"), "", strings.Join(notes, "\n"), "", footer))
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, modal)
}
