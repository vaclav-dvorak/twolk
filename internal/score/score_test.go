package score

import (
	"testing"
	"time"

	"github.com/vaclav-dvorak/twolk/internal/db"
)

// statsOf builds a Stats array from a sparse map (test fixtures only —
// the profile registry composes from simWeights).
func statsOf(m map[db.Stat]float64) db.Stats {
	var s db.Stats
	for st, v := range m {
		s[st] = v
	}
	return s
}

func testProfile() *Profile {
	return &Profile{
		Name:  "Test Hunter",
		Class: db.ClassHunter,
		Tree:  "Survival",
		Weights: statsOf(map[db.Stat]float64{
			db.Agility:           2.0,
			db.MeleeHit:          3.0,
			db.RangedAttackPower: 1.0,
		}),
		Caps:      []StatCap{{Stat: db.MeleeHit, Cap: 100}},
		ArmorType: db.ArmorMail,
	}
}

func gem(id int, name string, color db.GemColor, q db.Quality, st db.Stat, v float64) *db.Gem {
	g := &db.Gem{ID: id, Name: name, Color: color, Quality: q}
	g.Stats[st] = v
	return g
}

// jcGem marks a fixture gem jewelcrafter-only, as the upstream data
// does (requiredProfession 7).
func jcGem(g *db.Gem) *db.Gem {
	g.RequiredProfession = 7
	return g
}

func item(id int, slot db.Slot, armor db.ArmorType, st db.Stat, v float64) *db.Item {
	it := &db.Item{ID: id, Name: "item", Slot: slot, ArmorType: armor}
	it.Stats[st] = v
	return it
}

func testDatabase() *db.Database {
	return &db.Database{
		Items: map[int]*db.Item{},
		Gems: map[int]*db.Gem{
			1: gem(1, "Delicate Cardinal Ruby", db.GemRed, db.QualityEpic, db.Agility, 20),
			2: gem(2, "Delicate Scarlet Ruby", db.GemRed, db.QualityRare, db.Agility, 16),        // rare tier: skipped
			3: jcGem(gem(3, "Delicate Dragon's Eye", db.GemRed, db.QualityEpic, db.Agility, 34)), // JC-only: skipped
			4: gem(4, "Relentless Earthsiege Diamond", db.GemMeta, db.QualityRare, db.Agility, 21),
		},
		Effects: map[int][]*db.Effect{},
		Rules:   db.WotlkRules(),
	}
}

func TestBestGemSelection(t *testing.T) {
	s := New(testDatabase(), testProfile())
	if g := s.BestGem(db.GemRed); g == nil || g.ID != 1 {
		t.Fatalf("red socket gem = %+v, want Cardinal Ruby (epic, non-JC)", g)
	}
	if g := s.BestGem(db.GemMeta); g == nil || g.ID != 4 {
		t.Fatalf("meta socket gem = %+v, want rare diamond", g)
	}
}

func TestEffectiveStatsGemsAndBonus(t *testing.T) {
	d := testDatabase()
	it := item(10, db.SlotHead, db.ArmorMail, db.Agility, 60)
	it.GemSockets = []db.GemColor{db.GemRed}
	it.SocketBonus[db.Agility] = 4
	d.Items[it.ID] = it

	s := New(d, testProfile())
	// 60 base + 20 gem + 4 socket bonus
	if got := s.EffectiveStats(it)[db.Agility]; got != 84 {
		t.Fatalf("effective agility = %v, want 84", got)
	}
}

func TestGearStatsExcludeEffects(t *testing.T) {
	d := testDatabase()
	trinket := item(60, db.SlotTrinket, db.ArmorAny, db.Agility, 90)
	d.Items[trinket.ID] = trinket
	curated := &db.Effect{ItemID: 60, Kind: db.EffectCurated}
	curated.Bonus[db.Agility] = 90 // DMC: Greatness style avg proc
	d.Effects[60] = []*db.Effect{curated}

	s := New(d, testProfile())
	if got := s.GearStats(trinket)[db.Agility]; got != 90 {
		t.Errorf("passive agility = %v, want 90 (no proc merged)", got)
	}
	if got := s.EffectiveStats(trinket)[db.Agility]; got != 180 {
		t.Errorf("effective agility = %v, want 180 (proc avg included)", got)
	}
}

func TestEffectAverages(t *testing.T) {
	// the default profile states no ProcInterval, so these exercise the
	// engine default cadence (defaultProcInterval)
	s := New(testDatabase(), testProfile())
	onUse := &db.Effect{Kind: db.EffectOnUse, Duration: 20 * time.Second, Cooldown: 2 * time.Minute}
	onUse.Bonus[db.MeleeHaste] = 600
	if got := s.effectAverage(onUse)[db.MeleeHaste]; got != 100 {
		t.Errorf("on-use avg = %v, want 100 (600 * 20/120)", got)
	}

	proc := &db.Effect{Kind: db.EffectProc, Duration: 10 * time.Second,
		Cooldown: 45 * time.Second, ProcChance: 0.5}
	proc.Bonus[db.RangedAttackPower] = 490
	// wait = 2s/0.5 = 4s; uptime = 10/(45+4) ≈ 0.204
	got := s.effectAverage(proc)[db.RangedAttackPower]
	if got < 99 || got > 101 {
		t.Errorf("proc avg = %v, want ≈100", got)
	}

	curated := &db.Effect{Kind: db.EffectCurated}
	curated.Bonus[db.Agility] = 90
	if got := s.effectAverage(curated)[db.Agility]; got != 90 {
		t.Errorf("curated avg = %v, want passthrough 90", got)
	}
}

func TestStackingEffectAverages(t *testing.T) {
	s := New(testDatabase(), testProfile())
	// Fluid Death: 38 agility/stack ×10, refreshed every ~2s trigger —
	// held at full stacks minus the 20s ramp amortized over the fight:
	// 38 × 10 × (1 - 20/600) ≈ 367
	fluid := &db.Effect{Kind: db.EffectProc, Duration: 15 * time.Second,
		ProcChance: 1, MaxStacks: 10}
	fluid.Bonus[db.Agility] = 38
	if got := s.effectAverage(fluid)[db.Agility]; got < 366 || got > 368 {
		t.Errorf("fast stacking proc avg = %v, want ≈367", got)
	}

	// a stack cadence slower than the buff window never stacks: plain
	// single-stack uptime, 100 × 15/(75+20) ≈ 15.8
	slow := &db.Effect{Kind: db.EffectProc, Duration: 15 * time.Second,
		Cooldown: 75 * time.Second, ProcChance: 0.1, MaxStacks: 10}
	slow.Bonus[db.Agility] = 100
	if got := s.effectAverage(slow)[db.Agility]; got < 15 || got > 17 {
		t.Errorf("slow stacking proc avg = %v, want ≈15.8", got)
	}

	// Victor's Call: on-use window builds 215 AP/stack ×5, one stack
	// per ~2s trigger → ramp to 5 in 10s of the 20s window: average
	// 5×(1-10/40) = 3.75 stacks × 215 × 20/120 ≈ 134
	victor := &db.Effect{Kind: db.EffectOnUse, Duration: 20 * time.Second,
		Cooldown: 2 * time.Minute, MaxStacks: 5}
	victor.Bonus[db.AttackPower] = 215
	if got := s.effectAverage(victor)[db.AttackPower]; got < 133 || got > 136 {
		t.Errorf("stacking on-use avg = %v, want ≈134", got)
	}
}

// The trigger gate: a proc a build cannot fire is worth zero, one it
// can (or one whose trigger is unreadable) keeps the uptime average.
func TestProcTriggerGate(t *testing.T) {
	hunter := testProfile() // ranged-only rotation
	warrior := &Profile{Class: db.ClassWarrior, Tree: "Fury"}
	resto := &Profile{Class: db.ClassDruid, Tree: "Restoration"}

	proc := func(mask, callback string) *db.Effect {
		return &db.Effect{Kind: db.EffectProc, ProcMask: mask, Callback: callback}
	}
	cases := []struct {
		e    *db.Effect
		p    *Profile
		want bool
		desc string
	}{
		{proc("Melee", "OnSpellHitDealt"), hunter, false, "melee-only for hunter (Schnottz's Medallion)"},
		{proc("Melee", "OnSpellHitDealt"), warrior, true, "melee-only for warrior"},
		{proc("MeleeOrRanged", "OnSpellHitDealt"), hunter, true, "melee-or-ranged for hunter"},
		{proc("Ranged", "OnSpellHitDealt"), warrior, false, "ranged-only for warrior"},
		{proc("SpellHealing", "OnHealDealt"), resto, true, "heal proc for resto"},
		{proc("SpellHealing", "OnHealDealt"), hunter, false, "heal proc for hunter"},
		{proc("SpellDamage | SpellHealing", "OnSpellHitDealt | OnHealDealt"), resto, true, "hybrid heal/damage for resto"},
		{proc("Melee", "OnSpellHitTaken"), resto, true, "incoming-hit proc: attacker's mask, never gated"},
		{proc("config.ProcMask", "config.Callback"), hunter, true, "unreadable templated mask: never gated"},
		{proc("", ""), hunter, true, "no trigger info: never gated"},
		{&db.Effect{Kind: db.EffectOnUse}, hunter, true, "on-use: never gated"},
	}
	for _, c := range cases {
		if got := c.p.CanTrigger(c.e); got != c.want {
			t.Errorf("%s: CanTrigger = %v, want %v", c.desc, got, c.want)
		}
	}

	// end to end: the scorer keeps the trinket's passive stats and
	// drops only the unreachable proc
	d := testDatabase()
	trinket := item(70, db.SlotTrinket, db.ArmorAny, db.Mastery, 234)
	d.Items[70] = trinket
	e := proc("Melee", "OnSpellHitDealt")
	e.Duration, e.Cooldown, e.ProcChance = 20*time.Second, 100*time.Second, 0.1
	e.Bonus[db.Mastery] = 918
	d.Effects[70] = []*db.Effect{e}
	s := New(d, testProfile())
	if got := s.EffectiveStats(trinket)[db.Mastery]; got != 234 {
		t.Errorf("hunter effective mastery = %v, want 234 (proc gated to zero)", got)
	}
}

func TestCalibrateGemsToCaps(t *testing.T) {
	d := testDatabase()
	d.Gems[5] = gem(5, "Rigid King's Amber", db.GemYellow, db.QualityEpic, db.MeleeHit, 20)
	d.Gems[6] = gem(6, "Delicate Ametrine", db.GemOrange, db.QualityEpic, db.Agility, 15)

	s := New(d, testProfile()) // hit weight 3, agi 2, hit cap 100
	// raw weights: hit gem (60) beats the orange agi gem (30) for yellow
	if g := s.BestGem(db.GemYellow); g == nil || g.ID != 5 {
		t.Fatalf("under cap, yellow gem = %+v, want the hit gem", g)
	}

	// a set whose bare gear saturates the hit cap → hit gems are
	// worthless, yellow sockets switch to the agility gem
	capped := item(40, db.SlotChest, db.ArmorMail, db.MeleeHit, 120)
	s.Calibrate([]*db.Item{capped})
	if g := s.BestGem(db.GemYellow); g == nil || g.ID != 6 {
		t.Fatalf("at cap, yellow gem = %+v, want the agility gem", g)
	}

	// and back: an under-cap set restores the hit gem
	s.Calibrate([]*db.Item{item(41, db.SlotChest, db.ArmorMail, db.MeleeHit, 50)})
	if g := s.BestGem(db.GemYellow); g == nil || g.ID != 5 {
		t.Fatalf("recalibration back failed: %+v", g)
	}
}

// TestBestEnchantFollowsCalibration: with raw weights a +20 hit glove
// enchant (60) beats +44 AP (44); once the set caps hit, the assumption
// must flip to the AP enchant — hit past the cap is worth nothing.
func TestBestEnchantFollowsCalibration(t *testing.T) {
	d := testDatabase()
	d.Enchants = []*db.Enchant{
		{EffectID: 1, Name: "Expert Precision", Slot: db.SlotHands,
			Stats: statsOf(map[db.Stat]float64{db.MeleeHit: 20})},
		{EffectID: 2, Name: "Crusher", Slot: db.SlotHands,
			Stats: statsOf(map[db.Stat]float64{db.RangedAttackPower: 44})},
	}
	gloves := item(70, db.SlotHands, db.ArmorMail, db.Agility, 30)
	d.Items[gloves.ID] = gloves

	s := New(d, testProfile()) // hit weight 3, AP 1, hit cap 100
	if e := s.BestEnchant(gloves); e == nil || e.EffectID != 1 {
		t.Fatalf("under cap, glove enchant = %+v, want the hit enchant", e)
	}

	// bare gear saturates the cap → the AP enchant wins
	s.Calibrate([]*db.Item{item(40, db.SlotChest, db.ArmorMail, db.MeleeHit, 120), gloves})
	if e := s.BestEnchant(gloves); e == nil || e.EffectID != 2 {
		t.Fatalf("at cap, glove enchant = %+v, want the AP enchant", e)
	}

	// the user's actual case: 3 points below the cap, hit gems on offer —
	// the under-cap policy comparison must drop hit (chasing 3 points
	// overshoots in every socket) and flip the enchant too
	d.Gems[5] = gem(5, "Rigid King's Amber", db.GemYellow, db.QualityEpic, db.MeleeHit, 20)
	d.Gems[6] = gem(6, "Delicate Ametrine", db.GemOrange, db.QualityEpic, db.Agility, 15)
	chest := item(41, db.SlotChest, db.ArmorMail, db.MeleeHit, 97)
	chest.GemSockets = []db.GemColor{db.GemYellow}
	d.Items[chest.ID] = chest
	s.Calibrate([]*db.Item{chest, gloves})
	if e := s.BestEnchant(gloves); e == nil || e.EffectID != 2 {
		t.Fatalf("3 below cap, glove enchant = %+v, want the AP enchant", e)
	}
}

// TestCandidateCannotAssumeWornUnique: a unique-equipped gem socketed
// on a KEPT worn item is off the table for upgrade candidates — they
// fall back (here: ignore the bonus with the best non-unique gem). The
// replaced item's own unique frees up and stays assumable.
func TestCandidateCannotAssumeWornUnique(t *testing.T) {
	d := testDatabase()
	tear := gem(7, "Nightmare Tear", db.GemPrismatic, db.QualityEpic, db.Agility, 10)
	tear.Unique = true
	d.Gems[7] = tear

	cand := item(80, db.SlotHands, db.ArmorMail, db.Agility, 30)
	cand.GemSockets = []db.GemColor{db.GemBlue} // only the Tear fits blue
	cand.SocketBonus[db.Agility] = 15           // big enough to prefer matching
	chest := item(81, db.SlotChest, db.ArmorMail, db.Agility, 50)
	worn := item(82, db.SlotHands, db.ArmorMail, db.Agility, 25)
	for _, it := range []*db.Item{cand, chest, worn} {
		d.Items[it.ID] = it
	}

	s := New(d, testProfile())
	// unconstrained: the blue socket takes the Tear, bonus kept
	if gems, matched := s.AssumedGems(cand); !matched || gems[0] == nil || gems[0].ID != 7 {
		t.Fatalf("unconstrained assumption = %+v (matched %v), want the Tear", gems, matched)
	}

	// the chest (kept) holds the Tear → glove candidates cannot assume it
	ws := []Worn{
		{Item: worn, HasDetail: true},
		{Item: chest, Gems: []int{7}, HasDetail: true},
	}
	ctx := s.SwapContextFor(ws, 0)
	if !ctx.Taken[7] || len(ctx.Carry) != 0 {
		t.Fatalf("ctx = %+v, want the chest's Tear taken and nothing carried", ctx)
	}
	gems, matched := s.AssumedGemsSwap(cand, ctx)
	if len(gems) > 0 && gems[0] != nil && gems[0].ID == 7 {
		t.Fatalf("candidate assumed the Tear socketed on the chest: %+v", gems)
	}
	if matched {
		t.Fatalf("no non-unique gem matches blue: the bonus cannot be kept")
	}
	// ranking must price the fallback (30 base + 20 ruby, no bonus), not
	// the Tear package (30 + 10 + 15): delta (55-25)*2 = 60 would be wrong
	ups := s.UpgradesWorn(ws, 0, 10, nil)
	if len(ups) == 0 || ups[0].Item.ID != cand.ID || ups[0].Delta != 50 {
		t.Fatalf("upgrades = %+v, want the glove candidate at +50", ups)
	}

	// the Tear on the REPLACED item is not taken — candidates inherit it
	worn.GemSockets = []db.GemColor{db.GemBlue}
	ws2 := []Worn{
		{Item: worn, Gems: []int{7}, HasDetail: true},
		{Item: chest, HasDetail: true},
	}
	ctx2 := s.SwapContextFor(ws2, 0)
	if ctx2.Taken[7] || len(ctx2.Carry) != 1 || ctx2.Carry[0].ID != 7 {
		t.Fatalf("ctx = %+v, want the replaced item's Tear carried, not taken", ctx2)
	}
}

// TestCarriedPrismaticStaysOnCandidate: the replaced item's Nightmare
// Tear is a meta-keeper — the candidate re-socketes it even where the
// strategy comparison would rather run all-Ruby (meta activation is
// unmodeled per item; the swap keeps the rest of the set as-is). It
// lands in the socket where the package keeps the most value, ties
// preferring a color mismatch it can fix.
func TestCarriedPrismaticStaysOnCandidate(t *testing.T) {
	d := testDatabase()
	tear := gem(7, "Nightmare Tear", db.GemPrismatic, db.QualityEpic, db.Agility, 10)
	tear.Unique = true
	d.Gems[7] = tear

	worn := item(90, db.SlotChest, db.ArmorMail, db.Agility, 40)
	worn.GemSockets = []db.GemColor{db.GemRed, db.GemBlue}
	cand := item(91, db.SlotChest, db.ArmorMail, db.Agility, 50)
	cand.GemSockets = []db.GemColor{db.GemRed, db.GemBlue}
	cand.SocketBonus[db.Agility] = 4
	d.Items[worn.ID], d.Items[cand.ID] = worn, cand

	s := New(d, testProfile())
	// unconstrained, the ignore strategy wins: two Rubies (80) beat
	// Ruby + Tear + bonus (40+20+8) — the Tear would be dropped
	if gems, _ := s.AssumedGems(cand); gems[1] != nil && gems[1].ID == 7 {
		t.Fatalf("test premise broken: the unconstrained assumption already keeps the Tear")
	}

	ws := []Worn{{Item: worn, Gems: []int{1, 7}, HasDetail: true}}
	ctx := s.SwapContextFor(ws, 0)
	gems, matched := s.AssumedGemsSwap(cand, ctx)
	if len(gems) != 2 || gems[1] == nil || gems[1].ID != 7 {
		t.Fatalf("assumed = %+v, want the carried Tear in the blue socket", gems)
	}
	if gems[0] == nil || gems[0].ID != 1 {
		t.Fatalf("red socket = %+v, want the Ruby kept", gems[0])
	}
	if !matched {
		t.Fatalf("Ruby(red) + Tear(blue) matches every socket: bonus must be active")
	}
	// and the ranking prices the carried package: 50 base + 20 ruby +
	// 10 tear + 4 bonus = 84 agi vs worn 40+20+10 (no bonus on it) = 70
	// → delta (84-70)×2 = 28
	ups := s.UpgradesWorn(ws, 0, 10, nil)
	if len(ups) == 0 || ups[0].Item.ID != cand.ID || ups[0].Delta != 28 {
		t.Fatalf("upgrades = %+v, want the candidate at +28", ups)
	}
}

// TestMarginalValue: the display-row valuation credits only the points
// that fit under the cap, given the rest-of-set totals.
func TestMarginalValue(t *testing.T) {
	s := New(testDatabase(), testProfile()) // hit weight 3, cap 100
	base := statsOf(map[db.Stat]float64{db.MeleeHit: 90})
	// 0 → 20 hit on top of 90: only 10 fit under the cap → 30
	if got := s.MarginalValue(base, []db.Stat{db.MeleeHit}, 0, 20); got != 30 {
		t.Errorf("near cap: marginal = %v, want 30", got)
	}
	// fully capped base: worthless either way
	base[db.MeleeHit] = 120
	if got := s.MarginalValue(base, []db.Stat{db.MeleeHit}, 0, 20); got != 0 {
		t.Errorf("past cap: marginal = %v, want 0", got)
	}
	// uncapped stat: plain weight × delta
	if got := s.MarginalValue(base, []db.Stat{db.Agility}, 10, 30); got != 40 {
		t.Errorf("uncapped: marginal = %v, want 40", got)
	}
}

// TestSoftCap: a StatCap with SoftWeight prices surplus at the residual
// slope instead of zeroing it — in Value and in MarginalValue. (No cata
// profile currently uses one — the sim showed SV's fabled 757 haste
// breakpoint doesn't exist — but the primitive stays for real
// breakpoint specs, e.g. resto druid HoT ticks.)
func TestSoftCap(t *testing.T) {
	p := testProfile()
	p.Weights[db.MeleeHaste] = 2.0
	p.Caps = append(p.Caps, StatCap{Stat: db.MeleeHaste, Cap: 100, SoftWeight: 0.5})
	s := New(testDatabase(), p)

	// 150 haste: 100 at full weight + 50 at the residual
	if got := s.Value(statsOf(map[db.Stat]float64{db.MeleeHaste: 150})); got != 100*2.0+50*0.5 {
		t.Fatalf("soft-capped value = %v, want 225", got)
	}
	// marginal +100 from base 50: 50 fit under the cap, 50 spill over
	base := statsOf(map[db.Stat]float64{db.MeleeHaste: 50})
	if got := s.MarginalValue(base, []db.Stat{db.MeleeHaste}, 0, 100); got != 50*2.0+50*0.5 {
		t.Fatalf("soft-capped marginal = %v, want 125", got)
	}
	// hard caps unchanged: surplus worthless
	if got := s.Value(statsOf(map[db.Stat]float64{db.MeleeHit: 150})); got != 100*3.0 {
		t.Fatalf("hard-capped value = %v, want 300", got)
	}
}

func TestCapAppliesToSetScore(t *testing.T) {
	d := testDatabase()
	a := item(20, db.SlotHead, db.ArmorMail, db.MeleeHit, 80)
	b := item(21, db.SlotChest, db.ArmorMail, db.MeleeHit, 80)
	d.Items[a.ID], d.Items[b.ID] = a, b

	s := New(d, testProfile())
	// 160 hit total, cap 100 → only 100 count at weight 3
	if got := s.SetScore([]*db.Item{a, b}); got != 300 {
		t.Fatalf("set score = %v, want 300 (capped)", got)
	}
	// b's contribution: without it the set has 80 hit (under cap), so it
	// only adds the 20 capped points → 60
	if got := s.Contribution([]*db.Item{a, b}, 1); got != 60 {
		t.Fatalf("contribution = %v, want 60 (cap-aware)", got)
	}
}

// TestUpgradeDeltaRespectsCap: equipped item has 50 hit and the set sits
// 10 points below the cap; a candidate with 70 hit is +20 on paper but
// only 10 fit under the cap — the delta must credit exactly those 10.
func TestUpgradeDeltaRespectsCap(t *testing.T) {
	d := testDatabase()
	item1 := item(50, db.SlotHead, db.ArmorMail, db.MeleeHit, 50)
	rest := item(51, db.SlotChest, db.ArmorMail, db.MeleeHit, 40) // set: 90/100
	item2 := item(52, db.SlotHead, db.ArmorMail, db.MeleeHit, 70)
	for _, it := range []*db.Item{item1, rest, item2} {
		d.Items[it.ID] = it
	}

	s := New(d, testProfile()) // hit weight 3.0, cap 100
	ups := s.Upgrades([]*db.Item{item1, rest}, 0, 10, nil)
	if len(ups) != 1 || ups[0].Item.ID != 52 {
		t.Fatalf("expected item2 as the only upgrade: %+v", ups)
	}
	// 10 usable points × weight 3 = 30, NOT the imaginary 20 × 3 = 60
	if ups[0].Delta != 30 {
		t.Fatalf("delta = %v, want 30 (cap-clipped)", ups[0].Delta)
	}

	// sanity: with an under-cap set the same swap credits the full +20
	low := item(53, db.SlotChest, db.ArmorMail, db.MeleeHit, 10) // set: 60/100
	d.Items[low.ID] = low
	ups = s.Upgrades([]*db.Item{item1, low}, 0, 10, nil)
	if len(ups) != 1 || ups[0].Delta != 60 {
		t.Fatalf("under-cap delta = %+v, want full 60", ups)
	}
}

func TestUpgradesRankingAndFilters(t *testing.T) {
	d := testDatabase()
	cur := item(30, db.SlotHead, db.ArmorMail, db.Agility, 50)
	better := item(31, db.SlotHead, db.ArmorMail, db.Agility, 80)
	best := item(32, db.SlotHead, db.ArmorMail, db.Agility, 100)
	worse := item(33, db.SlotHead, db.ArmorMail, db.Agility, 40)
	cloth := item(34, db.SlotHead, db.ArmorCloth, db.Agility, 200)
	wrongSlot := item(35, db.SlotChest, db.ArmorMail, db.Agility, 200)
	otherClass := item(36, db.SlotHead, db.ArmorMail, db.Agility, 200)
	otherClass.ClassAllowlist = []db.Class{db.ClassRogue}
	for _, it := range []*db.Item{cur, better, best, worse, cloth, wrongSlot, otherClass} {
		d.Items[it.ID] = it
	}

	s := New(d, testProfile())
	ups := s.Upgrades([]*db.Item{cur}, 0, 10, nil)
	if len(ups) != 2 {
		t.Fatalf("upgrades = %d, want exactly 2 (better+best): %+v", len(ups), ups)
	}
	if ups[0].Item.ID != 32 || ups[1].Item.ID != 31 {
		t.Fatalf("wrong order: got %d, %d", ups[0].Item.ID, ups[1].Item.ID)
	}
	if ups[0].Delta != 100 { // (100-50) agi * 2.0
		t.Fatalf("delta = %v, want 100", ups[0].Delta)
	}

	// the allow predicate pre-filters candidates
	onlyBetter := s.Upgrades([]*db.Item{cur}, 0, 10, func(it *db.Item) bool { return it.ID != 32 })
	if len(onlyBetter) != 1 || onlyBetter[0].Item.ID != 31 {
		t.Fatalf("filtered upgrades wrong: %+v", onlyBetter)
	}
}

func TestEffectiveCaps(t *testing.T) {
	p := &Profile{
		Caps:   []StatCap{{Stat: db.ArmorPenetration, Cap: 1400}, {Stat: db.Expertise, Cap: 214}},
		HitCap: &HitCapSpec{Stat: db.MeleeHit, BasePct: 8},
	}
	caps := p.EffectiveCaps(nil)
	if len(caps) != 3 || caps[2].Cap != 263 {
		t.Fatalf("base caps wrong: %+v", caps)
	}
	// 4% hit (3 talents + draenei) and 10 expertise reduce their caps;
	// ArP is untouched
	red := map[db.Stat]float64{
		db.MeleeHit:  4 * MeleeHitRatingPerPct,
		db.Expertise: 10 * RatingPerExpertise,
	}
	caps = p.EffectiveCaps(red)
	if got := caps[2].Cap; got < 131 || got > 132 {
		t.Fatalf("reduced hit cap = %v, want ≈131.8", got)
	}
	if got := caps[1].Cap; got < 132 || got > 133 {
		t.Fatalf("reduced expertise cap = %v, want ≈132", got)
	}
	if caps[0].Cap != 1400 {
		t.Fatalf("ArP cap must be untouched: %+v", caps[0])
	}
	// over-reduction clamps at zero
	caps = p.EffectiveCaps(map[db.Stat]float64{db.MeleeHit: 9999})
	if caps[2].Cap != 0 {
		t.Fatalf("clamped cap wrong: %+v", caps[2])
	}

	// spell hit converts at its own rate: 17% ≈ 446 rating, not 558
	caster := &Profile{HitCap: &HitCapSpec{Stat: db.SpellHit, BasePct: 17}}
	if got := caster.EffectiveCaps(nil)[0].Cap; got != 446 {
		t.Fatalf("spell hit cap = %v, want 446", got)
	}
}

func TestCapTalentTable(t *testing.T) {
	for class, talents := range map[db.Class]int{
		db.ClassHunter: 1, db.ClassRogue: 2, db.ClassPaladin: 1,
		db.ClassWarrior: 3, db.ClassDruid: 2, db.ClassDeathKnight: 2,
	} {
		if got := len(WotLK.CapTalentsFor(class)); got != talents {
			t.Errorf("talents for class %v = %d, want %d", class, got, talents)
		}
	}
	// rogue: hit talent + flat-expertise talent, correct rating values
	var prec, wexp *CapTalent
	for i := range WotLK.CapTalentsFor(db.ClassRogue) {
		t := &WotLK.CapTalentsFor(db.ClassRogue)[i]
		switch t.Name {
		case "Precision":
			prec = t
		case "Weapon Expertise":
			wexp = t
		}
	}
	if prec == nil || prec.Max != 5 || prec.Stat != db.MeleeHit || prec.RatingPerPoint != MeleeHitRatingPerPct {
		t.Errorf("rogue Precision wrong: %+v", prec)
	}
	if wexp == nil || wexp.Max != 2 || wexp.Stat != db.Expertise || wexp.RatingPerPoint != 5*RatingPerExpertise {
		t.Errorf("rogue Weapon Expertise wrong: %+v", wexp)
	}
	// caster hit talents convert at the spell rate
	bop := WotLK.CapTalentsFor(db.ClassDruid)[0]
	if bop.Stat != db.SpellHit || bop.RatingPerPoint != 2*SpellHitRatingPerPct {
		t.Errorf("Balance of Power wrong: %+v", bop)
	}
	if RacialHitPct("Draenei") != 1 || RacialHitPct("Troll") != 0 {
		t.Error("racial hit table wrong")
	}
}

// TestUniqueEquippedExclusion: an upgrade already worn in the paired slot
// must not be proposed when unique-equipped — rings/trinkets and the
// other hand when dual-wielding; non-unique duplicates stay legal.
func TestUniqueEquippedExclusion(t *testing.T) {
	d := testDatabase()
	ring1 := item(70, db.SlotFinger, db.ArmorAny, db.Agility, 40)
	uniqueRing := item(71, db.SlotFinger, db.ArmorAny, db.Agility, 100)
	uniqueRing.Unique = true
	plainRing := item(72, db.SlotFinger, db.ArmorAny, db.Agility, 90)
	mh := item(73, db.SlotWeapon, db.ArmorAny, db.Agility, 40)
	uniqueBlade := item(74, db.SlotWeapon, db.ArmorAny, db.Agility, 100)
	uniqueBlade.Unique = true
	for _, it := range []*db.Item{ring1, uniqueRing, plainRing, mh, uniqueBlade} {
		d.Items[it.ID] = it
	}

	s := New(d, testProfile())

	// wearing the unique ring in finger 2: it must not be proposed for
	// finger 1, while the non-unique ring still is
	set := []*db.Item{ring1, uniqueRing}
	ups := s.Upgrades(set, 0, 10, nil)
	for _, u := range ups {
		if u.Item.ID == uniqueRing.ID {
			t.Fatalf("unique ring proposed while already worn: %+v", ups)
		}
	}
	if len(ups) == 0 || ups[0].Item.ID != plainRing.ID {
		t.Fatalf("non-unique ring should still be proposed: %+v", ups)
	}

	// non-unique duplicates are legal: wearing plainRing in finger 2
	// does not block proposing another copy for finger 1
	ups = s.Upgrades([]*db.Item{ring1, plainRing}, 0, 10, nil)
	found := false
	for _, u := range ups {
		found = found || u.Item.ID == plainRing.ID
	}
	if !found {
		t.Fatalf("non-unique duplicate wrongly excluded: %+v", ups)
	}

	// dual wield: unique blade in the off-hand blocks it for the main hand
	ups = s.Upgrades([]*db.Item{mh, uniqueBlade}, 0, 10, nil)
	for _, u := range ups {
		if u.Item.ID == uniqueBlade.ID {
			t.Fatalf("unique weapon proposed while in the other hand: %+v", ups)
		}
	}
}

// TestUniqueGroupExclusion: only one Ashen Verdict / Kirin Tor band may
// be worn — a different band of the same group must not be proposed for
// the other finger, but upgrading the group ring's own slot is legal.
func TestUniqueGroupExclusion(t *testing.T) {
	d := testDatabase()
	plain := item(80, db.SlotFinger, db.ArmorAny, db.Agility, 40)
	ashenVeng := item(50401, db.SlotFinger, db.ArmorAny, db.Agility, 60) // Ashen Verdict group
	ashenBetter := item(50402, db.SlotFinger, db.ArmorAny, db.Agility, 100)
	kirin := item(48954, db.SlotFinger, db.ArmorAny, db.Agility, 90) // Kirin Tor group
	for _, it := range []*db.Item{plain, ashenVeng, ashenBetter, kirin} {
		d.Items[it.ID] = it
	}
	s := New(d, testProfile())

	// finger 1 = plain, finger 2 = an Ashen band: no other Ashen band may
	// be proposed for finger 1; the Kirin Tor ring is fine
	ups := s.Upgrades([]*db.Item{plain, ashenVeng}, 0, 10, nil)
	for _, u := range ups {
		if d.UniqueGroup(u.Item.ID) == "Ashen Verdict band" {
			t.Fatalf("second Ashen band proposed: %+v", ups)
		}
	}
	if len(ups) == 0 || ups[0].Item.ID != kirin.ID {
		t.Fatalf("kirin tor ring should be proposed: %+v", ups)
	}

	// upgrading the Ashen slot itself to a better Ashen band stays legal
	ups = s.Upgrades([]*db.Item{plain, ashenVeng}, 1, 10, nil)
	found := false
	for _, u := range ups {
		found = found || u.Item.ID == ashenBetter.ID
	}
	if !found {
		t.Fatalf("upgrading own Ashen slot must remain legal: %+v", ups)
	}
}

func TestProfileRegistry(t *testing.T) {
	if got := len(profiles); got != 30 {
		t.Fatalf("profiles = %d, want 30 (every tree of every class)", got)
	}
	if Find(db.ClassHunter, "Boomkin") != nil {
		t.Error("unknown tree should return nil")
	}

	seen := map[string]bool{}
	for _, p := range profiles {
		if p.Name == "" || p.Tree == "" || p.Class == db.ClassUnknown {
			t.Errorf("incomplete profile: %+v", p)
		}
		if seen[p.Name] {
			t.Errorf("duplicate profile name %q", p.Name)
		}
		seen[p.Name] = true
		// weights come from the WoWSims sim defaults; hybrid sims weight
		// BOTH sides of a duplicated rating on purpose (the rating's full
		// value = the sum of what each side contributes), so there is no
		// both-sides prohibition anymore — just require SOME weights
		nonzero := false
		for _, v := range p.Weights {
			if v != 0 {
				nonzero = true
				break
			}
		}
		if !nonzero {
			t.Errorf("%s has no weights at all", p.Name)
		}
		// hit caps must gate a side the profile actually weights (or the
		// profile is a healer with no hit cap at all)
		if p.HitCap != nil && p.Weights[p.HitCap.Stat] == 0 {
			t.Errorf("%s hit cap on unweighted stat %v", p.Name, p.HitCap.Stat)
		}
		if len(p.WeaponTypes) == 0 || len(p.RangedTypes) == 0 {
			t.Errorf("%s missing weapon/ranged proposal filters", p.Name)
		}
		if p.ArmorType == db.ArmorAny {
			t.Errorf("%s missing armor type", p.Name)
		}
	}

	// fill-to-cap floor invariant (session 14 fury bug): a melee-DPS
	// profile running on a GUESSED expertise floor must weight expertise
	// above its top gemable stat, or the gem optimizer strips it below
	// the dodge cap. Specs whose pre-cap slope has been MEASURED are
	// exempt, and as of session 35 that is every WotLK melee DPS spec —
	// the sim is allowed to say a spec does not want the cap (frost DK
	// reads 1.29 against Strength 3.22, ret 1.60 against 2.53, because
	// their damage leans on effects that cannot be dodged), and a guess
	// must never override a measurement. The check stays for the next
	// profile someone adds on a floor. Tank profiles (Defense cap) value
	// expertise for smoothing, not cap-seeking — excluded on purpose.
	measured := map[string]bool{
		"Fury Warrior": true, "Arms Warrior": true,
		"Assassination Rogue": true, "Combat Rogue": true, "Subtlety Rogue": true,
		"Blood Death Knight": true, "Frost Death Knight": true, "Unholy Death Knight": true,
		"Retribution Paladin": true, "Enhancement Shaman": true, "Feral Druid": true,
	}
	for _, p := range profiles {
		if measured[p.Name] {
			continue
		}
		var hasExp, hasDef bool
		for _, c := range p.Caps {
			hasExp = hasExp || c.Stat == db.Expertise
			hasDef = hasDef || c.Stat == db.Defense
		}
		if !hasExp || hasDef {
			continue
		}
		top := max(p.Weights[db.Strength], p.Weights[db.Agility])
		if p.Weights[db.Expertise] <= top {
			t.Errorf("%s expertise %.2f not above top gemable stat %.2f — cap-seeking floor missing",
				p.Name, p.Weights[db.Expertise], top)
		}
	}

	// spot checks: casters gate spell hit at 17%, physical melee at 8%
	if p := Find(db.ClassMage, "Fire"); p.HitCap.Stat != db.SpellHit || p.HitCap.BasePct != 17 {
		t.Error("fire mage hit cap wrong")
	}
	if p := Find(db.ClassDruid, "Feral Combat"); p.HitCap.Stat != db.MeleeHit || p.ArmorType != db.ArmorLeather {
		t.Error("feral druid profile wrong")
	}
	if p := Find(db.ClassPaladin, "Holy"); p.HitCap != nil {
		t.Error("healers have no hit cap")
	}
}

func TestWornStats(t *testing.T) {
	d := testDatabase()
	d.Gems[7] = gem(7, "Delicate Cardinal Ruby", db.GemRed, db.QualityEpic, db.Agility, 20)
	d.Enchants = []*db.Enchant{func() *db.Enchant {
		e := &db.Enchant{EffectID: 3817, Name: "Test Scope"}
		e.Stats[db.MeleeHaste] = 40
		return e
	}()}
	it := item(90, db.SlotHead, db.ArmorMail, db.Agility, 60)
	it.GemSockets = []db.GemColor{db.GemRed, db.GemYellow}
	it.SocketBonus[db.Agility] = 6
	d.Items[it.ID] = it

	s := New(d, testProfile())

	// no detail → assumption-based (best gems + bonus): 60 + 20 + 20 + 6
	// (best red matches yellow? no — yellow socket takes the red gem only
	// if it matches... red gem doesn't match yellow, so assumption picks
	// per-color best; only the red socket has a candidate here)
	assumed := s.WornStats(Worn{Item: it})
	if assumed != s.EffectiveStats(it) {
		t.Fatalf("detail-less worn must equal assumptions")
	}

	// actual: one real gem, one EMPTY socket → no socket bonus, and the
	// real enchant counts
	w := Worn{Item: it, Gems: []int{7, 0}, EnchantID: 3817, HasDetail: true}
	got := s.WornStats(w)
	if got[db.Agility] != 80 { // 60 base + 20 gem, NO +6 bonus (empty socket)
		t.Fatalf("worn agility = %v, want 80", got[db.Agility])
	}
	if got[db.MeleeHaste] != 40 {
		t.Fatalf("worn enchant haste = %v, want 40", got[db.MeleeHaste])
	}

	// filling both sockets with matching colors activates the bonus
	d.Gems[8] = gem(8, "Rigid King's Amber", db.GemYellow, db.QualityEpic, db.MeleeHit, 20)
	w2 := Worn{Item: it, Gems: []int{7, 8}, HasDetail: true}
	if got := s.WornStats(w2)[db.Agility]; got != 86 { // 60+20+6
		t.Fatalf("matched bonus agility = %v, want 86", got)
	}

	// extra gem beyond sockets (belt buckle) still counts
	w3 := Worn{Item: it, Gems: []int{7, 8, 7}, HasDetail: true}
	if got := s.WornStats(w3)[db.Agility]; got != 106 { // +20 more
		t.Fatalf("buckle gem agility = %v, want 106", got)
	}
}

func TestWeaponDPSValue(t *testing.T) {
	d := testDatabase()
	prof := testProfile()
	prof.RangedDPSWeight = 5.3

	slow := item(95, db.SlotRanged, db.ArmorAny, db.Agility, 10)
	slow.WeaponDmgMin, slow.WeaponDmgMax, slow.WeaponSpeed = 500, 700, 3.0 // 200 dps
	fast := item(96, db.SlotRanged, db.ArmorAny, db.Agility, 40)
	fast.WeaponDmgMin, fast.WeaponDmgMax, fast.WeaponSpeed = 300, 420, 2.0 // 180 dps
	for _, it := range []*db.Item{slow, fast} {
		d.Items[it.ID] = it
	}
	s := New(d, prof)

	if got := WeaponDPS(slow); got != 200 {
		t.Fatalf("dps = %v, want 200", got)
	}
	// slow: 10 agi*2 + 200 dps*5.3 = 1080; fast: 40*2 + 180*5.3 = 1034 —
	// the higher-DPS weapon wins despite 30 less agility
	if a, b := s.SetScore([]*db.Item{slow}), s.SetScore([]*db.Item{fast}); a <= b {
		t.Fatalf("dps must dominate: slow=%v fast=%v", a, b)
	}
	// and upgrades credit the dps difference: replacing fast with slow
	ups := s.Upgrades([]*db.Item{fast}, 0, 10, nil)
	if len(ups) != 1 || ups[0].Item.ID != slow.ID {
		t.Fatalf("expected slow weapon as upgrade: %+v", ups)
	}
	if d := ups[0].Delta; d < 45 || d > 47 { // 20*5.3 - 30*2 = 106-60 = 46
		t.Fatalf("delta = %v, want ≈46", d)
	}
	// contribution includes weapon value
	if c := s.Contribution([]*db.Item{slow}, 0); c < 1079 || c > 1081 {
		t.Fatalf("contribution = %v, want ≈1080", c)
	}
}

func TestWeaponRacials(t *testing.T) {
	d := testDatabase()
	prof := testProfile()
	prof.Weights[db.MeleeCrit] = 2.0
	prof.RangedDPSWeight = 5.0

	bow := item(97, db.SlotRanged, db.ArmorAny, db.Agility, 50)
	bow.RangedType = db.RangedBow
	bow.WeaponDmgMin, bow.WeaponDmgMax, bow.WeaponSpeed = 200, 300, 2.5 // 100 dps
	gun := item(98, db.SlotRanged, db.ArmorAny, db.Agility, 50)
	gun.RangedType = db.RangedGun
	gun.WeaponDmgMin, gun.WeaponDmgMax, gun.WeaponSpeed = 200, 300, 2.5 // identical
	for _, it := range []*db.Item{bow, gun} {
		d.Items[it.ID] = it
	}

	s := New(d, prof)
	// raceless: identical weapons tie
	if a, b := s.SetScore([]*db.Item{bow}), s.SetScore([]*db.Item{gun}); a != b {
		t.Fatalf("raceless tie broken: bow=%v gun=%v", a, b)
	}
	// troll: the bow gains ~46 crit rating (1%) → wins by ~92 EP
	s.SetRace("Troll")
	a, b := s.SetScore([]*db.Item{bow}), s.SetScore([]*db.Item{gun})
	if diff := a - b; diff < 90 || diff > 94 {
		t.Fatalf("troll bow edge = %v, want ≈91.8", diff)
	}
	// dwarf: the gun wins instead
	s.SetRace("Dwarf")
	if a, b := s.SetScore([]*db.Item{bow}), s.SetScore([]*db.Item{gun}); a >= b {
		t.Fatalf("dwarf must prefer the gun: bow=%v gun=%v", a, b)
	}

	// orc axe expertise counts toward the expertise cap
	prof2 := testProfile()
	prof2.Weights[db.Expertise] = 2.0
	prof2.Caps = append(prof2.Caps, StatCap{Stat: db.Expertise, Cap: 30})
	axe := item(99, db.SlotWeapon, db.ArmorAny, db.Agility, 10)
	axe.WeaponType = db.WeaponAxe
	d.Items[axe.ID] = axe
	s2 := New(d, prof2)
	s2.SetRace("Orc")
	// 5 expertise ≈ 41 rating, capped at 30 → 30×2 = 60 from the racial
	got := s2.SetScore([]*db.Item{axe})
	if got != 10*2+30*2 {
		t.Fatalf("orc axe score = %v, want 80 (capped racial expertise)", got)
	}
	if WotLK.RacialWeaponNote("Orc", axe) == "" || WotLK.RacialWeaponNote("Troll", axe) != "" {
		t.Fatal("racial notes wrong")
	}
}

func TestGemAndEnchantTips(t *testing.T) {
	d := testDatabase() // has Delicate Cardinal Ruby (id 1, +20 agi epic)
	ench := &db.Enchant{EffectID: 900, Name: "Greater Agility", Slot: db.SlotHead}
	ench.Stats[db.Agility] = 30
	weak := &db.Enchant{EffectID: 901, Name: "Lesser Agility", Slot: db.SlotHead}
	weak.Stats[db.Agility] = 8
	furLining := &db.Enchant{EffectID: 902, Name: "Fur Lining - Attack Power", Slot: db.SlotHead}
	furLining.Stats[db.Agility] = 99 // would win, but is LW-only
	d.Enchants = []*db.Enchant{ench, weak, furLining}

	it := item(70, db.SlotHead, db.ArmorMail, db.Agility, 60)
	it.GemSockets = []db.GemColor{db.GemRed, db.GemRed}
	it.SocketBonus[db.Agility] = 6
	d.Items[it.ID] = it

	s := New(d, testProfile()) // agi weight 2.0
	// worn: bad gem in socket 0 (rare 16 agi), socket 1 EMPTY, weak enchant
	worn := []Worn{{Item: it, Gems: []int{2, 0}, EnchantID: 901, HasDetail: true}}

	gems := s.GemTips(worn)
	if len(gems) != 1 {
		t.Fatalf("gem tips = %d, want 1 package tip: %+v", len(gems), gems)
	}
	// optimal package: epic agi gem in both red sockets, bonus kept.
	// worn contributes 16 agi (rare gem, empty socket, no bonus);
	// recommended 20+20+6 = 46 → delta 30 agi × 2.0 = 60
	tip := gems[0]
	if !tip.Matched || len(tip.Changes) != 2 {
		t.Fatalf("package tip wrong: %+v", tip)
	}
	if tip.Changes[0].To.ID != 1 || tip.Changes[1].To.ID != 1 {
		t.Fatalf("changes wrong: %+v", tip.Changes)
	}
	if tip.Delta != 60 {
		t.Fatalf("package delta = %v, want 60", tip.Delta)
	}

	// enchant: without LW, best is Greater Agility (+22 agi over current)
	enchs := s.EnchantTips(worn)
	if len(enchs) != 1 || enchs[0].To.EffectID != 900 || enchs[0].Delta != 44 {
		t.Fatalf("enchant tip wrong: %+v", enchs)
	}
	// with LW, Fur Lining wins
	s.SetProfessions(map[string]bool{"Leatherworking": true})
	enchs = s.EnchantTips(worn)
	if len(enchs) != 1 || enchs[0].To.EffectID != 902 {
		t.Fatalf("profession enchant tip wrong: %+v", enchs)
	}
	s.SetProfessions(nil)

	// fully optimized: no tips
	perfect := []Worn{{Item: it, Gems: []int{1, 1}, EnchantID: 900, HasDetail: true}}
	if g, e := s.GemTips(perfect), s.EnchantTips(perfect); len(g) != 0 || len(e) != 0 {
		t.Fatalf("optimized gear must yield no tips: %+v %+v", g, e)
	}

	// candidates are scored WITH the assumed best enchant, mirroring the
	// enriched worn side that counts its real enchant
	if got := s.EffectiveStats(it)[db.Agility]; got != 60+20+20+6+30 {
		t.Fatalf("candidate effective agility = %v, want gear+gems+bonus+enchant = 136", got)
	}
}

func TestIgnoreWeakSocketBonus(t *testing.T) {
	d := testDatabase() // red epic agi gem (id 1, +20 agi)
	// a yellow gem exists but is weak for this profile (hit weighted 3,
	// but give it a poor stat): +20 stamina
	d.Gems[9] = gem(9, "Solid King's Amber", db.GemYellow, db.QualityEpic, db.Stamina, 20)

	// item A: yellow+blue sockets with a WEAK bonus (+4 stamina) — raw
	// agility gems in both sockets beat matching
	weak := item(80, db.SlotHead, db.ArmorMail, db.Agility, 10)
	weak.GemSockets = []db.GemColor{db.GemYellow, db.GemBlue}
	weak.SocketBonus[db.Stamina] = 4
	// item B: red socket with a STRONG bonus (+8 agility) — matching wins
	strong := item(81, db.SlotChest, db.ArmorMail, db.Agility, 10)
	strong.GemSockets = []db.GemColor{db.GemRed}
	strong.SocketBonus[db.Agility] = 8
	d.Items[weak.ID], d.Items[strong.ID] = weak, strong

	s := New(d, testProfile()) // agi 2.0, stam ~0

	gems, matched := s.AssumedGems(weak)
	if matched {
		t.Fatalf("weak bonus must be skipped: %+v", gems)
	}
	if gems[0].ID != 1 || gems[1].ID != 1 {
		t.Fatalf("ignore package must use the best raw gem: %+v", gems)
	}
	// GearStats: 10 base + 2×20 gem agi, NO bonus
	if got := s.GearStats(weak)[db.Agility]; got != 50 {
		t.Fatalf("weak-bonus agility = %v, want 50", got)
	}
	if got := s.GearStats(weak)[db.Stamina]; got != 0 {
		t.Fatalf("skipped bonus must not add stamina: %v", got)
	}

	gems, matched = s.AssumedGems(strong)
	if !matched || gems[0].ID != 1 {
		t.Fatalf("strong bonus must be kept: %+v matched=%v", gems, matched)
	}
	// 10 base + 20 gem + 8 bonus
	if got := s.GearStats(strong)[db.Agility]; got != 38 {
		t.Fatalf("strong-bonus agility = %v, want 38", got)
	}
}

func TestMetaGuardsInGemTips(t *testing.T) {
	d := testDatabase()
	nt := gem(49110, "Nightmare Tear", db.GemPrismatic, db.QualityEpic, db.Agility, 10)
	d.Gems[nt.ID] = nt

	it := item(85, db.SlotHead, db.ArmorMail, db.Agility, 10)
	it.GemSockets = []db.GemColor{db.GemBlue, db.GemBlue}
	// no bonus: the ignore package (2x epic agi gem) always wins raw
	d.Items[it.ID] = it
	s := New(d, testProfile())

	// the worn Nightmare Tear must survive the recommendation: the only
	// change is filling the empty socket with the raw agility gem
	worn := []Worn{{Item: it, Gems: []int{nt.ID, 0}, HasDetail: true}}
	tips := s.GemTips(worn)
	if len(tips) != 1 || len(tips[0].Changes) != 1 {
		t.Fatalf("tips = %+v", tips)
	}
	ch := tips[0].Changes[0]
	if ch.SocketIdx != 1 || ch.From != nil || ch.To.ID != 1 {
		t.Fatalf("expected only an empty-socket fill: %+v", ch)
	}

	// a lesser prismatic (Enchanted Tear) is upgraded WITHIN the
	// prismatic family, never swapped for a colored gem
	lesser := gem(42702, "Enchanted Tear", db.GemPrismatic, db.QualityRare, db.Agility, 6)
	d.Gems[lesser.ID] = lesser
	worn2 := []Worn{{Item: it, Gems: []int{lesser.ID, 1}, HasDetail: true}}
	tips = s.GemTips(worn2)
	if len(tips) != 1 || len(tips[0].Changes) != 1 {
		t.Fatalf("prismatic upgrade tips = %+v", tips)
	}
	ch = tips[0].Changes[0]
	if ch.From == nil || ch.From.ID != lesser.ID || ch.To.ID != nt.ID {
		t.Fatalf("expected Enchanted Tear → Nightmare Tear: %+v", ch)
	}

	// meta activation check: Relentless (one-of-each rule, id 41398) +
	// only red gems → likely inactive; adding the prismatic satisfies
	// all colors
	meta := gem(41398, "Relentless Earthsiege Diamond", db.GemMeta, db.QualityRare, db.Agility, 21)
	d.Gems[meta.ID] = meta
	broken := []Worn{{Item: it, Gems: []int{meta.ID, 1}, HasDetail: true}}
	if g, cond := s.InactiveMeta(broken); g == nil || !cond.OneOfEach() {
		t.Fatalf("meta with only red gems must be flagged inactive (got %v, %v)", g, cond)
	}
	fixed := []Worn{{Item: it, Gems: []int{meta.ID, nt.ID}, HasDetail: true}}
	if g, _ := s.InactiveMeta(fixed); g != nil {
		t.Fatal("prismatic satisfies all colors")
	}
	if g, _ := s.InactiveMeta(worn); g != nil {
		t.Fatal("no meta equipped → nothing to flag")
	}
}

func TestMetaConditionPerGem(t *testing.T) {
	d := testDatabase()
	csd := gem(41285, "Chaotic Skyflare Diamond", db.GemMeta, db.QualityRare, db.Agility, 21)
	purple := gem(40034, "Guardian's Twilight Opal", db.GemPurple, db.QualityRare, db.Agility, 10)
	d.Gems[csd.ID] = csd
	d.Gems[purple.ID] = purple

	it := item(86, db.SlotHead, db.ArmorMail, db.Agility, 10)
	it.GemSockets = []db.GemColor{db.GemMeta, db.GemBlue, db.GemBlue}
	d.Items[it.ID] = it
	s := New(d, testProfile())

	// CSD needs 2 blue-counting gems: two purples activate it even with
	// zero yellow gems (the fury-warrior false-positive case)
	active := []Worn{{Item: it, Gems: []int{csd.ID, purple.ID, purple.ID}, HasDetail: true}}
	if g, _ := s.InactiveMeta(active); g != nil {
		t.Fatalf("CSD with 2 purples must count as active, flagged %v", g)
	}
	// one purple (1 blue) is not enough
	broken := []Worn{{Item: it, Gems: []int{csd.ID, purple.ID, 0}, HasDetail: true}}
	g, cond := s.InactiveMeta(broken)
	if g == nil || g.ID != csd.ID {
		t.Fatalf("CSD with 1 blue must be flagged, got %v", g)
	}
	if cond.OneOfEach() || cond.String() != "at least 2 blue gems" {
		t.Fatalf("condition = %q", cond.String())
	}
}

func TestGemTieBreakDeterministic(t *testing.T) {
	d := testDatabase() // Delicate Cardinal Ruby (id 1, +20 agi epic)
	// identical stats, higher ID (Stormjewel-style rarity)
	d.Gems[45879] = gem(45879, "Delicate Stormjewel", db.GemRed, db.QualityEpic, db.Agility, 20)

	// run several times: map iteration order must never flip the pick
	for range 20 {
		s := New(d, testProfile())
		if g := s.BestGem(db.GemRed); g == nil || g.ID != 1 {
			t.Fatalf("tie must break to the lower ID (standard cut), got %+v", g)
		}
		if s.bestAny == nil || s.bestAny.ID != 1 {
			t.Fatalf("bestAny tie-break wrong: %+v", s.bestAny)
		}
	}
}

func TestOptimizeGemsHitBalancing(t *testing.T) {
	d := testDatabase()
	// pure hit yellow, hybrid orange (10 agi + 10 hit)
	rigid := gem(50, "Rigid King's Amber", db.GemYellow, db.QualityEpic, db.MeleeHit, 20)
	d.Gems[rigid.ID] = rigid
	glint := gem(51, "Glinting Ametrine", db.GemOrange, db.QualityEpic, db.Agility, 10)
	glint.Stats[db.MeleeHit] = 10
	d.Gems[glint.ID] = glint

	mk := func(baseHit float64) []Worn {
		chest := item(60, db.SlotChest, db.ArmorMail, db.MeleeHit, baseHit)
		chest.GemSockets = []db.GemColor{db.GemYellow}
		d.Items[chest.ID] = chest
		return []Worn{{Item: chest, Gems: []int{1}, HasDetail: true}} // agi gem socketed
	}
	s := New(d, testProfile()) // hit weight 3.0 > agi 2.0, cap 100

	// far below cap (base 40): pure hit gem wins the yellow socket
	plan := s.OptimizeGems(mk(40))
	if len(plan.Tips) != 1 || plan.Tips[0].Changes[0].To.ID != rigid.ID {
		t.Fatalf("under cap must socket pure hit: %+v", plan)
	}

	// 10 short of cap (base 90): the hybrid caps exactly — pure hit
	// would waste 10, pure agility would leave the cap unmet
	plan = s.OptimizeGems(mk(90))
	if len(plan.Tips) != 1 || plan.Tips[0].Changes[0].To.ID != glint.ID {
		t.Fatalf("near cap must socket the hybrid: %+v", plan)
	}
	// exact delta: -20 agi (+10 back) +10 counted hit = -10*2 + 10*3 = +10
	if plan.Delta < 9.9 || plan.Delta > 10.1 {
		t.Fatalf("plan delta = %v, want 10", plan.Delta)
	}

	// at cap (base 100): hit gems are worthless, keep the agility gem
	plan = s.OptimizeGems(mk(100))
	if len(plan.Tips) != 0 {
		t.Fatalf("at cap nothing should change: %+v", plan)
	}
}

func TestOptimizeGemsExpertiseCapSeeking(t *testing.T) {
	d := testDatabase()
	// three blue candidates: a nominally heavier Strength gem, an
	// Expertise gem, and a hit gem. Expertise is floored ABOVE Strength,
	// so it must fill to its cap and then STOP — neither be stripped for
	// Strength (the old bug) nor overshoot the cap.
	str := gem(80, "Bold Cardinal Ruby", db.GemBlue, db.QualityEpic, db.Strength, 20)
	exp := gem(81, "Guardian's Dreadstone", db.GemBlue, db.QualityEpic, db.Expertise, 20)
	hitg := gem(82, "Rigid King's Amber", db.GemBlue, db.QualityEpic, db.MeleeHit, 20)
	for _, g := range []*db.Gem{str, exp, hitg} {
		d.Gems[g.ID] = g
	}
	prof := &Profile{
		Name: "Test Fury", Class: db.ClassWarrior, Tree: "Fury",
		Weights: statsOf(map[db.Stat]float64{
			db.Strength: 2.0, db.Expertise: 3.0, db.MeleeHit: 1.0,
		}),
		Caps:      []StatCap{{Stat: db.MeleeHit, Cap: 10}, {Stat: db.Expertise, Cap: 20}},
		HitCap:    &HitCapSpec{Stat: db.MeleeHit, BasePct: 8},
		ArmorType: db.ArmorPlate,
	}
	s := New(d, prof)

	// item already hit-capped (10) so the hit dimension is inert; two blue
	// sockets, worn with two Strength gems — expertise at 0, below its cap
	chest := item(90, db.SlotChest, db.ArmorPlate, db.MeleeHit, 10)
	chest.GemSockets = []db.GemColor{db.GemBlue, db.GemBlue}
	d.Items[chest.ID] = chest
	ws := []Worn{{Item: chest, Gems: []int{str.ID, str.ID}, HasDetail: true}}

	after := s.TotalsWorn(s.OptimizeGems(ws).Set)
	// best assignment is {Str, Expertise}: 2.0*20 + 3.0*20 = 100, beating
	// {Str,Str}=80 and {Exp,Exp}=60 (only 20 expertise counts past the cap)
	if after[db.Expertise] != 20 {
		t.Fatalf("expertise = %v, want 20 (filled to cap, no overshoot)", after[db.Expertise])
	}
	if after[db.Strength] != 20 {
		t.Fatalf("strength = %v, want 20 (second socket kept for Str past the exp cap)", after[db.Strength])
	}
}

func TestOptimizeGemsKeepsMetaActive(t *testing.T) {
	d := testDatabase()
	nt := gem(52, "Nightmare Tear", db.GemPrismatic, db.QualityEpic, db.Agility, 10)
	d.Gems[nt.ID] = nt
	csd := gem(53, "Chaotic Skyflare Diamond", db.GemMeta, db.QualityRare, db.Agility, 21)
	d.Gems[csd.ID] = csd
	// the test meta's rule is registered explicitly (fresh map, not the
	// shared WotLK table): unlisted metas are no longer assumed
	// one-of-each — they run unconstrained and get flagged instead
	d.Rules.MetaConditions = map[int]db.MetaCondition{53: {MinRed: 1, MinYellow: 1, MinBlue: 1}}

	helm := item(61, db.SlotHead, db.ArmorMail, db.Agility, 10)
	helm.GemSockets = []db.GemColor{db.GemMeta, db.GemRed, db.GemRed}
	d.Items[helm.ID] = helm
	s := New(d, testProfile())

	// meta + two agility gems: pure-value optimum would be 2x Cardinal
	// Ruby (all red), but the one-of-each rule then fails; the
	// optimizer must keep a prismatic in one socket
	ws := []Worn{{Item: helm, Gems: []int{53, 1, 52}, HasDetail: true}}
	plan := s.OptimizeGems(ws)
	for _, tip := range plan.Tips {
		for _, ch := range tip.Changes {
			if ch.From != nil && ch.From.ID == nt.ID {
				t.Fatalf("optimizer must not drop the meta-activating prismatic: %+v", plan)
			}
		}
	}
}

func TestOptimizeGemsStableAndMinimal(t *testing.T) {
	d := testDatabase()
	// a second epic red gem with identical stats: every assignment of
	// the two across two red sockets ties exactly
	d.Gems[45879] = gem(45879, "Delicate Stormjewel", db.GemRed, db.QualityEpic, db.Agility, 20)
	bracer := item(70, db.SlotWrist, db.ArmorMail, db.Agility, 10)
	bracer.GemSockets = []db.GemColor{db.GemRed, db.GemRed}
	d.Items[bracer.ID] = bracer
	s := New(d, testProfile())

	// currently socketed: one of each — already optimal; the plan must
	// propose NOTHING, every run (ties break toward fewest changes, and
	// map iteration order must not decide)
	ws := []Worn{{Item: bracer, Gems: []int{45879, 1}, HasDetail: true}}
	for range 30 {
		if plan := s.OptimizeGems(ws); len(plan.Tips) != 0 {
			t.Fatalf("tied optimum must keep current gems: %+v", plan.Tips)
		}
	}
}

func TestBeltBuckleAssumption(t *testing.T) {
	d := testDatabase()
	belt := item(80, db.SlotWaist, db.ArmorMail, db.Agility, 50)
	belt.GemSockets = []db.GemColor{db.GemRed}
	belt.SocketBonus[db.Agility] = 4
	bare := item(81, db.SlotWaist, db.ArmorMail, db.Agility, 50) // no sockets at all
	head := item(82, db.SlotHead, db.ArmorMail, db.Agility, 50)  // control: not a belt
	head.GemSockets = []db.GemColor{db.GemRed}
	head.SocketBonus[db.Agility] = 4
	for _, it := range []*db.Item{belt, bare, head} {
		d.Items[it.ID] = it
	}
	s := New(d, testProfile())

	// candidates and detail-less worn belts assume the buckle: one
	// Cardinal Ruby (+20 agi) more than the same item on another slot
	if got := s.EffectiveStats(belt)[db.Agility]; got != 94 { // 50+20+4 +20 buckle
		t.Errorf("belt effective agility = %v, want 94 (buckle gem included)", got)
	}
	if got := s.EffectiveStats(head)[db.Agility]; got != 74 { // 50+20+4, no buckle
		t.Errorf("head effective agility = %v, want 74 (no buckle off-waist)", got)
	}
	// a socketless belt still gets the buckle socket (and no bonus)
	if got := s.EffectiveStats(bare)[db.Agility]; got != 70 {
		t.Errorf("socketless belt effective agility = %v, want 70", got)
	}
	// enriched worn belt WITHOUT the buckle scores bare — the asymmetry
	// that drives SocketTips (the armory pads the gem list with zeros,
	// so trailing zeros must not count as a buckle)
	w := Worn{Item: belt, Gems: []int{1, 0, 0}, HasDetail: true}
	if got := s.WornStats(w)[db.Agility]; got != 74 {
		t.Errorf("bare worn belt agility = %v, want 74", got)
	}
	if HasExtraGem(w) {
		t.Error("zero-padded gem list must not count as a buckle")
	}
	// ...and WITH it the extra gem counts
	wb := Worn{Item: belt, Gems: []int{1, 1, 0}, HasDetail: true}
	if got := s.WornStats(wb)[db.Agility]; got != 94 {
		t.Errorf("buckled worn belt agility = %v, want 94", got)
	}
	if !HasExtraGem(wb) {
		t.Error("extra gem beyond sockets is the buckle")
	}
}

func TestBuckleTips(t *testing.T) {
	d := testDatabase()
	belt := item(80, db.SlotWaist, db.ArmorMail, db.Agility, 50)
	belt.GemSockets = []db.GemColor{db.GemRed}
	head := item(82, db.SlotHead, db.ArmorMail, db.Agility, 50)
	head.GemSockets = []db.GemColor{db.GemRed}
	d.Items[belt.ID], d.Items[head.ID] = belt, head
	s := New(d, testProfile())

	ws := []Worn{
		{Item: belt, Gems: []int{1, 0, 0}, HasDetail: true},
		{Item: head, Gems: []int{1, 0, 0}, HasDetail: true},
	}
	tips := s.SocketTips(ws)
	if len(tips) != 1 || tips[0].ItemIdx != 0 || tips[0].Gem.ID != 1 {
		t.Fatalf("want one buckle tip, for the belt, with Cardinal Ruby: %+v", tips)
	}
	if tips[0].Delta != 40 { // 20 agi × weight 2.0
		t.Errorf("buckle tip delta = %v, want 40", tips[0].Delta)
	}
	// buckled belt: nothing to suggest
	ws[0].Gems = []int{1, 1, 0}
	if tips := s.SocketTips(ws); len(tips) != 0 {
		t.Fatalf("buckled belt must yield no tip: %+v", tips)
	}
	// detail-less import: no tip (already scored under the assumption)
	if tips := s.SocketTips([]Worn{{Item: belt}}); len(tips) != 0 {
		t.Fatalf("detail-less import must yield no tip: %+v", tips)
	}
}

func TestGemTipsBuckleSocket(t *testing.T) {
	d := testDatabase()
	weak := gem(54, "Weak Ruby", db.GemRed, db.QualityEpic, db.Agility, 5)
	d.Gems[weak.ID] = weak
	belt := item(84, db.SlotWaist, db.ArmorMail, db.Agility, 50)
	belt.GemSockets = []db.GemColor{db.GemRed}
	d.Items[belt.ID] = belt
	s := New(d, testProfile())

	// no buckle: GemTips proposes nothing for the missing socket —
	// applying the buckle is SocketTips' business
	ws := []Worn{{Item: belt, Gems: []int{1, 0, 0}, HasDetail: true}}
	if tips := s.GemTips(ws); len(tips) != 0 {
		t.Fatalf("no re-gemming on a bare, optimally-gemmed belt: %+v", tips)
	}
	// a weak gem in a worn buckle is upgradeable like any socket
	ws[0].Gems = []int{1, 54, 0}
	tips := s.GemTips(ws)
	if len(tips) != 1 || len(tips[0].Changes) != 1 {
		t.Fatalf("want one buckle re-gem: %+v", tips)
	}
	if ch := tips[0].Changes[0]; ch.SocketIdx != 1 || ch.From == nil || ch.From.ID != 54 || ch.To.ID != 1 {
		t.Fatalf("want buckle socket upgrade Weak Ruby → Cardinal Ruby at index 1: %+v", ch)
	}
}

func TestOptimizeGemsBuckle(t *testing.T) {
	d := testDatabase()
	belt := item(83, db.SlotWaist, db.ArmorMail, db.Agility, 50)
	belt.GemSockets = []db.GemColor{db.GemRed}
	d.Items[belt.ID] = belt
	s := New(d, testProfile())

	// bare belt, native socket already optimal: the only change proposed
	// is a gem in the (to be applied) buckle socket, indexed past the
	// item's own sockets
	ws := []Worn{{Item: belt, Gems: []int{1, 0, 0}, HasDetail: true}}
	plan := s.OptimizeGems(ws)
	if len(plan.Tips) != 1 || len(plan.Tips[0].Changes) != 1 {
		t.Fatalf("want exactly the buckle change: %+v", plan.Tips)
	}
	if ch := plan.Tips[0].Changes[0]; ch.SocketIdx != 1 || ch.From != nil || ch.To.ID != 1 {
		t.Fatalf("want empty buckle socket → Cardinal Ruby at index 1: %+v", ch)
	}
	if plan.Delta != 40 {
		t.Errorf("plan delta = %v, want 40", plan.Delta)
	}

	// a weak gem in an existing buckle is re-gemmable
	weak := gem(54, "Weak Ruby", db.GemRed, db.QualityEpic, db.Agility, 5)
	d.Gems[weak.ID] = weak
	s = New(d, testProfile())
	ws = []Worn{{Item: belt, Gems: []int{1, 54, 0}, HasDetail: true}}
	plan = s.OptimizeGems(ws)
	if len(plan.Tips) != 1 || len(plan.Tips[0].Changes) != 1 ||
		plan.Tips[0].Changes[0].SocketIdx != 1 || plan.Tips[0].Changes[0].To.ID != 1 {
		t.Fatalf("worn buckle gem must be upgradeable: %+v", plan.Tips)
	}
}

func TestBlacksmithSockets(t *testing.T) {
	d := testDatabase()
	bracer := item(85, db.SlotWrist, db.ArmorMail, db.Agility, 50)
	bracer.GemSockets = []db.GemColor{db.GemRed}
	gloves := item(86, db.SlotHands, db.ArmorMail, db.Agility, 50) // no native sockets
	d.Items[bracer.ID], d.Items[gloves.ID] = bracer, gloves
	s := New(d, testProfile())

	// non-blacksmith: no extra socket assumed
	if got := s.EffectiveStats(bracer)[db.Agility]; got != 70 { // 50 + 20 gem
		t.Errorf("bracer effective agility = %v, want 70 without Blacksmithing", got)
	}
	if got := s.EffectiveStats(gloves)[db.Agility]; got != 50 {
		t.Errorf("gloves effective agility = %v, want 50 without Blacksmithing", got)
	}
	// ...but a worn extra gem still counts (sockets outlive the profession)
	w := Worn{Item: bracer, Gems: []int{1, 1, 0}, HasDetail: true}
	if got := s.WornStats(w)[db.Agility]; got != 90 {
		t.Errorf("worn BS-socket gem agility = %v, want 90", got)
	}
	// and no socket tip without the profession
	if tips := s.SocketTips([]Worn{{Item: bracer, Gems: []int{1, 0, 0}, HasDetail: true}}); len(tips) != 0 {
		t.Fatalf("no socket tip without Blacksmithing: %+v", tips)
	}

	// blacksmith: both slots gain the assumed socket, tips fire
	s.SetProfessions(map[string]bool{"Blacksmithing": true})
	if got := s.EffectiveStats(bracer)[db.Agility]; got != 90 {
		t.Errorf("bracer effective agility = %v, want 90 with Blacksmithing", got)
	}
	if got := s.EffectiveStats(gloves)[db.Agility]; got != 70 {
		t.Errorf("gloves effective agility = %v, want 70 with Blacksmithing", got)
	}
	tips := s.SocketTips([]Worn{
		{Item: bracer, Gems: []int{1, 0, 0}, HasDetail: true},
		{Item: gloves, Gems: []int{0, 0, 0}, HasDetail: true},
	})
	if len(tips) != 2 {
		t.Fatalf("want socket tips for bracer and gloves: %+v", tips)
	}
	// the optimizer gems the blacksmith socket too (index past native)
	plan := s.OptimizeGems([]Worn{{Item: gloves, Gems: []int{0, 0, 0}, HasDetail: true}})
	if len(plan.Tips) != 1 || plan.Tips[0].Changes[0].SocketIdx != 0 || plan.Tips[0].Changes[0].To.ID != 1 {
		t.Fatalf("optimizer must gem the blacksmith socket: %+v", plan.Tips)
	}
}

func TestJewelcrafterGems(t *testing.T) {
	d := testDatabase() // gem 3 = Delicate Dragon's Eye (34 agi, epic, JC-only)
	it := item(87, db.SlotHead, db.ArmorMail, db.Agility, 50)
	it.GemSockets = []db.GemColor{db.GemRed}
	d.Items[it.ID] = it
	s := New(d, testProfile())

	// non-JC: the epic cut
	if got := s.EffectiveStats(it)[db.Agility]; got != 70 {
		t.Errorf("non-JC effective agility = %v, want 70", got)
	}
	// Dragon's Eyes never leak into the general picks
	if g := s.BestGem(db.GemRed); g == nil || g.ID != 1 {
		t.Fatalf("BestGem must stay the epic cut: %+v", g)
	}
	// jewelcrafter: the Dragon's Eye
	s.SetProfessions(map[string]bool{"Jewelcrafting": true})
	if got := s.EffectiveStats(it)[db.Agility]; got != 84 { // 50 + 34
		t.Errorf("JC effective agility = %v, want 84 (Dragon's Eye)", got)
	}
}

func TestJewelerGemQuota(t *testing.T) {
	d := testDatabase()
	a := item(88, db.SlotHead, db.ArmorMail, db.Agility, 10)
	a.GemSockets = []db.GemColor{db.GemRed, db.GemRed}
	b := item(89, db.SlotChest, db.ArmorMail, db.Agility, 10)
	b.GemSockets = []db.GemColor{db.GemRed, db.GemRed}
	d.Items[a.ID], d.Items[b.ID] = a, b
	s := New(d, testProfile())
	s.SetProfessions(map[string]bool{"Jewelcrafting": true})

	// four bare sockets, quota three: 3 Dragon's Eyes + 1 epic cut
	ws := []Worn{
		{Item: a, Gems: []int{0, 0}, HasDetail: true},
		{Item: b, Gems: []int{0, 0}, HasDetail: true},
	}
	de, epic := 0, 0
	for _, tip := range s.GemTips(ws) {
		for _, ch := range tip.Changes {
			if isJewelerGem(ch.To) {
				de++
			} else {
				epic++
			}
		}
	}
	if de != 3 || epic != 1 {
		t.Fatalf("tips = %d Dragon's Eyes + %d epic, want 3 + 1", de, epic)
	}
	// the optimizer lands on the same quota
	plan := s.OptimizeGems(ws)
	de = 0
	for _, w := range plan.Set {
		for _, id := range w.Gems {
			if isJewelerGem(s.d.ResolveGem(id)) {
				de++
			}
		}
	}
	if de != 3 {
		t.Fatalf("optimized set carries %d Dragon's Eyes, want 3", de)
	}

	// quota already worn: nothing to suggest
	full := []Worn{
		{Item: a, Gems: []int{3, 3}, HasDetail: true},
		{Item: b, Gems: []int{3, 1}, HasDetail: true},
	}
	if tips := s.GemTips(full); len(tips) != 0 {
		t.Fatalf("full quota must yield no tips: %+v", tips)
	}
	if plan := s.OptimizeGems(full); len(plan.Tips) != 0 {
		t.Fatalf("full quota: optimizer must keep the set: %+v", plan.Tips)
	}

	// non-JC never gets a Dragon's Eye proposed — but keeps worn ones
	s.SetProfessions(nil)
	plan = s.OptimizeGems(ws)
	for _, tip := range plan.Tips {
		for _, ch := range tip.Changes {
			if isJewelerGem(ch.To) {
				t.Fatalf("non-JC proposed a Dragon's Eye: %+v", ch)
			}
		}
	}
	if plan := s.OptimizeGems(full); len(plan.Tips) != 0 {
		t.Fatalf("non-JC must keep worn Dragon's Eyes: %+v", plan.Tips)
	}
}

func TestSetBonusScoring(t *testing.T) {
	d := testDatabase()
	// two pieces of a real curated set (T10 hunter: 2pc = 300 EP) plus
	// an off-set alternative that beats the piece on raw stats but not
	// on stats + broken bonus
	p1 := item(80, db.SlotHead, db.ArmorMail, db.Agility, 100)
	p1.SetName = "Ahn'Kahar Blood Hunter's Battlegear"
	p2 := item(81, db.SlotChest, db.ArmorMail, db.Agility, 100)
	p2.SetName = "Ahn'Kahar Blood Hunter's Battlegear"
	off := item(82, db.SlotHead, db.ArmorMail, db.Agility, 130)
	for _, it := range []*db.Item{p1, p2, off} {
		d.Items[it.ID] = it
	}
	s := New(d, testProfile()) // agi weight 2.0

	ws := wrapWorn([]*db.Item{p1, p2})
	// set score includes the 2pc: (100+100)*2 agi + 300 bonus
	if got := s.SetScoreWorn(ws); got != 700 {
		t.Fatalf("set score = %v, want 700 (400 stats + 300 bonus)", got)
	}

	// the off-piece is +30 agi (+60 EP) but breaks the 2pc (-300): it
	// must NOT be proposed as an upgrade...
	if ups := s.UpgradesWorn(ws, 0, 10, nil); len(ups) != 0 {
		t.Fatalf("bonus-breaking piece proposed: %+v", ups)
	}
	// ...and appears as a downgrade with the bonus priced in
	downs := s.DowngradesWorn(ws, 0, 10, nil)
	if len(downs) != 1 || downs[0].Item.ID != off.ID {
		t.Fatalf("downgrades = %+v", downs)
	}
	if got := downs[0].Delta; got != -240 {
		t.Fatalf("swap delta = %v, want -240 (+60 stats -300 bonus)", got)
	}

	// without the twin piece there is no bonus to break: now it IS an
	// upgrade at its raw +60
	solo := wrapWorn([]*db.Item{p1})
	ups := s.UpgradesWorn(solo, 0, 10, nil)
	if len(ups) != 1 || ups[0].Delta != 60 {
		t.Fatalf("solo upgrade = %+v, want +60", ups)
	}
}

func TestUniqueGemOncePerItem(t *testing.T) {
	d := testDatabase() // Cardinal Ruby (id 1, red, +20 agi)
	nt := gem(49110, "Nightmare Tear", db.GemPrismatic, db.QualityEpic, db.Agility, 25)
	nt.Unique = true
	solid := gem(30, "Shifting Dreadstone", db.GemPurple, db.QualityEpic, db.Agility, 10)
	d.Gems[nt.ID], d.Gems[solid.ID] = nt, solid

	// two blue sockets + a bonus strong enough that matching wins
	it := item(90, db.SlotHead, db.ArmorMail, db.Agility, 10)
	it.GemSockets = []db.GemColor{db.GemBlue, db.GemBlue}
	it.SocketBonus[db.Agility] = 20
	d.Items[it.ID] = it
	s := New(d, testProfile()) // agi 2.0

	// the Tear is the best blue-matching gem but unique-equipped: it may
	// fill only ONE socket; the other falls back to the purple hybrid
	gems, matched := s.AssumedGems(it)
	if !matched {
		t.Fatalf("strong bonus must be kept: %+v", gems)
	}
	if gems[0].ID != nt.ID || gems[1].ID != solid.ID {
		t.Fatalf("want [Tear, fallback], got [%v, %v]", gems[0], gems[1])
	}
	// 10 base + 25 Tear + 10 fallback + 20 bonus
	if got := s.GearStats(it)[db.Agility]; got != 65 {
		t.Fatalf("gear agility = %v, want 65", got)
	}
}

func TestOptimizeGemsUniqueAcrossItems(t *testing.T) {
	d := testDatabase()
	nt := gem(49110, "Nightmare Tear", db.GemPrismatic, db.QualityEpic, db.Agility, 25)
	nt.Unique = true
	d.Gems[nt.ID] = nt

	head := item(91, db.SlotHead, db.ArmorMail, db.Agility, 10)
	head.GemSockets = []db.GemColor{db.GemBlue}
	chest := item(92, db.SlotChest, db.ArmorMail, db.Agility, 10)
	chest.GemSockets = []db.GemColor{db.GemBlue}
	d.Items[head.ID], d.Items[chest.ID] = head, chest
	s := New(d, testProfile())

	countTears := func(ws []Worn) int {
		n := 0
		for _, w := range ws {
			for _, id := range w.Gems {
				if id == nt.ID {
					n++
				}
			}
		}
		return n
	}

	// both sockets empty: per-socket value says Tear (25) twice, but the
	// optimizer may spend it once — the other socket gets the runner-up
	// (Cardinal Ruby, 20, color-mismatched but legal). Determinism: the
	// plan must not flip run to run.
	for range 10 {
		ws := []Worn{
			{Item: head, Gems: []int{0}, HasDetail: true},
			{Item: chest, Gems: []int{0}, HasDetail: true},
		}
		plan := s.OptimizeGems(ws)
		if got := countTears(plan.Set); got != 1 {
			t.Fatalf("plan sockets %d Tears, want exactly 1: %+v", got, plan.Tips)
		}
		// 25 + 20, all ×2.0
		if plan.Delta != 90 {
			t.Fatalf("plan delta = %v, want 90", plan.Delta)
		}
	}

	// a Tear already worn on the head still counts: the chest socket
	// must not receive a second one
	ws := []Worn{
		{Item: head, Gems: []int{nt.ID}, HasDetail: true},
		{Item: chest, Gems: []int{0}, HasDetail: true},
	}
	plan := s.OptimizeGems(ws)
	if got := countTears(plan.Set); got != 1 {
		t.Fatalf("worn Tear duplicated: %d in plan %+v", got, plan.Tips)
	}
}

func TestGemTipsUniqueAcrossItems(t *testing.T) {
	d := testDatabase()
	nt := gem(49110, "Nightmare Tear", db.GemPrismatic, db.QualityEpic, db.Agility, 25)
	nt.Unique = true
	solid := gem(30, "Solid Majestic Zircon", db.GemBlue, db.QualityEpic, db.Agility, 10)
	d.Gems[nt.ID], d.Gems[solid.ID] = nt, solid

	mk := func(id int, slot db.Slot) *db.Item {
		it := item(id, slot, db.ArmorMail, db.Agility, 10)
		it.GemSockets = []db.GemColor{db.GemBlue}
		it.SocketBonus[db.Agility] = 20 // matching wins: rec = the Tear
		d.Items[it.ID] = it
		return it
	}
	head, chest := mk(93, db.SlotHead), mk(94, db.SlotChest)
	s := New(d, testProfile())

	// head already wears the character's one Tear: the chest tip must
	// fall back to the best non-unique blue gem
	ws := []Worn{
		{Item: head, Gems: []int{nt.ID}, HasDetail: true},
		{Item: chest, Gems: []int{0}, HasDetail: true},
	}
	tips := s.GemTips(ws)
	if len(tips) != 1 || tips[0].ItemIdx != 1 {
		t.Fatalf("want one tip for the chest, got %+v", tips)
	}
	if to := tips[0].Changes[0].To; to.ID != solid.ID {
		t.Fatalf("chest must get the non-unique fallback, got %v", to)
	}

	// both sockets empty: the Tear is suggested exactly once
	ws = []Worn{
		{Item: head, Gems: []int{0}, HasDetail: true},
		{Item: chest, Gems: []int{0}, HasDetail: true},
	}
	tears := 0
	for _, tip := range s.GemTips(ws) {
		for _, ch := range tip.Changes {
			if ch.To.ID == nt.ID {
				tears++
			}
		}
	}
	if tears != 1 {
		t.Fatalf("Tear suggested %d times, want once", tears)
	}
}

func TestBestSetBasicsAndUnique(t *testing.T) {
	d := testDatabase()
	// two trinket slots; one unique-equipped standout and a runner-up
	worn1 := item(200, db.SlotTrinket, db.ArmorAny, db.Agility, 10)
	worn2 := item(201, db.SlotTrinket, db.ArmorAny, db.Agility, 10)
	star := item(202, db.SlotTrinket, db.ArmorAny, db.Agility, 100)
	star.Unique = true
	second := item(203, db.SlotTrinket, db.ArmorAny, db.Agility, 80)
	// and a plain head slot with one clear upgrade
	head := item(204, db.SlotHead, db.ArmorMail, db.Agility, 20)
	better := item(205, db.SlotHead, db.ArmorMail, db.Agility, 60)
	for _, it := range []*db.Item{worn1, worn2, star, second, head, better} {
		d.Items[it.ID] = it
	}
	s := New(d, testProfile())

	ws := wrapWorn([]*db.Item{head, worn1, worn2})
	for range 10 { // deterministic run to run
		_, set, sc := s.BestSet(ws, nil)
		if set[0].ID != better.ID {
			t.Fatalf("head not upgraded: %v", set[0])
		}
		ids := []int{set[1].ID, set[2].ID}
		if !(ids[0] == star.ID && ids[1] == second.ID ||
			ids[0] == second.ID && ids[1] == star.ID) {
			t.Fatalf("trinkets = %v, want the unique once + runner-up", ids)
		}
		// 60 + 100 + 80 agility × 2.0
		if want := 480.0; sc != want {
			t.Fatalf("score = %v, want %v", sc, want)
		}
	}

	// allow=nil vs restrictive filter: banning the star trinket promotes
	// the runner-up into both slots? No — same item twice is fine unless
	// unique; two copies of `second` are legal.
	_, set, _ := s.BestSet(ws, func(it *db.Item) bool { return it.ID != star.ID })
	if set[1].ID != second.ID || set[2].ID != second.ID {
		t.Fatalf("filtered trinkets = %d,%d, want runner-up twice", set[1].ID, set[2].ID)
	}
}

func TestBestSetFormsCuratedTierSet(t *testing.T) {
	d := testDatabase()
	const setName = "Ahn'Kahar Blood Hunter's Battlegear" // real curated 2pc
	twoPc := 0.0
	for _, b := range WotLK.SetBonusesFor(setName) {
		if b.Pieces == 2 {
			twoPc = b.EP
		}
	}
	if twoPc <= 0 {
		t.Fatal("test premise: hunter T10 2pc must have positive EP")
	}
	// per-piece raw deficit: each tier piece is (twoPc/2 - 10) EP weaker
	// than the off-piece, so the pair pays +20 EP only via the bonus
	defAgi := (twoPc/2 - 10) / 2.0 // agi weight 2.0
	offHead := item(210, db.SlotHead, db.ArmorMail, db.Agility, 200)
	offChest := item(211, db.SlotChest, db.ArmorMail, db.Agility, 200)
	tierHead := item(212, db.SlotHead, db.ArmorMail, db.Agility, 200-defAgi)
	tierHead.SetName = setName
	tierChest := item(213, db.SlotChest, db.ArmorMail, db.Agility, 200-defAgi)
	tierChest.SetName = setName
	for _, it := range []*db.Item{offHead, offChest, tierHead, tierChest} {
		d.Items[it.ID] = it
	}
	s := New(d, testProfile())

	// starting from the off-pieces (the local optimum a greedy sweep
	// cannot leave: each tier piece alone is a downgrade), the tier seed
	// must discover the 2pc
	_, set, _ := s.BestSet(wrapWorn([]*db.Item{offHead, offChest}), nil)
	if set[0].ID != tierHead.ID || set[1].ID != tierChest.ID {
		t.Fatalf("2pc not formed: %v, %v", set[0].Name, set[1].Name)
	}

	// inverse: pieces so much weaker the bonus cannot pay — stay off-set
	tierHead.Stats[db.Agility] = 200 - (twoPc/2+50)/2.0
	tierChest.Stats[db.Agility] = 200 - (twoPc/2+50)/2.0
	s = New(d, testProfile()) // fresh memoization
	_, set, _ = s.BestSet(wrapWorn([]*db.Item{offHead, offChest}), nil)
	if set[0].ID != offHead.ID || set[1].ID != offChest.ID {
		t.Fatalf("overpriced 2pc formed anyway: %v, %v", set[0].Name, set[1].Name)
	}
}

func TestBestSetKeepsOwnedPiecesInPlace(t *testing.T) {
	d := testDatabase()
	junk := item(220, db.SlotFinger, db.ArmorAny, db.Agility, 10)
	bandB := item(221, db.SlotFinger, db.ArmorAny, db.Agility, 90)
	bandA := item(222, db.SlotFinger, db.ArmorAny, db.Agility, 100)
	bandA.Unique = true
	for _, it := range []*db.Item{junk, bandB, bandA} {
		d.Items[it.ID] = it
	}
	s := New(d, testProfile())

	// wearing the second-best ring in finger 1: the raw descent upgrades
	// finger 1 to the best ring and re-suggests the OWNED ring for
	// finger 2 — the result must instead keep the owned ring in place
	// and propose only the truly missing one
	_, set, _ := s.BestSet(wrapWorn([]*db.Item{bandB, junk}), nil)
	if set[0].ID != bandB.ID || set[1].ID != bandA.ID {
		t.Fatalf("owned ring must stay in its slot: got [%s, %s]", set[0].Name, set[1].Name)
	}
	// mirrored: owned ring in finger 2 stays in finger 2
	_, set, _ = s.BestSet(wrapWorn([]*db.Item{junk, bandB}), nil)
	if set[0].ID != bandA.ID || set[1].ID != bandB.ID {
		t.Fatalf("owned ring must stay in its slot: got [%s, %s]", set[0].Name, set[1].Name)
	}
}

func TestCalibrateNearCapPolicy(t *testing.T) {
	d := testDatabase()
	d.Gems[5] = gem(5, "Rigid King's Amber", db.GemYellow, db.QualityEpic, db.MeleeHit, 20)
	d.Gems[6] = gem(6, "Delicate Ametrine", db.GemOrange, db.QualityEpic, db.Agility, 15)

	// base hit 90, cap 100: keeping hit gems means every socket chases
	// the 10-point gap (usable +10 ×3.0 = 30 EP, the rest capped away);
	// zeroing frees three sockets for agility (3×15 ×2.0 = 90 EP)
	chest := item(45, db.SlotChest, db.ArmorMail, db.MeleeHit, 90)
	chest.GemSockets = []db.GemColor{db.GemYellow, db.GemYellow, db.GemYellow}
	d.Items[chest.ID] = chest

	s := New(d, testProfile())
	s.Calibrate([]*db.Item{chest})
	if g := s.BestGem(db.GemYellow); g == nil || g.ID != 6 {
		t.Fatalf("near cap with many sockets must socket agility, got %+v", g)
	}

	// far below the cap the hit gems genuinely pay: 3×20 = 60 usable
	// hit (180 EP) beats 90 EP of agility — keep them
	far := item(46, db.SlotChest, db.ArmorMail, db.MeleeHit, 20)
	far.GemSockets = chest.GemSockets
	d.Items[far.ID] = far
	s.Calibrate([]*db.Item{far})
	if g := s.BestGem(db.GemYellow); g == nil || g.ID != 5 {
		t.Fatalf("far under cap must keep hit gems, got %+v", g)
	}
}

func TestTalentReachability(t *testing.T) {
	// warrior Precision: Fury row 5 (20 pts required), 3 ranks
	var prec *CapTalent
	for i, ct := range capTalents[db.ClassWarrior] {
		if ct.Name == "Precision" {
			prec = &capTalents[db.ClassWarrior][i]
		}
	}
	if prec == nil {
		t.Fatal("warrior Precision not in capTalents")
	}
	for _, tc := range []struct {
		treePts, want int
	}{
		{-1, 3}, // unknown tree total — no restriction
		{0, 0},  // no Fury points: locked
		{20, 0}, // exactly the row requirement: nothing left to place
		{22, 2}, // partially reachable
		{40, 3}, // deep Fury: full rank
	} {
		if got := prec.MaxReachable(tc.treePts); got != tc.want {
			t.Errorf("MaxReachable(%d) = %d, want %d", tc.treePts, got, tc.want)
		}
	}

	if got := WotLK.TreePoints(db.ClassHunter, "Marksmanship", []int{0, 15, 56}); got != 15 {
		t.Errorf("hunter MM points = %d, want 15", got)
	}
	if got := WotLK.TreePoints(db.ClassHunter, "Nonsense", []int{0, 15, 56}); got != -1 {
		t.Errorf("unknown tree = %d, want -1", got)
	}
	if got := WotLK.TreePoints(db.ClassHunter, "Survival", []int{0, 15}); got != -1 {
		t.Errorf("malformed sums = %d, want -1", got)
	}
	// every cap talent's tree name must resolve in its class's tab
	// order, or reachability silently degrades to "unrestricted"
	for class, list := range capTalents {
		for _, ct := range list {
			if WotLK.TreePoints(class, ct.Tree, []int{1, 2, 3}) < 0 {
				t.Errorf("%v %s: tree %q not in ClassicTreeOrder", class, ct.Name, ct.Tree)
			}
		}
	}
}

// TestCandidateMetaActivation: under a swap context with color facts,
// the candidate's assumed meta must be one the swap outcome can
// actually activate; with none activatable the meta prices as zero
// (kept only so the socket-bonus match survives).
func TestCandidateMetaActivation(t *testing.T) {
	build := func(rule54 db.MetaCondition) (*Scorer, *db.Item, []Worn) {
		d := testDatabase()
		d.Gems[53] = gem(53, "Chaotic Skyflare Diamond", db.GemMeta, db.QualityRare, db.Agility, 25)
		d.Gems[54] = gem(54, "Revitalizing Skyflare Diamond", db.GemMeta, db.QualityRare, db.Agility, 15)
		d.Rules.MetaConditions = map[int]db.MetaCondition{
			4:  {MinRed: 1, MinYellow: 1, MinBlue: 1}, // testDatabase's meta
			53: {MinBlue: 2},
			54: rule54,
		}
		wornHelm := item(60, db.SlotHead, db.ArmorMail, db.Agility, 50)
		wornHelm.GemSockets = []db.GemColor{db.GemMeta}
		chest := item(61, db.SlotChest, db.ArmorMail, db.Agility, 50)
		chest.GemSockets = []db.GemColor{db.GemRed, db.GemRed}
		cand := item(62, db.SlotHead, db.ArmorMail, db.Agility, 55)
		cand.GemSockets = []db.GemColor{db.GemMeta}
		for _, it := range []*db.Item{wornHelm, chest, cand} {
			d.Items[it.ID] = it
		}
		s := New(d, testProfile())
		// worn: helm carries the 2-blue meta, the KEPT chest two red gems
		ws := []Worn{
			{Item: wornHelm, Gems: []int{53}, HasDetail: true},
			{Item: chest, Gems: []int{1, 1}, HasDetail: true},
		}
		return s, cand, ws
	}

	// the kept chest provides red only: the best metas (1/1/1 and
	// 2-blue) can't light up — the assumed meta must be the 2-red one
	s, cand, ws := build(db.MetaCondition{MinRed: 2})
	gems, _ := s.AssumedGemsSwap(cand, s.SwapContextFor(ws, 0))
	if len(gems) != 1 || gems[0].ID != 54 {
		t.Fatalf("assumed meta = %+v, want the activatable 2-red meta (54)", gems)
	}

	// no meta can activate at all: the socket keeps a meta for the
	// bonus match but its stats price as ZERO
	s, cand, ws = build(db.MetaCondition{MinBlue: 3})
	gems, _ = s.AssumedGemsSwap(cand, s.SwapContextFor(ws, 0))
	if len(gems) != 1 || gems[0].Color != db.GemMeta {
		t.Fatalf("meta socket must keep a meta: %+v", gems)
	}
	if gems[0].Stats != (db.Stats{}) {
		t.Fatalf("unactivatable meta must price as zero, got %+v", gems[0].Stats)
	}

	// without color facts (plain import) the optimistic stance stands:
	// best meta assumed active
	s, cand, ws = build(db.MetaCondition{MinBlue: 3})
	for i := range ws {
		ws[i].HasDetail = false
	}
	gems, _ = s.AssumedGemsSwap(cand, s.SwapContextFor(ws, 0))
	if len(gems) != 1 || gems[0].Stats == (db.Stats{}) {
		t.Fatalf("plain import must keep the optimistic meta: %+v", gems)
	}
}

// TestCandidateMetaCarryRescue: a prismatic carried off the replaced
// item counts toward the candidate's meta activation — the Nightmare
// Tear that kept the worn meta lit keeps the candidate's lit too.
func TestCandidateMetaCarryRescue(t *testing.T) {
	d := testDatabase()
	nt := gem(52, "Nightmare Tear", db.GemPrismatic, db.QualityEpic, db.Agility, 10)
	nt.Unique = true
	d.Gems[52] = nt
	d.Rules.MetaConditions = map[int]db.MetaCondition{
		4: {MinRed: 1, MinYellow: 1, MinBlue: 1},
	}
	wornHelm := item(60, db.SlotHead, db.ArmorMail, db.Agility, 50)
	wornHelm.GemSockets = []db.GemColor{db.GemMeta, db.GemRed}
	chest := item(61, db.SlotChest, db.ArmorMail, db.Agility, 50)
	chest.GemSockets = []db.GemColor{db.GemRed}
	cand := item(62, db.SlotHead, db.ArmorMail, db.Agility, 55)
	cand.GemSockets = []db.GemColor{db.GemMeta, db.GemRed}
	for _, it := range []*db.Item{wornHelm, chest, cand} {
		d.Items[it.ID] = it
	}
	s := New(d, testProfile())
	// the worn helm's Tear provides the yellow+blue counts; the kept
	// chest is red-only — the candidate inherits the Tear (carry) and
	// its meta stays assumed ACTIVE with real stats
	ws := []Worn{
		{Item: wornHelm, Gems: []int{4, 52}, HasDetail: true},
		{Item: chest, Gems: []int{1}, HasDetail: true},
	}
	gems, _ := s.AssumedGemsSwap(cand, s.SwapContextFor(ws, 0))
	if len(gems) != 2 || gems[0].ID != 4 || gems[0].Stats == (db.Stats{}) {
		t.Fatalf("carried Tear must keep the meta active: %+v", gems)
	}
	foundNT := false
	for _, g := range gems {
		foundNT = foundNT || g.ID == 52
	}
	if !foundNT {
		t.Fatalf("Tear not carried onto the candidate: %+v", gems)
	}
}

// TestUnknownMetaRule: a meta the rules table doesn't cover gets NO
// invented rule — InactiveMeta stays silent, UnknownMeta reports it,
// and the optimizer flags the plan instead of constraining on 1/1/1.
func TestUnknownMetaRule(t *testing.T) {
	d := testDatabase()
	mystery := gem(999, "Mystery Diamond", db.GemMeta, db.QualityRare, db.Agility, 21)
	d.Gems[999] = mystery
	helm := item(60, db.SlotHead, db.ArmorMail, db.Agility, 50)
	helm.GemSockets = []db.GemColor{db.GemMeta, db.GemRed, db.GemRed}
	d.Items[helm.ID] = helm
	s := New(d, testProfile())

	// two pure red gems would FAIL a one-of-each rule — but no rule is
	// known, so no inactivity claim may be made
	ws := []Worn{{Item: helm, Gems: []int{999, 1, 1}, HasDetail: true}}
	if g, _ := s.InactiveMeta(ws); g != nil {
		t.Fatalf("no rule known, yet meta claimed inactive: %v", g.Name)
	}
	if g := s.UnknownMeta(ws); g == nil || g.ID != 999 {
		t.Fatalf("unknown meta not reported: %+v", g)
	}
	plan := s.OptimizeGems(ws)
	if plan.UnknownMeta == nil || plan.UnknownMeta.ID != 999 {
		t.Fatalf("optimizer must flag the unknown meta: %+v", plan.UnknownMeta)
	}
}
