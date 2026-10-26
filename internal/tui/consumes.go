package tui

import (
	"strings"

	"github.com/vaclav-dvorak/twolk/internal/db"
	"github.com/vaclav-dvorak/twolk/internal/score"
)

// Consumables are ASSUMED, never configured: food and flask cost gold
// rather than gear, so a planner that made the user declare them would
// only be collecting a setting it can derive. The planner takes the
// best pair for the set it is scoring — the worn set for the gear
// screen, the SOLVED set on the BiS screen, which can want a different
// meal — and names it wherever those numbers are shown, because the
// totals carry it: a caps readout that says 240 hit when the gear has
// 200 has to say where the other 40 came from. See score/consumes.go
// for how the baseline enters scoring.

// applyConsumes picks the best food and flask for a set and sets them
// as the scorer's non-gear baseline. Every scoring path reads the
// baseline through the stat totals, so this is the only wiring needed.
func (m Model) applyConsumes(s *score.Scorer, set []*db.Item) (food, flask *db.Consumable) {
	if s == nil {
		return nil, nil
	}
	food, flask = s.BestConsumables(set)
	var base db.Stats
	if food != nil {
		base = addStats(base, food.Stats)
	}
	if flask != nil {
		base = addStats(base, flask.Stats)
	}
	s.SetBaseline(base)
	return food, flask
}

// applyWornConsumes is applyConsumes for the EQUIPPED set, priced on
// its real gems and enchants — the meal has to answer "what should this
// character eat tonight", which is a question about his actual stats.
func (m Model) applyWornConsumes(s *score.Scorer, ws []score.Worn) (food, flask *db.Consumable) {
	if s == nil {
		return nil, nil
	}
	food, flask = s.BestConsumablesForWorn(ws)
	var base db.Stats
	if food != nil {
		base = addStats(base, food.Stats)
	}
	if flask != nil {
		base = addStats(base, flask.Stats)
	}
	s.SetBaseline(base)
	return food, flask
}

// consumesLine names the assumed buffs for display, "" when the version
// has no consumables table (nothing is assumed, nothing to disclose).
func consumesLine(food, flask *db.Consumable) string {
	var names []string
	for _, c := range []*db.Consumable{food, flask} {
		if c != nil {
			names = append(names, c.Name)
		}
	}
	if len(names) == 0 {
		return ""
	}
	return dimStyle.Render("buffs: " + strings.Join(names, " · "))
}
