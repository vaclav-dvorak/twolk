package tui

import (
	"fmt"
	"math"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"

	"github.com/vaclav-dvorak/twolk/internal/config"
	"github.com/vaclav-dvorak/twolk/internal/db"
	"github.com/vaclav-dvorak/twolk/internal/score"
)

// TestCataBiS opens the BiS screen on the cata survival hunter: the
// solver must fill every slot from the cata pool (hunter layouts —
// ranged plus 2H or dual-wield), and the shopping list must price
// vendor pieces in cata currencies, not WotLK emblems.
func TestCataBiS(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if err := cfg.UpsertCharacter([]byte(fetchedExport),
		config.StoredCharacter{Game: "cata", Server: "whitemane", Realm: "Gilneas"}, -1); err != nil {
		t.Fatal(err)
	}
	m := New(cfg)
	m = drive(t, m, tea.WindowSizeMsg{Width: 140, Height: 45})
	d, err := db.LoadCata()
	if err != nil {
		t.Fatal(err)
	}
	m = drive(t, m, dbLoadedMsg{slug: "cata", db: d})
	if m.scorer == nil {
		t.Fatal("no scorer — BiS needs a profile")
	}

	m = keys(t, m, "b")
	if m.mode != modeBiS {
		t.Fatal("b did not open the BiS screen")
	}
	if len(m.bisSet) == 0 {
		t.Fatal("BiS solver returned an empty set")
	}
	seen := map[int]bool{}
	groups := map[string]bool{}
	for i, it := range m.bisSet {
		if it == nil {
			t.Fatalf("BiS row %d unsolved", i)
		}
		if it.ILvl < 346 {
			t.Errorf("BiS pick %s ilvl %d — below cata endgame", it.Name, it.ILvl)
		}
		// set legality: no duplicate unique-equipped picks (DS rings and
		// trinkets are name-grouped families), no other class's legendary
		if it.Unique && seen[it.ID] {
			t.Errorf("unique %s picked twice", it.Name)
		}
		seen[it.ID] = true
		if g := d.UniqueGroup(it.ID); g != "" {
			if groups[g] {
				t.Errorf("unique family %q picked twice", g)
			}
			groups[g] = true
		}
		if !it.UsableBy(db.ClassHunter) {
			t.Errorf("BiS pick %s not hunter-usable", it.Name)
		}
	}
	view := m.View()
	for _, e := range []db.Emblem{db.EmblemFrost, db.EmblemTriumph} {
		if strings.Contains(view, e.String()) {
			t.Errorf("shopping list prices in WotLK emblem %s", e)
		}
	}
}

// dualWieldExport is the fetched shape for a hunter wearing a
// dual-wield stat-stick pair (the Twinblades of the Hakkari) — the
// displaced-weapon case: when the two-hand layout wins, neither sword
// fits the single weapon row's role and both drop out of the BiS
// baseline.
const dualWieldExport = `{"name":"Zulja","realm":"Gilneas","level":85,"faction":"Horde",
	"race":"Orc","class":"Hunter","origin":"whitemane",
	"equipment":[
		{"item":60303,"ench":4209,"gems":[68778,52209,0]},
		{"item":62385,"ench":4105,"gems":[52212,0,0]},
		{"item":69621,"ench":0,"gems":[0,0,0]},
		{"item":69620,"ench":0,"gems":[0,0,0]}],
	"talents":[{"tree":"Survival","points":[]}],
	"professions":[]}`

// cataModel builds a model with the given export loaded at the given
// cata phase, on the normal screen.
func cataModel(t *testing.T, export string, phase int) Model {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if err := cfg.UpsertCharacter([]byte(export),
		config.StoredCharacter{Game: "cata", Server: "whitemane", Realm: "Gilneas"}, -1); err != nil {
		t.Fatal(err)
	}
	m := New(cfg)
	m = drive(t, m, tea.WindowSizeMsg{Width: 140, Height: 45})
	d, err := db.LoadCata()
	if err != nil {
		t.Fatal(err)
	}
	m = drive(t, m, dbLoadedMsg{slug: "cata", db: d})
	m.filter.ServerPhase = phase
	return m
}

// bisModel is cataModel on the BiS screen.
func bisModel(t *testing.T, export string, phase int) Model {
	t.Helper()
	m := cataModel(t, export, phase)
	m = keys(t, m, "b")
	if m.mode != modeBiS {
		t.Fatal("b did not open the BiS screen")
	}
	return m
}

// reforgedExport wears a reforged two-hander (36 crit moved into
// mastery — a poor choice on purpose) plus unreforged pieces, for the
// reforge-UI pins below.
const reforgedExport = `{"name":"Zulja","realm":"Gilneas","level":85,"faction":"Horde",
	"race":"Orc","class":"Hunter","origin":"whitemane",
	"equipment":[
		{"item":60303,"ench":4209,"gems":[68778,52209,0]},
		{"item":62385,"ench":4105,"gems":[52212,0,0]},
		{"item":65168,"ench":4227,"gems":[0,0,0],"reforge":{"source":32,"target":49,"value":72}}],
	"talents":[{"tree":"Survival","points":[]}],
	"professions":[]}`

// TestCataReforgeUI pins reforge as a first-class citizen in the UI:
// the worn card shows the ACTIVE reforge, the quick-tips pane points a
// suboptimal reforge at the optimization advisor, the advisor carries
// a reforge section, and the upgrade compare shows the candidate's
// ASSUMED reforge against the worn one.
func TestCataReforgeUI(t *testing.T) {
	m := cataModel(t, reforgedExport, 2)

	// the reforged weapon's item card shows the active reforge (checked
	// on the card builder: the tiny 3-item fixture makes the on-screen
	// pane elide bottom sections that a full character keeps)
	rows := m.gearRows()
	weap := -1
	for i, r := range rows {
		if r.item.ID == 65168 {
			weap = i
		}
	}
	card := ansi.Strip(m.viewItemDetail(rows, weap))
	if !strings.Contains(card, "⚒ Reforged: 72 Crit → Mastery") {
		t.Fatalf("worn card missing the active reforge:\n%s", card)
	}
	// unreforged items with reforgeable stats say so
	if head := ansi.Strip(m.viewItemDetail(rows, 0)); !strings.Contains(head, "⚒ not reforged") {
		t.Fatalf("unreforged card missing the marker:\n%s", head)
	}
	// the mis-reforged weapon must surface in the tips pane
	if view := ansi.Strip(m.View()); !strings.Contains(view, "reforges on") ||
		!strings.Contains(view, "not optimal") {
		t.Fatalf("tips pane missing the reforge hint:\n%s", view)
	}

	m = keys(t, m, "o")
	if m.mode != modeOptimize {
		t.Fatal("o did not open the optimization advisor")
	}
	view := ansi.Strip(m.View())
	if !strings.Contains(view, "reforges") || !strings.Contains(view, "72 Crit → Mastery") {
		t.Fatalf("advisor missing the reforge section:\n%s", view)
	}
	text := m.optPlanText()
	if !strings.Contains(text, "72 Crit -> Mastery =>") {
		t.Fatalf("plan text missing the reforge change:\n%s", text)
	}
	m = drive(t, m, tea.KeyMsg{Type: tea.KeyEsc})

	// upgrade compare on the weapon row: the candidate side carries its
	// assumed reforge against the worn one
	m.cursor = weap
	m = drive(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.mode != modeUpgrades {
		t.Fatal("enter did not open the upgrade browser")
	}
	view = ansi.Strip(m.View())
	if !strings.Contains(view, "reforge: ⚒ 72 Crit → Mastery  ⇒  ") {
		t.Fatalf("upgrade compare missing the reforge row:\n%s", view)
	}
}

// trollBowExport is a Troll hunter wearing the Big Bendy Blasting Bow
// (no native crit) with a hit scope and a mastery→hit reforge — the
// racial/enchant/reforge decomposition case.
const trollBowExport = `{"name":"Zulja","realm":"Gilneas","level":85,"faction":"Horde",
	"race":"Troll","class":"Hunter","origin":"whitemane",
	"equipment":[
		{"item":60303,"ench":4209,"gems":[68778,52209,0]},
		{"item":63791,"ench":4176,"gems":[0,0,0],"slot":"Ranged",
			"reforge":{"source":49,"target":31,"value":25}}],
	"talents":[{"tree":"Survival","points":[]}],
	"professions":[]}`

// TestCataCompareDecomposed pins the compare pane's one-contribution-
// one-section rule: the stats table carries the items' OWN stats only,
// a weapon racial gets its own priced section (Troll bow crit is real
// but never on the item tooltip), the shared enchant still explains its
// stats, and the reforge section shows the full swing — the gained stat
// AND the sacrificed one.
func TestCataCompareDecomposed(t *testing.T) {
	m := cataModel(t, trollBowExport, 2)
	rows := m.gearRows()
	for i, r := range rows {
		if r.item != nil && r.item.ID == 63791 {
			m.upSlot = i
		}
	}
	cand := m.game.DB.Items[62460] // Sky Piercer — a crossbow: no Troll racial
	if cand == nil {
		t.Fatal("Sky Piercer not in the cata db")
	}
	pane := ansi.Strip(m.viewCompare(rows[m.upSlot].worn(), cand))

	// the bow has zero native crit: the racial's 179 must not leak into
	// the stats table — it lives in its own section, priced as a loss
	stats := pane[strings.Index(pane, "stats:"):strings.Index(pane, "racial:")]
	if strings.Contains(stats, "179") {
		t.Errorf("racial crit leaked into the stats table:\n%s", stats)
	}
	if !strings.Contains(pane, "racial: ★ Troll racial: +1% crit with bows → none") {
		t.Fatalf("racial section missing:\n%s", pane)
	}
	racial := pane[strings.Index(pane, "racial:"):strings.Index(pane, "enchants:")]
	if !strings.Contains(racial, "179") || !strings.Contains(racial, "-179") {
		t.Errorf("racial section must price losing the 179 crit:\n%s", racial)
	}
	// the enchant section always shows its stats; whether it prices a
	// DELTA depends on whether the assumed enchant actually changes,
	// which is a function of the weights (the measured cata hit of
	// session 38 flipped the assumed scope from the hit one to the
	// agility one). Assert the invariant, not the weights of the day.
	ench := pane[strings.Index(pane, "enchants:"):strings.Index(pane, "reforge:")]
	// the header says which case this is: one enchant, or one replaced
	// by another
	changed := strings.Contains(ench, "→")
	if !strings.ContainsAny(ench, "0123456789") {
		t.Errorf("enchant section must show its stats:\n%s", ench)
	}
	if !changed && (strings.Contains(ench, "-") || strings.Contains(ench, "+")) {
		t.Errorf("unchanged enchant must not price a delta:\n%s", ench)
	}
	if changed && !strings.Contains(ench, "+") && !strings.Contains(ench, "-") {
		t.Errorf("a changed enchant must price its delta:\n%s", ench)
	}
	// the reforge section carries both sides of the trade: +25 hit AND
	// the -25 mastery it costs
	ref := pane[strings.Index(pane, "reforge:"):]
	if !strings.Contains(ref, "Hit") || !strings.Contains(ref, "Mastery") ||
		!strings.Contains(ref, "-25") {
		t.Errorf("reforge section must show the sacrificed stat too:\n%s", ref)
	}
	// the candidate's ASSUMED reforge is the cap-aware set pick: this
	// two-item set is far under the hit cap, so the crossbow assumes
	// mastery → hit (the context-free assumption would say haste)
	if !strings.Contains(ref, "⇒  25 Mastery → Hit (assumed)") {
		t.Errorf("candidate must assume the under-cap hit reforge:\n%s", ref)
	}
}

// TestCataBiSWeaponDisplaced pins the displaced-weapon pricing: a
// hunter in a dual-wield pair whose BiS layout is a two-hander (the
// honest ordering once flat AP is not double-counted) must see the
// two-hander priced against LOSING BOTH swords — never against an
// empty hand.
func TestCataBiSWeaponDisplaced(t *testing.T) {
	m := bisModel(t, dualWieldExport, 2)
	var weap []int
	for i := range m.bisSet {
		if m.bisSet[i].Slot == db.SlotWeapon {
			weap = append(weap, i)
		}
	}
	if len(weap) != 1 {
		t.Fatalf("two-hander expected to beat the pair (no AP double-count), got %d weapon rows", len(weap))
	}
	i := weap[0]
	if m.bisWorn[i].Item.ID != 0 {
		t.Fatalf("worn pair should be displaced, baseline row holds %s", m.bisWorn[i].Item.Name)
	}
	ws := wornOf(m.gearRows())
	first, delta, rows, ok := m.bisPairDelta(ws)
	if !ok || first != i || !rows[i] {
		t.Fatalf("displaced pricing not engaged: first=%d delta=%.0f rows=%v ok=%v", first, delta, rows, ok)
	}
	// the group delta charges both displaced swords; pricing the pick
	// against an empty hand must be visibly larger
	naive := m.bisScorer.SwapDelta(m.bisWorn, i, m.bisSet[i])
	if delta >= naive-800 {
		t.Errorf("group delta %.0f does not charge the worn pair (naive empty-hand %.0f)", delta, naive)
	}
	m.bisCursor = i // scroll the weapon row into view
	view := ansi.Strip(m.View())
	want := fmt.Sprintf("%+6.0f", delta)
	if !strings.Contains(view, want) {
		t.Errorf("weapon row missing group delta %s", want)
	}

	// the acquire card discloses the reforge each pick was priced with
	reforged := -1
	for j := range m.bisSet {
		if m.bisNeeded(j) && m.bisScorer.AssumedReforge(m.bisSet[j]) != nil {
			reforged = j
			break
		}
	}
	if reforged < 0 {
		t.Fatal("no BiS pick with an assumed reforge — fixture too small?")
	}
	m.bisCursor = reforged
	if card := ansi.Strip(m.View()); !strings.Contains(card, "⚒ ") {
		t.Errorf("acquire card missing the assumed reforge:\n%s", card)
	}
	// the assumed enchant is disclosed the same way (the weapon pick is
	// always enchantable)
	m.bisCursor = i
	if card := ansi.Strip(m.View()); !strings.Contains(card, "✎ ") {
		t.Errorf("acquire card missing the assumed enchant:\n%s", card)
	}
}

// TestCataBiSWeaponPairRender pins the pair rendering: when the solved
// layout carries two needed weapon rows and the worn weapon was
// displaced, the first row shows the group delta and the second a dim
// "pair" marker. The dual-wield picks are fabricated onto the solved
// state — post-AP-fix no real hunter solve prefers the pair, but the
// layout stays legal and other builds (fury, data drift) can reach it.
func TestCataBiSWeaponPairRender(t *testing.T) {
	m := bisModel(t, dualWieldExport, 2)
	var mh, oh *db.Item
	for _, it := range m.game.DB.Items {
		switch it.ID {
		case 69621:
			mh = it
		case 69620:
			oh = it
		}
	}
	if mh == nil || oh == nil {
		t.Fatal("Twinblades not in the cata db")
	}
	weap := -1
	for i := range m.bisSet {
		if m.bisSet[i].Slot == db.SlotWeapon {
			weap = i
			break
		}
	}
	if weap < 0 {
		t.Fatal("no weapon row solved")
	}
	// fabricate a winning dual-wield shape in place of the solved row
	m.bisSet = append(m.bisSet[:weap], append([]*db.Item{mh, oh}, m.bisSet[weap+1:]...)...)
	worn := append([]score.Worn(nil), m.bisWorn[:weap]...)
	worn = append(worn, score.Worn{Item: score.EmptyHand}, score.Worn{Item: score.EmptyHand})
	m.bisWorn = append(worn, m.bisWorn[weap+1:]...)

	ws := wornOf(m.gearRows())
	first, delta, rows, ok := m.bisPairDelta(ws)
	if !ok || first != weap || !rows[weap] || !rows[weap+1] {
		t.Fatalf("pair pricing not engaged: first=%d rows=%v ok=%v", first, rows, ok)
	}
	m.bisCursor = weap // scroll the weapon rows into view
	view := ansi.Strip(m.View())
	var sawMH, sawOH bool
	for _, line := range strings.Split(view, "\n") {
		switch {
		case strings.Contains(line, "Main hand"):
			sawMH = true
			if want := fmt.Sprintf("%+6.0f", delta); !strings.Contains(line, want) {
				t.Errorf("main-hand row missing group delta %s: %q", want, line)
			}
		case strings.Contains(line, "Off hand"):
			sawOH = true
			if !strings.Contains(line, "  pair ") {
				t.Errorf("off-hand row missing pair marker: %q", line)
			}
		}
	}
	if !sawMH || !sawOH {
		t.Errorf("weapon rows not rendered (mh=%v oh=%v)", sawMH, sawOH)
	}
}

// TestCataBiSPaneScroll pins the fold layout on a squeezed terminal:
// the item list always keeps every solved row, the info panes clip to
// what is left and scroll under J/K, and the shared scrollbar plus the
// help hint mark the fold.
func TestCataBiSPaneScroll(t *testing.T) {
	m := bisModel(t, dualWieldExport, 2)
	m = drive(t, m, tea.WindowSizeMsg{Width: 140, Height: 40})

	panes, _, _ := m.bisPanes(m.gearRows())
	paneFull := lipgloss.Height(panes)
	itemVis, paneVis := m.listPaneSplit(lipgloss.Height(m.viewHeader()), len(m.bisSet), paneFull)
	if itemVis < len(m.bisSet) {
		t.Fatalf("item list gave up rows: %d visible of %d", itemVis, len(m.bisSet))
	}
	if paneFull <= paneVis {
		t.Fatalf("fixture panes fit the window (%d <= %d) — shrink the test terminal", paneFull, paneVis)
	}

	view := ansi.Strip(m.View())
	for _, it := range m.bisSet {
		if lipgloss.Width(it.Name) > 50 {
			continue // longer names truncate into the name column
		}
		if !strings.Contains(view, it.Name) {
			t.Errorf("item row %q pushed off screen", it.Name)
		}
	}
	if !strings.Contains(view, "j/k scroll info") {
		t.Errorf("help missing the pane-scroll hint:\n%s", view)
	}

	m = keys(t, m, "j")
	if m.bisPaneOff != 1 {
		t.Fatalf("j must scroll the info panes, off=%d", m.bisPaneOff)
	}
	if after := ansi.Strip(m.View()); after == view {
		t.Errorf("j did not change the view")
	}
	for range 200 {
		m = keys(t, m, "j")
	}
	if want := paneFull - paneVis; m.bisPaneOff != want {
		t.Errorf("pane offset must clamp at %d, got %d", want, m.bisPaneOff)
	}
	// the card follows the cursor: moving it rewinds the fold
	m = drive(t, m, tea.KeyMsg{Type: tea.KeyDown})
	if m.bisPaneOff != 0 {
		t.Errorf("cursor move must reset the pane scroll, off=%d", m.bisPaneOff)
	}
}

// TestCataOptimizeScroll pins the advisor's scrolling: a plan taller
// than the screen scrolls under ↑/↓ instead of truncating to a
// "screen too small" stub.
func TestCataOptimizeScroll(t *testing.T) {
	m := cataModel(t, reforgedExport, 2)
	m = drive(t, m, tea.WindowSizeMsg{Width: 140, Height: 24})
	m = keys(t, m, "o")
	if m.mode != modeOptimize {
		t.Fatal("o did not open the optimization advisor")
	}

	body := m.optBody(m.gearRows())
	vis := m.optVisible(lipgloss.Height(m.viewHeader()))
	if lipgloss.Height(body) <= vis {
		t.Fatalf("fixture plan fits the window (%d <= %d) — shrink the test terminal", lipgloss.Height(body), vis)
	}
	view := ansi.Strip(m.View())
	if !strings.Contains(view, "↑/↓ scroll") {
		t.Errorf("help missing the scroll hint:\n%s", view)
	}
	if strings.Contains(view, "screen too small") {
		t.Errorf("truncation stub must be gone:\n%s", view)
	}

	m = drive(t, m, tea.KeyMsg{Type: tea.KeyDown})
	if m.optOff != 1 {
		t.Fatalf("down must scroll the plan, off=%d", m.optOff)
	}
	if after := ansi.Strip(m.View()); after == view {
		t.Errorf("scrolling did not change the view")
	}
	for range 300 {
		m = drive(t, m, tea.KeyMsg{Type: tea.KeyDown})
	}
	if want := lipgloss.Height(body) - vis; m.optOff != want {
		t.Errorf("plan offset must clamp at %d, got %d", want, m.optOff)
	}
	m = drive(t, m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.mode != modeNormal {
		t.Fatal("esc must still close the advisor")
	}
}

// TestCataUpgradePaneScroll pins the upgrade browser's split: the
// candidate list is promised its rows, an overgrown compare pane clips
// and scrolls under j/k (arrows keep moving the cursor), and moving
// the cursor rewinds the pane.
func TestCataUpgradePaneScroll(t *testing.T) {
	m := cataModel(t, dualWieldExport, 2)
	m = drive(t, m, tea.WindowSizeMsg{Width: 140, Height: 30})
	m = drive(t, m, tea.KeyMsg{Type: tea.KeyEnter}) // upgrades for row 0
	if m.mode != modeUpgrades {
		t.Fatal("enter did not open the upgrade browser")
	}
	if len(m.upList) == 0 {
		t.Fatal("no upgrade candidates in the fixture")
	}

	pane := m.viewCompare(m.gearRows()[m.upSlot].worn(), m.upList[0].Item)
	_, paneVis := m.listPaneSplit(lipgloss.Height(m.viewHeader()), min(len(m.upList), 10), lipgloss.Height(pane))
	if lipgloss.Height(pane) <= paneVis {
		t.Fatalf("fixture pane fits the window (%d <= %d) — shrink the test terminal", lipgloss.Height(pane), paneVis)
	}
	view := ansi.Strip(m.View())
	if !strings.Contains(view, "j/k scroll info") {
		t.Errorf("help missing the pane-scroll hint:\n%s", view)
	}

	m = keys(t, m, "j")
	if m.upPaneOff != 1 {
		t.Fatalf("j must scroll the compare pane, off=%d", m.upPaneOff)
	}
	if after := ansi.Strip(m.View()); after == view {
		t.Errorf("j did not change the view")
	}
	// arrows move the cursor and rewind the pane
	m = drive(t, m, tea.KeyMsg{Type: tea.KeyDown})
	if m.upCursor != 1 || m.upPaneOff != 0 {
		t.Errorf("down must move the cursor and reset the pane: cursor=%d off=%d", m.upCursor, m.upPaneOff)
	}
	for range 200 {
		m = keys(t, m, "j")
	}
	pane = m.viewCompare(m.gearRows()[m.upSlot].worn(), m.upList[1].Item)
	_, paneVis = m.listPaneSplit(lipgloss.Height(m.viewHeader()), min(len(m.upList), 10), lipgloss.Height(pane))
	if want := lipgloss.Height(pane) - paneVis; m.upPaneOff != want {
		t.Errorf("pane offset must clamp at %d, got %d", want, m.upPaneOff)
	}
}

// TestGearListGhostSlots pins the unfilled-slot rows on the gear list:
// a worn item the db cannot resolve (leveling gear below the endgame
// pool) renders an "unscored" row under its export slot claim, truly
// empty slots read "unequipped", and worn slots ghost nothing. The
// unscorable item also disables scoring and raises the warning modal
// (gearcheck.go) — the list rows are what the dismissed modal leaves
// behind.
func TestGearListGhostSlots(t *testing.T) {
	// reforgedExport gear (head, chest, weapon) plus a sub-floor
	// trinket the cata db does not contain, slot-tagged like every
	// fetch import is
	export := strings.Replace(reforgedExport,
		`{"item":60303,"ench":4209,"gems":[68778,52209,0]}`,
		`{"item":60303,"ench":4209,"gems":[68778,52209,0]},
		{"item":99999,"ench":0,"gems":[0,0,0],"slot":"Trinket"}`, 1)
	m := cataModel(t, export, 2)
	if m.scorer != nil || !m.gapWarn {
		t.Fatalf("an unscorable worn item must disable scoring and warn: scorer=%v warn=%v",
			m.scorer != nil, m.gapWarn)
	}
	m = drive(t, m, tea.KeyMsg{Type: tea.KeyEsc}) // dismiss the warning
	view := ansi.Strip(m.View())

	if !strings.Contains(view, "unscored — worn item below the scored item pool") {
		t.Errorf("missing the unscored trinket row:\n%s", view)
	}
	if strings.Count(view, "unscored") != 1 {
		t.Errorf("want exactly one unscored row:\n%s", view)
	}
	if !strings.Contains(view, "unequipped") {
		t.Errorf("missing unequipped rows for naked slots:\n%s", view)
	}
	for _, line := range strings.Split(view, "\n") {
		// the slot label sits in the row's first columns; the detail
		// pane to the right may mention slot names too
		if len(line) > 30 && strings.Contains(line[:30], "Head") && strings.Contains(line, "unequipped") {
			t.Errorf("worn head slot must not ghost: %q", line)
		}
	}
}

// TestUpgradeDowngradeBudget pins the context-row budget below the
// equipped piece: five nearest-worse rows when the slot is nearly
// done, two once it has real choices, none past ten upgrades.
func TestUpgradeDowngradeBudget(t *testing.T) {
	m := cataModel(t, dualWieldExport, 2)
	budget := func(slot int) (ups, downs int) {
		t.Helper()
		m.cursor = slot
		m = drive(t, m, tea.KeyMsg{Type: tea.KeyEnter})
		if m.mode != modeUpgrades {
			t.Fatal("enter did not open the upgrade browser")
		}
		for _, u := range m.upList {
			if u.Delta > 0.5 {
				ups++
			} else {
				downs++
			}
		}
		m = drive(t, m, tea.KeyMsg{Type: tea.KeyEsc})
		return ups, downs
	}
	sawMid, sawFlood := false, false
	for slot := range m.gearRows() {
		ups, downs := budget(slot)
		var want int
		switch {
		case ups > 10:
			want, sawFlood = 0, true
		case ups > 2:
			want, sawMid = 2, true
		default:
			want = 5
		}
		if downs > want {
			t.Errorf("slot %d: %d upgrades allow %d downgrades, got %d", slot, ups, want, downs)
		}
	}
	if !sawMid && !sawFlood {
		t.Skip("fixture exercised neither budget tier")
	}
}

// TestCataValorGearRaidless pins the valor route of drop-sourced
// gear: the T11 legs/gloves the upstream only knows as Argaloth drops
// are ALSO quartermaster stock — they must pass a raids-off filter,
// render the vendor price, and be solvable into the raid-less BiS.
func TestCataValorGearRaidless(t *testing.T) {
	m := cataModel(t, dualWieldExport, 2)
	m.filter.Raids = false
	m.filter.RaidBoE = false
	m.openBis()
	for _, id := range []int{60304, 60305, 60307} { // T11 chest/legs/gloves
		it := m.game.DB.Items[id]
		if it == nil {
			t.Fatalf("%d not in the cata db", id)
		}
		if !m.allowItem(it) {
			t.Errorf("%s must pass the raids-off filter (valor-priced)", it.Name)
		}
		if line := ansi.Strip(m.sourceText(it)); !strings.Contains(line, "Valor Points") {
			t.Errorf("%s source line missing the valor price: %q", it.Name, line)
		}
	}
	// no raid-locked pick may ENTER the raid-less set. Gear the
	// character already owns is a different matter: BiS keeps what it
	// cannot improve on, and never advises unequipping a piece just
	// because the filter hides where it drops.
	worn := map[int]bool{}
	for _, r := range m.gearRows() {
		worn[r.item.ID] = true
	}
	for _, it := range m.bisSet {
		if !m.allowItem(it) && !worn[it.ID] {
			t.Errorf("raid-less BiS acquires filtered item %s", it.Name)
		}
	}
}

// tunedExport is a fully geared hunter whose gear is REFORGE-TUNED: hit
// landed exactly on the cap by moving rating out of three hit-heavy
// pieces and into two others. That tuning is worth over a thousand
// points of set score, and pricing it against fresh-drop assumptions is
// what made the BiS screen unreadable — see TestCataBiSTunedBasis.
const tunedExport = `{"name":"Zulja","realm":"Gilneas","level":85,"faction":"Horde",
	"race":"Troll","class":"Hunter","origin":"whitemane",
	"equipment":[
		{"item":63459,"ench":4209,"gems":[68778,52209,0],"slot":"Head","reforge":{"source":49,"target":32,"value":80}},
		{"item":57931,"ench":0,"gems":[0,0,0],"slot":"Neck"},
		{"item":58124,"ench":4204,"gems":[52212,0,0],"slot":"Shoulder","reforge":{"source":36,"target":32,"value":60}},
		{"item":56564,"ench":4102,"gems":[52212,0,0],"slot":"Chest","reforge":{"source":36,"target":32,"value":99}},
		{"item":56539,"ench":0,"gems":[52212,52212,0],"slot":"Waist","reforge":{"source":49,"target":31,"value":63}},
		{"item":69615,"ench":4440,"gems":[52212,52212,0],"slot":"Legs"},
		{"item":63446,"ench":4076,"gems":[52212,0,0],"slot":"Feet","reforge":{"source":49,"target":32,"value":60}},
		{"item":56340,"ench":4190,"gems":[0,0,0],"slot":"Wrist","reforge":{"source":31,"target":32,"value":44}},
		{"item":58125,"ench":3222,"gems":[52212,0,0],"slot":"Hands"},
		{"item":68812,"ench":0,"gems":[0,0,0],"slot":"Finger","reforge":{"source":36,"target":32,"value":52}},
		{"item":62362,"ench":0,"gems":[0,0,0],"slot":"Finger","reforge":{"source":49,"target":32,"value":50}},
		{"item":58181,"ench":0,"gems":[0,0,0],"slot":"Trinket","reforge":{"source":31,"target":32,"value":128}},
		{"item":56328,"ench":0,"gems":[0,0,0],"slot":"Trinket","reforge":{"source":31,"target":32,"value":114}},
		{"item":67134,"ench":4100,"gems":[0,0,0],"slot":"Back"},
		{"item":65168,"ench":4227,"gems":[0,0,0],"slot":"Weapon","reforge":{"source":49,"target":31,"value":85}},
		{"item":56366,"ench":4177,"gems":[0,0,0],"slot":"Ranged"},
		{"item":65906,"ench":0,"gems":[0,0,0]}],
	"talents":[{"tree":"Survival","points":null}],
	"professions":[{"name":"Leatherworking","skill":525},{"name":"Skinning","skill":525}]}`

// TestCataBiSTunedBasis pins the accounting basis of the BiS screen on a
// reforge-tuned character. Both sides of every number must be the same
// kind of thing: the solved set is priced with a set-level cap-aware
// reforge plan, and each shopping delta compares the pick against the
// worn piece under those same fresh-drop assumptions. Mixing the bases
// (tuned actuals on the worn side, assumptions on the pick side) made
// every single row read negative — a strictly better set advertised as
// a downgrade.
func TestCataBiSTunedBasis(t *testing.T) {
	m := bisModel(t, tunedExport, 2)
	sc := m.bisScorer

	// the solver's own score is the assumed-plan score of its set
	if got, want := m.bisScore, sc.AssumedSetScore(m.bisSet); math.Abs(got-want) > 0.5 {
		t.Errorf("bis score %.1f, want the assumed-plan score %.1f", got, want)
	}
	// the tuned worn set really is worth more than its own fresh-drop
	// assumption — the trap this test exists for
	ws := wornOf(m.gearRows())
	baseline := m.bisBaseline()
	if sc.SetScoreWorn(ws) <= sc.AssumedSetScore(baseline) {
		t.Fatal("fixture is no longer tuned above its own assumptions")
	}
	// ... and the set the solver found is better still, on one basis
	if sc.AssumedSetScore(m.bisSet) <= sc.AssumedSetScore(baseline) {
		t.Fatal("solved set does not beat the worn set on the assumed basis")
	}

	// every piece worth farming must advertise a gain, not a loss — on
	// the row as rendered, not just in the model
	view := ansi.Strip(m.View())
	needed := 0
	for i := range m.bisSet {
		if !m.bisNeeded(i) {
			continue
		}
		needed++
		d := sc.ItemSwapDelta(baseline, i, m.bisSet[i])
		if d <= 0 {
			t.Errorf("row %d (%s → %s) prices %+.0f — negative on the like-for-like basis",
				i, m.bisWorn[i].Item.Name, m.bisSet[i].Name, d)
			continue
		}
		// the row renders as mark, slot, delta — pin the whole prefix so
		// the number is tied to its slot
		want := fmt.Sprintf("   %-10s%+6.0f ", m.bisSlotLabel(i), d)
		if !strings.Contains(view, want) {
			t.Errorf("no row %q in the list — %s priced on another basis",
				want, m.bisSet[i].Name)
		}
	}
	if needed == 0 {
		t.Fatal("fixture already wears the BiS set — nothing priced")
	}

	// and the summary agrees: a positive gear gain
	if !strings.Contains(view, "from gear)") || strings.Contains(view, "(-") {
		t.Errorf("summary must show a positive gear gain:\n%s", view)
	}
}

// laggingExport is tunedExport with a 333 head — a second rung below
// the set's 346 floor, so the lowest-ilvl escalation has something to
// rank.
var laggingExport = strings.Replace(tunedExport, `{"item":63459,`, `{"item":63829,`, 1)

// TestGearMarkerEscalation pins when the gear list escalates its
// below-average arrow to red. Red means "this is the weakest link,
// replace it first" — which only says something when SOME below-average
// item is not the weakest. On a set spanning one tier (346s under 359s)
// every flagged row is equally lowest, red would repaint the whole
// yellow group, and the legend would advertise a distinction the screen
// does not make.
func TestGearMarkerEscalation(t *testing.T) {
	lipgloss.SetColorProfile(termenv.ANSI256)
	red, yellow := errStyle.Render("▲ "), warnStyle.Render("▲ ")

	// one tier of spread: all yellow, no "lowest" legend entry
	view := cataModel(t, tunedExport, 2).View()
	if !strings.Contains(view, yellow) {
		t.Error("below-average rows are not flagged at all")
	}
	if strings.Contains(view, red) {
		t.Error("red arrow on a set whose below-average rows share one ilvl")
	}
	if strings.Contains(ansi.Strip(view), "lowest") {
		t.Error("legend advertises a red arrow the list never draws")
	}

	// two rungs below the average: the 333 head is the weakest link
	m := cataModel(t, laggingExport, 2)
	m.cursor = 4 // off the head row: the selection restyles its marker
	view = m.View()
	if !strings.Contains(view, red) {
		t.Error("no red arrow on a set with a genuinely lagging piece")
	}
	if !strings.Contains(view, yellow) {
		t.Error("the 346 rows should stay yellow next to the 333 one")
	}
	if !strings.Contains(ansi.Strip(view), "lowest") {
		t.Error("legend missing the lowest entry while a red arrow shows")
	}
	for _, line := range strings.Split(view, "\n") {
		if strings.Contains(ansi.Strip(line), "Shocktrooper Hood") && !strings.Contains(line, red) {
			t.Errorf("the 333 head is not the red row: %q", ansi.Strip(line))
		}
	}
}

// druidExport is reforgedExport's shape for a druid; tree and talent
// ranks are filled per spec.
func druidExport(tree, ranks string) string {
	return `{"name":"Druid","realm":"Gilneas","level":85,"faction":"Horde",
	"race":"Tauren","class":"Druid","origin":"whitemane",
	"equipment":[
		{"item":60303,"ench":0,"gems":[0,0,0]},
		{"item":65168,"ench":0,"gems":[0,0,0]}],
	"talents":[{"tree":"` + tree + `","points":[]}],
	"talent_ranks":{` + ranks + `},
	"professions":[]}`
}

// TestDruidSpecsSolve: every cata druid spec resolves its profile and
// runs the full planner — upgrades and the BiS solve — proposing only
// leather armor.
func TestDruidSpecsSolve(t *testing.T) {
	for _, tc := range []struct{ tree, ranks, want string }{
		{"Balance", `"33596":2`, "Balance Druid"},
		{"Feral Combat", ``, "Feral Druid (cat)"},
		{"Feral Combat", `"57880":2,"80313":1`, "Feral Druid (bear)"},
		{"Restoration", ``, "Restoration Druid"},
	} {
		m := bisModel(t, druidExport(tc.tree, tc.ranks), 3)
		if p := m.baseProfile(); p == nil || p.Name != tc.want {
			t.Fatalf("%s: profile %v, want %s", tc.tree, p, tc.want)
		}
		if len(m.bisSet) == 0 {
			t.Fatalf("%s: BiS solved nothing", tc.want)
		}
		for _, it := range m.bisSet {
			if it.ID == 60303 || it.ID == 65168 {
				continue // the fixture's worn (hunter) items may be kept
			}
			if it.ArmorType != db.ArmorAny && it.ArmorType != db.ArmorLeather && it.Slot != db.SlotBack {
				t.Errorf("%s: BiS picked %s (%v armor)", tc.want, it.Name, it.ArmorType)
			}
		}
	}
}
