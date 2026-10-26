package score

import (
	"testing"

	"github.com/vaclav-dvorak/twolk/internal/db"
)

// reforgeProfile is testProfile with a reforging rule set (WotLK
// defaults otherwise, so gem/enchant behavior stays identical) and
// enough secondary weights to differentiate reforge targets.
func reforgeProfile() *Profile {
	r := *WotLK
	r.Reforging = true
	return &Profile{
		Name: "Test Reforge Hunter", Class: db.ClassHunter, Tree: "Survival",
		Weights: statsOf(map[db.Stat]float64{
			db.Agility:    2.0,
			db.MeleeHit:   3.0,
			db.MeleeCrit:  1.0,
			db.MeleeHaste: 0.8,
			db.Mastery:    0.5,
		}),
		Caps:      []StatCap{{Stat: db.MeleeHit, Cap: 100}},
		ArmorType: db.ArmorMail,
		Rules:     &r,
	}
}

func TestAssumedReforge(t *testing.T) {
	d := testDatabase()
	s := New(d, reforgeProfile())

	// best pair wins: mastery→haste (80×0.3) beats crit→haste (negative);
	// hit is the top weight but CAPPED, so it is never a target
	it := &db.Item{ID: 50, Name: "stat stick", Slot: db.SlotWeapon}
	it.Stats[db.MeleeCrit] = 100
	it.Stats[db.Mastery] = 200
	r := s.AssumedReforge(it)
	if r == nil || r.From != db.Mastery || r.To != db.MeleeHaste || r.Amount != 80 {
		t.Fatalf("assumed reforge = %+v, want 80 Mastery→Melee Haste (hit capped)", r)
	}
	eff := s.EffectiveStats(it)
	if eff[db.MeleeHaste] != 80 || eff[db.Mastery] != 120 {
		t.Fatalf("effective stats miss the reforge: haste=%v mastery=%v, want 80/120",
			eff[db.MeleeHaste], eff[db.Mastery])
	}

	// a native stat cannot be the target: haste present → crit is next
	it2 := &db.Item{ID: 51, Name: "haste stick", Slot: db.SlotWeapon}
	it2.Stats[db.MeleeHaste] = 50
	it2.Stats[db.Mastery] = 200
	if r := s.AssumedReforge(it2); r == nil || r.From != db.Mastery || r.To != db.MeleeCrit {
		t.Fatalf("assumed reforge = %+v, want Mastery→Melee Crit (haste is native)", r)
	}

	// the game floors the 40%: 67 mastery moves 26, not 26.8
	it3 := &db.Item{ID: 52, Name: "odd stick", Slot: db.SlotWeapon}
	it3.Stats[db.Mastery] = 67
	if r := s.AssumedReforge(it3); r == nil || r.Amount != 26 {
		t.Fatalf("assumed reforge = %+v, want floored amount 26", r)
	}

	// no positive gain (only the top uncapped weight present, all
	// targets worse) → no reforge assumed
	it4 := &db.Item{ID: 53, Name: "pure crit", Slot: db.SlotWeapon}
	it4.Stats[db.MeleeCrit] = 100
	if r := s.AssumedReforge(it4); r != nil {
		t.Fatalf("assumed reforge = %+v, want none (no gaining target)", r)
	}
}

// TestAssumedReforgeOn pins the cap-aware set-context assumption: on a
// concrete base, a capped stat is a legal reforge target and pays
// exactly up to the remaining gap — the upgrade ranking's pick.
func TestAssumedReforgeOn(t *testing.T) {
	d := testDatabase()
	s := New(d, reforgeProfile()) // hit weight 3.0, capped at 100

	it := &db.Item{ID: 50, Name: "stat stick", Slot: db.SlotWeapon}
	it.Stats[db.Mastery] = 200 // 80 moves

	// far under the cap: hit is the top target (context-free says haste)
	var under db.Stats
	if r := s.AssumedReforgeOn(it, under); r == nil || r.From != db.Mastery ||
		r.To != db.MeleeHit || r.Amount != 80 {
		t.Fatalf("under cap: assumed = %+v, want 80 Mastery→Melee Hit", r)
	}

	// 40 under the cap: 40 hit points at weight 3.0 still beat 80 haste
	// at 0.8 (120-40 mastery loss vs 24)
	var near db.Stats
	near[db.MeleeHit] = 60
	if r := s.AssumedReforgeOn(it, near); r == nil || r.To != db.MeleeHit {
		t.Fatalf("near cap: assumed = %+v, want →Melee Hit (gap still pays)", r)
	}

	// at the cap: overflow is worthless, the best uncapped target wins
	var capped db.Stats
	capped[db.MeleeHit] = 100
	if r := s.AssumedReforgeOn(it, capped); r == nil || r.To != db.MeleeCrit {
		t.Fatalf("at cap: assumed = %+v, want →Melee Crit", r)
	}
}

func TestAssumedReforgeFollowsCalibration(t *testing.T) {
	d := testDatabase()
	s := New(d, reforgeProfile())

	// under the cap, native hit (weight 3.0) is never reforged away
	it := &db.Item{ID: 50, Name: "hit stick", Slot: db.SlotWeapon}
	it.Stats[db.MeleeHit] = 100
	if r := s.AssumedReforge(it); r != nil {
		t.Fatalf("under the cap hit must stay, got %+v", r)
	}

	// a set saturating the hit cap zeroes hit in the calibrated
	// weights — now reforging OUT of hit gains value
	capped := item(60, db.SlotHead, db.ArmorMail, db.MeleeHit, 200)
	d.Items[capped.ID] = capped
	s.Calibrate([]*db.Item{capped})
	if r := s.AssumedReforge(it); r == nil || r.From != db.MeleeHit || r.To != db.MeleeCrit {
		t.Fatalf("saturated cap: assumed reforge = %+v, want Melee Hit→Melee Crit", r)
	}
}

func TestReforgeDisabledByRules(t *testing.T) {
	d := testDatabase()
	s := New(d, testProfile()) // WotLK rules: no reforging

	it := &db.Item{ID: 50, Name: "stat stick", Slot: db.SlotWeapon}
	it.Stats[db.Mastery] = 200
	if r := s.AssumedReforge(it); r != nil {
		t.Fatalf("wotlk profile assumed a reforge: %+v", r)
	}
}

func TestOptimizeReforges(t *testing.T) {
	d := testDatabase()
	// crit weighted high enough that the LAST hit reforge (mostly
	// overcap) loses to it: the optimizer must split the set
	r := *WotLK
	r.Reforging = true
	prof := &Profile{
		Name: "Test Split Hunter", Class: db.ClassHunter, Tree: "Survival",
		Weights: statsOf(map[db.Stat]float64{
			db.Agility: 2.0, db.MeleeHit: 3.0, db.MeleeCrit: 2.0, db.Mastery: 0.5,
		}),
		Caps:      []StatCap{{Stat: db.MeleeHit, Cap: 100}},
		ArmorType: db.ArmorMail,
		Rules:     &r,
	}
	s := New(d, prof)

	// three identical mastery items, each able to move 40 into hit; the
	// cap (100) fits 2.5 of them — optimal is hit, hit, crit
	var ws []Worn
	for i := 0; i < 3; i++ {
		it := item(70+i, db.SlotHead, db.ArmorMail, db.Mastery, 100)
		d.Items[it.ID] = it
		ws = append(ws, Worn{Item: it, HasDetail: true})
	}
	tips := s.OptimizeReforges(ws)
	if len(tips) != 3 {
		t.Fatalf("want 3 tips, got %d: %+v", len(tips), tips)
	}
	toHit, toCrit := 0, 0
	for _, tip := range tips {
		if tip.From != nil || tip.To == nil || tip.To.From != db.Mastery {
			t.Fatalf("unexpected tip shape: %+v", tip)
		}
		switch tip.To.To {
		case db.MeleeHit:
			toHit++
		case db.MeleeCrit:
			toCrit++
		}
	}
	if toHit != 2 || toCrit != 1 {
		t.Fatalf("want the set split 2×hit 1×crit around the cap, got %d/%d", toHit, toCrit)
	}
	// combined gain is exact: 80×3 (hit to cap, 20 over) + 40×2 (crit)
	// − 120×0.5 (mastery source) = 320 − 60... per-change deltas price
	// alone and may sum higher near the cap
	if gain := s.ReforgeGain(ws, tips); gain < 250 || gain > 321 {
		t.Fatalf("combined reforge gain = %.0f, want ~300", gain)
	}

	// a worn reforge that only loses value gets a CLEAR tip (To nil):
	// hit already saturated by another item, crit is the top secondary,
	// yet the item is reforged crit→mastery
	capItem := item(80, db.SlotChest, db.ArmorMail, db.MeleeHit, 200)
	d.Items[capItem.ID] = capItem
	bad := item(81, db.SlotLegs, db.ArmorMail, db.MeleeCrit, 100)
	d.Items[bad.ID] = bad
	ws2 := []Worn{
		{Item: capItem, HasDetail: true},
		{Item: bad, HasDetail: true,
			Reforge: &db.Reforge{From: db.MeleeCrit, To: db.Mastery, Amount: 40}},
	}
	tips2 := s.OptimizeReforges(ws2)
	if len(tips2) != 2 {
		t.Fatalf("want 2 tips (reforge out of overcap + clear), got %+v", tips2)
	}
	// best first: the saturated item reforges its surplus hit into crit
	// (80×2, staying at the cap), the mis-reforged item clears
	out, clear := tips2[0], tips2[1]
	if out.ItemIdx != 0 || out.To == nil || out.To.From != db.MeleeHit ||
		out.To.To != db.MeleeCrit || out.Delta != 160 {
		t.Fatalf("overcap tip = %+v, want 80 Hit→Crit (+160)", out)
	}
	if clear.ItemIdx != 1 || clear.To != nil || clear.Delta != 40*(2.0-0.5) {
		t.Fatalf("clear tip = %+v, want clear (+60)", clear)
	}

	// wotlk profile: no reforging, no tips
	if tips := New(d, testProfile()).OptimizeReforges(ws); tips != nil {
		t.Fatalf("wotlk profile produced reforge tips: %+v", tips)
	}

	// churn floor: neighboring secondary weights sit ~0.01 apart — a
	// "move 40 rating for +0.4" retarget is never worth suggesting
	churn := *WotLK
	churn.Reforging = true
	sc := New(d, &Profile{
		Name: "Test Churn Hunter", Class: db.ClassHunter, Tree: "Survival",
		Weights: statsOf(map[db.Stat]float64{
			db.Mastery: 0.88, db.MeleeHaste: 0.89,
		}),
		ArmorType: db.ArmorMail,
		Rules:     &churn,
	})
	flat := item(85, db.SlotWaist, db.ArmorMail, db.Mastery, 100)
	d.Items[flat.ID] = flat
	if tips := sc.OptimizeReforges([]Worn{{Item: flat, HasDetail: true}}); len(tips) != 0 {
		t.Fatalf("sub-point retarget must be suppressed, got %+v", tips)
	}
}

func TestWornActualReforge(t *testing.T) {
	d := testDatabase()
	s := New(d, reforgeProfile())

	it := item(90, db.SlotHead, db.ArmorMail, db.MeleeCrit, 100)
	d.Items[it.ID] = it

	// detailed worn item: the ACTUAL reforge counts, no assumption
	w := Worn{Item: it, HasDetail: true,
		Reforge: &db.Reforge{From: db.MeleeCrit, To: db.MeleeHit, Amount: 40}}
	got := s.WornStats(w)
	if got[db.MeleeCrit] != 60 || got[db.MeleeHit] != 40 {
		t.Fatalf("worn reforge not applied: crit=%v hit=%v, want 60/40", got[db.MeleeCrit], got[db.MeleeHit])
	}

	// detailed but unreforged: actuals stay actuals — nothing assumed
	if got := s.WornStats(Worn{Item: it, HasDetail: true}); got[db.MeleeHit] != 0 {
		t.Fatalf("unreforged worn item got an assumed reforge: hit=%v", got[db.MeleeHit])
	}

	// detail-less worn falls back to assumptions, reforge included
	itM := item(91, db.SlotHands, db.ArmorMail, db.Mastery, 100)
	d.Items[itM.ID] = itM
	if got := s.WornStats(Worn{Item: itM}); got[db.MeleeCrit] != 40 || got[db.Mastery] != 60 {
		t.Fatalf("detail-less worn misses assumed reforge: crit=%v mastery=%v",
			got[db.MeleeCrit], got[db.Mastery])
	}
}

// TestEnchantTipsReforgeAware pins the enchant/reforge coupling at the
// hit cap (session 26): an empty slot must never go unproposed just
// because the weight-ranked FIRST candidate is a hit enchant that
// gains nothing on a reforge-capped set — every candidate is priced,
// and hit candidates are priced jointly with a reforge re-solve (the
// enchanted hit un-buys reforged hit elsewhere).
func TestEnchantTipsReforgeAware(t *testing.T) {
	d := testDatabase()
	// head: 250 mastery — reforging 40% (=100) into hit lands the 100
	// cap exactly, which is what OptimizeReforges will do
	head := item(60, db.SlotHead, db.ArmorMail, db.Mastery, 250)
	gun := item(61, db.SlotRanged, db.ArmorAny, db.Agility, 10)
	d.Items[head.ID], d.Items[gun.ID] = head, gun

	hitEnch := &db.Enchant{EffectID: 910, Name: "Hit Scope", Slot: db.SlotRanged}
	hitEnch.Stats[db.MeleeHit] = 88
	agiEnch := &db.Enchant{EffectID: 911, Name: "Agi Scope", Slot: db.SlotRanged}
	agiEnch.Stats[db.Agility] = 40
	d.Enchants = []*db.Enchant{hitEnch, agiEnch}

	s := New(d, reforgeProfile()) // hit 3.0 capped 100, agi 2.0, crit 1.0, mastery 0.5
	ws := []Worn{
		{Item: head, HasDetail: true},
		{Item: gun, HasDetail: true}, // enchant slot EMPTY
	}

	plan := s.Optimize(ws)
	// the reforge pass caps hit (head mastery→hit 100)…
	if len(plan.Reforges) != 1 || plan.Reforges[0].To == nil || plan.Reforges[0].To.To != db.MeleeHit {
		t.Fatalf("reforge plan = %+v, want head mastery→hit", plan.Reforges)
	}
	// …and the gun still gets an enchant: agi (exact +80) beats the
	// weight-ranked hit scope (reforge-aware: 88 hit frees the head
	// reforge into crit, 264+100-300 = +64)
	if len(plan.Enchants) != 1 || plan.Enchants[0].To.EffectID != agiEnch.EffectID {
		t.Fatalf("enchant plan = %+v, want the agi scope on the gun", plan.Enchants)
	}

	// with only a WEAK agi alternative the hit scope must win via the
	// joint value — the old single-pick logic proposed NOTHING here
	agiEnch.Stats[db.Agility] = 10 // exact +20 < joint +64
	plan = s.Optimize(ws)
	if len(plan.Enchants) != 1 || plan.Enchants[0].To.EffectID != hitEnch.EffectID {
		t.Fatalf("enchant plan = %+v, want the hit scope (reforge-aware value)", plan.Enchants)
	}
	// and the joint plan re-points the head reforge at crit — the
	// enchanted hit carries the cap alone
	if len(plan.Reforges) != 1 || plan.Reforges[0].To.To != db.MeleeCrit {
		t.Fatalf("reforge plan = %+v, want head mastery→crit alongside the hit scope", plan.Reforges)
	}
	// composition invariant: Set is EXACTLY worn + the listed sections
	// (the summary pane prices Set — un-tipped solver state must never
	// ride along)
	composed := append([]Worn(nil), ws...)
	for _, tip := range plan.Enchants {
		composed[tip.ItemIdx].EnchantID = tip.To.EffectID
	}
	for _, tip := range plan.Reforges {
		composed[tip.ItemIdx].Reforge = tip.To
	}
	if c, p := s.TotalsWorn(composed), s.TotalsWorn(plan.Set); c != p {
		t.Fatalf("plan.Set diverges from the listed changes: composed=%v set=%v", c, p)
	}
	agiEnch.Stats[db.Agility] = 40

	// phase ceiling: a post-launch enchant is not proposable below its
	// phase, and returns once the ceiling lifts
	agiEnch.Phase = 3
	s.SetServerPhase(1)
	if tips := s.EnchantTips(ws); len(tips) != 1 || tips[0].To.EffectID != hitEnch.EffectID {
		t.Fatalf("P1 tips = %+v, want only the hit scope", tips)
	}
	s.SetServerPhase(3)
	if tips := s.EnchantTips(ws); len(tips) != 1 || tips[0].To.EffectID != agiEnch.EffectID {
		t.Fatalf("P3 tips = %+v, want the agi scope back", tips)
	}
}

// TestAssumedPlan pins the set-level reforge assumption against the two
// errors the per-item one cannot avoid: a set far OVER a cap keeps the
// surplus (nothing dumps it) and a set UNDER the cap never reforges
// into it. Both cost real score, and both broke the BiS screen: the
// solved set was priced hundreds below what the same gear is worth once
// reforged, so every shopping delta measured against tuned worn gear
// came out negative.
func TestAssumedPlan(t *testing.T) {
	d := testDatabase()
	s := New(d, reforgeProfile()) // hit weight 3.0, hit cap 100

	// two hit-heavy pieces: 300 hit against a cap of 100. The per-item
	// assumption keeps every point (hit is its top weight); the plan
	// dumps the surplus into crit.
	hitty := func(id int) *db.Item {
		it := &db.Item{ID: id, Name: "hit stick", Slot: db.SlotWeapon}
		it.Stats[db.MeleeHit] = 150
		it.Stats[db.Mastery] = 100
		return it
	}
	set := []*db.Item{hitty(60), hitty(61)}
	tot, refs := s.AssumedPlan(set)
	// each piece can shed one 60-rating chunk (40% of its 150 hit), so
	// 180 is the floor the reforge granularity allows — the point is
	// that BOTH pieces shed theirs
	if tot[db.MeleeHit] != 180 {
		t.Errorf("plan totals %v hit against a cap of 100, want 180 (both chunks dumped)",
			tot[db.MeleeHit])
	}
	for i, r := range refs {
		if r == nil || r.From != db.MeleeHit {
			t.Errorf("plan reforge %d = %+v, want hit moved out", i, r)
		}
	}
	if got, want := s.AssumedSetScore(set), s.SetScoreWorn(wrapWorn(set)); got <= want {
		t.Errorf("plan score %.0f not above the per-item basis %.0f", got, want)
	}

	// under the cap the plan reforges INTO hit — the assumption the
	// per-item pick refuses because hit is a capped stat
	lean := &db.Item{ID: 62, Name: "mastery stick", Slot: db.SlotWeapon}
	lean.Stats[db.Mastery] = 100
	_, refs = s.AssumedPlan([]*db.Item{lean})
	if refs[0] == nil || refs[0].To != db.MeleeHit {
		t.Errorf("under-cap plan reforge = %+v, want mastery → hit", refs[0])
	}
	if r := s.AssumedReforge(lean); r != nil && r.To == db.MeleeHit {
		t.Error("per-item assumption should still refuse a capped target")
	}
}

// TestItemSwapDeltaBasis pins the like-for-like pricing: the same item
// swap must come out identical whether the baseline set is scored as
// worn instances or as plain items, and a strictly better piece must
// price positive even when the worn side is perfectly tuned.
func TestItemSwapDeltaBasis(t *testing.T) {
	d := testDatabase()
	s := New(d, reforgeProfile())

	base := &db.Item{ID: 70, Name: "plain", Slot: db.SlotWeapon, ILvl: 300}
	base.Stats[db.Agility] = 100
	better := &db.Item{ID: 71, Name: "plain plus", Slot: db.SlotWeapon, ILvl: 359}
	better.Stats[db.Agility] = 200
	filler := &db.Item{ID: 72, Name: "filler", Slot: db.SlotChest}
	filler.Stats[db.MeleeHit] = 150 // pushes the set over the hit cap

	set := []*db.Item{base, filler}
	got := s.ItemSwapDelta(set, 0, better)
	if got <= 0 {
		t.Errorf("strictly better item prices %+.0f on the like-for-like basis", got)
	}
	// 100 agility at weight 2.0, with the hit surplus reforged the same
	// way on both sides
	if want := 200.0; got < want-1 || got > want+1 {
		t.Errorf("delta %+.1f, want %+.0f (100 agility × 2.0)", got, want)
	}
}
