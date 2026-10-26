package tui

import (
	"regexp"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/vaclav-dvorak/twolk/internal/armory"
	"github.com/vaclav-dvorak/twolk/internal/db"
	"github.com/vaclav-dvorak/twolk/internal/game"
)

// A fishing pole (or a tournament lance) left in the main hand at
// logout is not in the item database, so the weapon row vanished from
// the gear list without a trace and the set scored as if the slot were
// bare — 883 hit against a 961 cap for a character the game showed at
// 968, with the advisor offering to reforge for the difference. The
// planner must refuse to score that set instead.
func TestFishingPoleDisablesScoring(t *testing.T) {
	m := cataModel(t, withWeapon(hitCappedExport, `{"item":6256,"slot":"Weapon"}`), 3)

	if m.scorer != nil {
		t.Fatal("an unscorable weapon must disable scoring")
	}
	if len(m.gaps) != 1 || m.gaps[0].slot != db.SlotWeapon || m.gaps[0].id != 6256 {
		t.Fatalf("gaps = %+v, want the weapon slot with item 6256", m.gaps)
	}
	if !m.gapWarn {
		t.Fatal("the warning modal must be armed")
	}

	view := ansi.Strip(m.View())
	for _, want := range []string{"cannot score", "Weapon", "item 6256", "Scoring is off"} {
		if !strings.Contains(view, want) {
			t.Errorf("warning modal missing %q:\n%s", want, view)
		}
	}

	// any key dismisses it; the gear screen then keeps the reason on
	// the header and shows which slot went unscored
	m = drive(t, m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.gapWarn {
		t.Fatal("a key must dismiss the warning")
	}
	view = ansi.Strip(m.View())
	if !strings.Contains(view, "scoring disabled — unscorable Weapon") {
		t.Errorf("header does not explain the missing score:\n%s", view)
	}
	if !strings.Contains(view, "unscored — Fishing Pole") {
		t.Errorf("gear list does not name the unscored weapon:\n%s", view)
	}
	// nothing that would quote a number from the broken set is reachable
	for _, k := range []string{"o", "b", "p"} {
		if next := keys(t, m, k); next.mode != modeNormal {
			t.Errorf("%q opened an advisor for an unscorable set", k)
		}
	}
}

// The same export with a real weapon scores as before: the check must
// not fire on a character whose gear the database knows.
func TestFullKitScoresNormally(t *testing.T) {
	m := cataModel(t, hitCappedExport, 3)
	if len(m.gaps) != 0 || m.gapWarn {
		t.Fatalf("a resolvable set must not warn: %+v", m.gaps)
	}
	if m.scorer == nil {
		t.Fatal("scoring disabled for a fully resolvable set")
	}
}

// Shirts and tabards are unresolvable on EVERY character — the item
// database carries no cosmetics — so their presence alone proves
// nothing about the gear.
func TestCosmeticsAreNotGaps(t *testing.T) {
	// a worn tabard, filed under no slot, exactly as a fetch import
	// delivers it
	export := strings.Replace(hitCappedExport,
		`{"item":56366,"slot":"Ranged"}`,
		`{"item":56366,"slot":"Ranged"},{"item":65909,"ench":0,"gems":[0,0,0]}`, 1)
	m := cataModel(t, export, 3)
	if len(m.gaps) != 0 {
		t.Fatalf("a cosmetic must not read as a gap: %+v", m.gaps)
	}
	if m.scorer == nil {
		t.Fatal("scoring disabled by a tabard")
	}
}

// A plain paste claims no slots at all, so the unscorable main hand
// arrives unattributed: a complete kit plus unknown gear plus no
// weapon is the reading that still holds.
func TestPlainPasteInfersTheWeaponSlot(t *testing.T) {
	export := stripSlots(withWeapon(hitCappedExport, `{"item":6256}`))
	m := cataModel(t, export, 3)
	if len(m.gaps) != 1 || m.gaps[0].slot != db.SlotWeapon {
		t.Fatalf("gaps = %+v, want the weapon slot", m.gaps)
	}
	if m.scorer != nil {
		t.Fatal("scoring must be disabled")
	}
}

// That inference needs the rest of the kit intact: a character still
// gearing up has empty slots for ordinary reasons, and the gear list's
// "unequipped" rows already say so. (Sub-floor gear only — a known
// tool or prop is a gap on sight, no inference involved.)
func TestPartialKitDoesNotInferAWeapon(t *testing.T) {
	export := `{"name":"Zulja","realm":"Gilneas","level":85,"faction":"Horde",
		"race":"Orc","class":"Hunter","origin":"whitemane",
		"equipment":[{"item":63459},{"item":99999}],
		"talents":[{"tree":"Survival","points":[]}],
		"professions":[{"name":"Skinning","skill":525}]}`
	m := cataModel(t, export, 3)
	if len(m.gaps) != 0 {
		t.Fatalf("a partial kit must not infer a weapon gap: %+v", m.gaps)
	}
	if m.scorer == nil {
		t.Fatal("a partial kit still scores")
	}
}

// The modal opens once per problem: a weight edit rebuilds the scorer
// and must not raise it again.
func TestWarningOpensOncePerProblem(t *testing.T) {
	m := cataModel(t, withWeapon(hitCappedExport, `{"item":6256,"slot":"Weapon"}`), 3)
	m = drive(t, m, tea.KeyMsg{Type: tea.KeyEsc})
	m.rebuildScorer()
	if m.gapWarn {
		t.Error("a rebuild re-raised a dismissed warning")
	}
}

// withWeapon swaps the export's weapon line for another one.
func withWeapon(export, weapon string) string {
	out := strings.Replace(export, `{"item":65168,"slot":"Weapon"}`, weapon, 1)
	if out == export {
		panic("weapon line not found in the export")
	}
	return out
}

// stripSlots turns a fetch import into the shape of a plain paste: item
// ids and nothing else about where they are worn.
func stripSlots(export string) string {
	return regexp.MustCompile(`,"slot":"[A-Za-z]+"`).ReplaceAllString(export, "")
}

// The item pool carries a few of the tools (the Northrend fishing
// poles are real weapons with real damage), so resolving is not the
// test: db.NonGear decides, and the row the list already shows for the
// item is not doubled by a ghost.
func TestResolvableToolIsStillAGap(t *testing.T) {
	d, err := db.Load()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := d.Items[45992]; !ok {
		t.Skip("the wotlk pool no longer carries the Jeweled Fishing Pole")
	}
	m := Model{
		game: &game.Game{DB: d},
		char: &armory.Character{Equipment: []armory.EquippedItem{{ItemID: 45992}}},
	}
	gaps := m.gearGaps()
	if len(gaps) != 1 {
		t.Fatalf("gaps = %+v, want one", gaps)
	}
	g := gaps[0]
	if !g.junk || g.name != "Jeweled Fishing Pole" || g.slot != db.SlotWeapon {
		t.Fatalf("gap = %+v, want the named fishing pole in the weapon slot", g)
	}
	if g.what() != "Jeweled Fishing Pole (item 45992)" || g.note() != "Jeweled Fishing Pole" {
		t.Errorf("wording = %q / %q", g.what(), g.note())
	}
}

// The armor side of the same taxonomy: the Chef's Hat occupies the
// head slot and carries nothing, and cooking in it is as easy to log
// out in as fishing.
func TestChefsHatDisablesScoring(t *testing.T) {
	export := strings.Replace(hitCappedExport,
		`{"item":63459,"ench":4209,"gems":[68778,52209,0],"slot":"Head","reforge":{"source":49,"target":32,"value":80}}`,
		`{"item":46349,"slot":"Head"}`, 1)
	if export == hitCappedExport {
		t.Fatal("head line not found in the export")
	}
	m := cataModel(t, export, 3)
	if m.scorer != nil {
		t.Fatal("a costume in the head slot must disable scoring")
	}
	if len(m.gaps) != 1 || m.gaps[0].slot != db.SlotHead || m.gaps[0].name != "Chef's Hat" {
		t.Fatalf("gaps = %+v, want the named Chef's Hat in the head slot", m.gaps)
	}
	view := ansi.Strip(m.View())
	if !strings.Contains(view, "Head") || !strings.Contains(view, "Chef's Hat (item 46349)") {
		t.Errorf("warning does not name the hat:\n%s", view)
	}
	m = drive(t, m, tea.KeyMsg{Type: tea.KeyEsc})
	if view = ansi.Strip(m.View()); !strings.Contains(view, "unscored — Chef's Hat") {
		t.Errorf("gear list does not name the unscored head slot:\n%s", view)
	}
}
