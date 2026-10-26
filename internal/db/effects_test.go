package db

import (
	"testing"
	"time"
)

// Verbatim excerpts from wowsims sim/common/wotlk sources at the pinned commit.
const procSrc = `
	newProcStatBonusEffect(ProcStatBonusEffect{
		Name:       "Mirror of Truth",
		ID:         40684,
		AuraID:     60065,
		Bonus:      stats.Stats{stats.AttackPower: 1000, stats.RangedAttackPower: 1000},
		Duration:   time.Second * 10,
		Callback:   core.CallbackOnSpellHitDealt,
		ProcMask:   core.ProcMaskMeleeOrRanged,
		Outcome:    core.OutcomeCrit,
		Harmful:    true,
		ProcChance: 0.10,
		ICD:        time.Second * 50,
	})
	newProcStatBonusEffect(ProcStatBonusEffect{
		Name:     "Ashen Band of Unmatched Vengeance",
		ID:       50401,
		AuraID:   72416,
		Bonus:    stats.Stats{stats.AttackPower: 480, stats.RangedAttackPower: 480},
		Duration: time.Second * 10,
		Callback: core.CallbackOnSpellHitDealt,
		ProcMask: core.ProcMaskMeleeOrRanged,
		Outcome:  core.OutcomeLanded,
		Harmful:  true,
		PPM:      1,
	})
`

const cdsSrc = `
	newHasteActive := testFirstOnly(func(itemID int32, bonus float64, duration time.Duration, cooldown time.Duration) {
		core.NewSimpleStatOffensiveTrinketEffect(itemID, stats.Stats{stats.MeleeHaste: bonus, stats.SpellHaste: bonus}, duration, cooldown)
	})
	newHasteActive(36972, 256, time.Second*20, time.Minute*2) // Tome of Arcane Phenomena
	//newHasteActive(37558, 122, time.Second*20, time.Minute*2) // Tidal Boon
	newSpellPowerActive := testFirstOnly(func(itemID int32, bonus float64, duration time.Duration, cooldown time.Duration) {
		core.NewSimpleStatOffensiveTrinketEffect(itemID, stats.Stats{stats.SpellPower: bonus}, duration, cooldown)
	})
	newSpellPowerActive(37873, 346, time.Second*20, time.Minute*2) // Mark of the War Prisoner
`

func TestParseProcEffects(t *testing.T) {
	effs := ParseProcEffects([]byte(procSrc))
	if len(effs) != 2 {
		t.Fatalf("parsed %d procs, want 2", len(effs))
	}
	mirror := effs[0]
	if mirror.ItemID != 40684 || mirror.Name != "Mirror of Truth" || mirror.Kind != EffectProc {
		t.Errorf("mirror identity wrong: %+v", mirror)
	}
	if mirror.Bonus[AttackPower] != 1000 || mirror.Bonus[RangedAttackPower] != 1000 {
		t.Errorf("mirror bonus wrong: %v", mirror.Bonus)
	}
	if mirror.Duration != 10*time.Second || mirror.Cooldown != 50*time.Second {
		t.Errorf("mirror timing wrong: dur=%v icd=%v", mirror.Duration, mirror.Cooldown)
	}
	if mirror.ProcChance != 0.10 || mirror.PPM != 0 {
		t.Errorf("mirror chance wrong: %+v", mirror)
	}
	if mirror.Outcome != "Crit" || mirror.ProcMask != "MeleeOrRanged" || mirror.Callback != "OnSpellHitDealt" {
		t.Errorf("mirror trigger wrong: %+v", mirror)
	}

	band := effs[1]
	if band.PPM != 1 || band.ProcChance != 0 || band.Outcome != "Landed" {
		t.Errorf("ppm-based proc wrong: %+v", band)
	}
}

func TestParseOnUseEffects(t *testing.T) {
	effs := ParseOnUseEffects([]byte(cdsSrc))
	if len(effs) != 2 {
		t.Fatalf("parsed %d on-use effects, want 2 (commented call skipped)", len(effs))
	}
	tome := effs[0]
	if tome.ItemID != 36972 || tome.Kind != EffectOnUse || tome.Name != "Tome of Arcane Phenomena" {
		t.Errorf("tome identity wrong: %+v", tome)
	}
	if tome.Bonus[MeleeHaste] != 256 || tome.Bonus[SpellHaste] != 256 {
		t.Errorf("tome bonus wrong: %v", tome.Bonus)
	}
	if tome.Duration != 20*time.Second || tome.Cooldown != 2*time.Minute {
		t.Errorf("tome timing wrong: %+v", tome)
	}
	mark := effs[1]
	if mark.ItemID != 37873 || mark.Bonus[SpellPower] != 346 || mark.Bonus[MeleeHaste] != 0 {
		t.Errorf("factory stat binding leaked between factories: %+v", mark)
	}
}

// Verbatim excerpts from both games' stat_bonus_stacking.go at the
// pinned commits: a plain wotlk proc, a wotlk on-use CD builder, a
// plain cata proc (shared. prefix) and a version-indexed cata literal.
// The type definition and factory signature reproduce the wotlk file's
// non-call mentions of the type names, which the parser must skip.
const stackingSrc = `
type StackingStatBonusEffect struct {
	Name       string
	Bonus      stats.Stats
}

func newStackingStatBonusEffect(config StackingStatBonusEffect) {
}

	newStackingStatBonusEffect(StackingStatBonusEffect{
		Name:      "Herkuml War Token",
		ID:        50355,
		AuraID:    71396,
		Duration:  time.Second * 10,
		MaxStacks: 20,
		Bonus:     stats.Stats{stats.AttackPower: 17, stats.RangedAttackPower: 17},
		Callback:  core.CallbackOnSpellHitDealt,
		ProcMask:  core.ProcMaskMeleeOrRanged,
		Harmful:   true,
	})
	newStackingStatBonusCD(StackingStatBonusCD{
		Name:      "Victor's Call",
		ID:        47725,
		AuraID:    67737,
		Duration:  time.Second * 20,
		MaxStacks: 5,
		Bonus:     stats.Stats{stats.AttackPower: 215, stats.RangedAttackPower: 215},
		CD:        time.Minute * 2,
		Callback:  core.CallbackOnSpellHitDealt,
		ProcMask:  core.ProcMaskMelee,
		Outcome:   core.OutcomeLanded,
	})
	shared.NewStackingStatBonusEffect(shared.StackingStatBonusEffect{
		Name:       "Fluid Death",
		ItemID:     58181,
		AuraID:     92104,
		Bonus:      stats.Stats{stats.Agility: 38},
		MaxStacks:  10,
		ProcMask:   core.ProcMaskMeleeOrRanged,
		Duration:   time.Second * 15,
		Callback:   core.CallbackOnSpellHitDealt,
		Harmful:    false,
		ProcChance: 1,
	})
	shared.NewStackingStatBonusEffect(shared.StackingStatBonusEffect{
		Name:       "Wrath of Unchaining" + labelSuffix,
		ItemID:     []int32{77974, 77197, 77994}[version],
		AuraID:     []int32{109717, 107960, 109719}[version],
		Bonus:      stats.Stats{stats.Agility: []float64{78, 88, 99}[version]},
		MaxStacks:  10,
		Callback:   core.CallbackOnSpellHitDealt,
		Outcome:    core.OutcomeLanded,
		ProcMask:   core.ProcMaskMeleeOrRanged,
		Duration:   time.Second * 10,
		ProcChance: 1,
	})
`

func TestParseStackingEffects(t *testing.T) {
	effs := ParseStackingEffects([]byte(stackingSrc))
	if len(effs) != 6 {
		t.Fatalf("parsed %d stacking effects, want 6 (three versions of one literal)", len(effs))
	}
	herkuml := effs[0]
	if herkuml.ItemID != 50355 || herkuml.Kind != EffectProc || herkuml.MaxStacks != 20 {
		t.Errorf("herkuml identity wrong: %+v", herkuml)
	}
	if herkuml.Bonus[AttackPower] != 17 || herkuml.Duration != 10*time.Second ||
		herkuml.ProcMask != "MeleeOrRanged" {
		t.Errorf("herkuml fields wrong: %+v", herkuml)
	}
	victor := effs[1]
	if victor.ItemID != 47725 || victor.Kind != EffectOnUse || victor.MaxStacks != 5 ||
		victor.Cooldown != 2*time.Minute || victor.Duration != 20*time.Second ||
		victor.Bonus[AttackPower] != 215 {
		t.Errorf("stacking on-use CD wrong: %+v", victor)
	}
	fluid := effs[2]
	if fluid.ItemID != 58181 || fluid.MaxStacks != 10 || fluid.Bonus[Agility] != 38 ||
		fluid.ProcChance != 1 || fluid.Cooldown != 0 {
		t.Errorf("fluid death wrong: %+v", fluid)
	}
	// the versioned literal expands into one effect per difficulty,
	// pairing the i-th item id with the i-th per-stack bonus
	wantVer := []struct {
		id  int
		agi float64
	}{{77974, 78}, {77197, 88}, {77994, 99}}
	for i, w := range wantVer {
		e := effs[3+i]
		if e.ItemID != w.id || e.Bonus[Agility] != w.agi || e.MaxStacks != 10 ||
			e.Name != "Wrath of Unchaining" {
			t.Errorf("version %d wrong: %+v", i, e)
		}
	}
}

func TestApplyCurated(t *testing.T) {
	d := &Database{Effects: map[int][]*Effect{
		50362: {{ItemID: 50362, Kind: EffectProc}}, // pretend extracted
	}}
	d.applyCurated()
	if got := d.Effects[50362][0].Kind; got != EffectProc {
		t.Error("curated entry must not shadow extracted data")
	}
	if len(d.Effects[44253]) != 1 || d.Effects[44253][0].Bonus[Agility] != 90 {
		t.Error("curated DMC Greatness (agility) missing")
	}
}
