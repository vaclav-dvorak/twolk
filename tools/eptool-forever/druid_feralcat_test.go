//go:build ignore

// eptool-forever: direct mean-DPS differences (never core.StatWeights) on the
// pinned wowsims/forever sim. Copy into the package directory as
// zz_ep_test.go (drop the build tag) and run
//
//	go test -tags with_db ./sim/<pkg> -run TestZZEP -v -count=1 -timeout 3h -iters 20000 -spec <name>
//
// The player is built from the package's own test-suite fixtures exactly as
// core.CharacterSuiteConfig.defaultPlayerAndRaid builds it.
package feralcat

import (
	"flag"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/wowsims/forever/sim/core"
	"github.com/wowsims/forever/sim/core/proto"
)

// TestFeralCat's default combo (note: that suite runs with empty consumables).
func epSpecs() []epSpec {
	return []epSpec{
		{name: "feralcat", melee: true, feral: true, school: proto.Stat_StatStamina, cfg: func() core.CharacterSuiteConfig {
			return core.CharacterSuiteConfig{
				Class: proto.Class_ClassDruid, Race: proto.Race_RaceNightElf,
				GearSet:     core.GetGearSet("../../../ui/specs/druid/feralcat/gear_sets", "default"),
				Talents:     DefaultTalents,
				SpecOptions: core.SpecOptionsCombo{Label: "Standard", SpecOptions: DefaultSpecOptions},
				Rotation:    core.GetAplRotation("../../../ui/specs/druid/feralcat/apls", "adaptive"),
				Consumables: &proto.ConsumesSpec{},
			}
		}},
	}
}

// ---- generic part (identical in every package) ----

var epIters = flag.Int("iters", 10000, "iterations per variant")
var epOnly = flag.String("spec", "", "comma list of spec names to run (default all)")

type epDelta struct {
	label  string
	stat   int // proto.Stat index, or pseudo index when pseudo
	pseudo bool
	amount float64
}

type epSpec struct {
	name   string
	cfg    func() core.CharacterSuiteConfig
	caster bool       // numeraire spell damage (else melee attack power)
	school proto.Stat // main school damage stat; Stat_StatStamina = none
	dw     bool       // dual wield: measure off-hand dps
	feral  bool       // measure feral attack power
	melee  bool       // expertise sweep
	extra  []epDelta  // spec specific extra deltas
}

func S(s proto.Stat) int                           { return int(s) }
func P(s proto.PseudoStat) int                     { return int(s) }
func sd(l string, s proto.Stat, a float64) epDelta { return epDelta{l, S(s), false, a} }

// Mirrors core.CharacterSuiteConfig.defaultPlayerAndRaid (unexported).
func epRaid(c core.CharacterSuiteConfig, bonus *proto.UnitStats) *proto.Raid {
	ib := core.Ternary(c.IndividualBuffs != nil, c.IndividualBuffs, core.FullIndividualBuffs)
	rb := core.Ternary(c.RaidBuffs != nil, c.RaidBuffs, core.FullRaidBuffs)
	pb := core.Ternary(c.PartyBuffs != nil, c.PartyBuffs, core.FullPartyBuffs)
	db := core.Ternary(c.Debuffs != nil, c.Debuffs, core.FullDebuffs)
	p := core.WithSpec(&proto.Player{
		Name: "ep", Class: c.Class, Race: c.Race,
		Equipment: c.GearSet.GearSet, Consumables: c.Consumables, Buffs: ib,
		TalentsString: c.Talents,
		Profession1:   core.Ternary(c.Profession1 != proto.Profession_ProfessionUnknown, c.Profession1, proto.Profession_Engineering),
		Profession2:   c.Profession2,
		Rotation:      c.Rotation.Rotation,
		ItemSwap:      c.ItemSwapSet.ItemSwap, EnableItemSwap: c.ItemSwapSet.ItemSwap != nil,
		Cooldowns: c.Cooldowns, HealingModel: c.HealingModel,
		InFrontOfTarget: c.InFrontOfTarget, DistanceFromTarget: c.StartingDistance,
		ReactionTimeMs:     core.TernaryInt32(c.ReactionTimeMs != 0, c.ReactionTimeMs, 100),
		ChannelClipDelayMs: core.TernaryInt32(c.ChannelClipDelayMs != 0, c.ChannelClipDelayMs, 50),
		BonusStats:         bonus,
	}, c.SpecOptions.SpecOptions)
	return core.SinglePlayerRaidProto(p, pb, rb, db)
}

func epEnc(c core.CharacterSuiteConfig) *proto.Encounter {
	if c.Encounter.Encounter != nil {
		return c.Encounter.Encounter
	}
	return core.MakeSingleTargetEncounter(0)
}

func epRun(t *testing.T, sp epSpec, ds ...epDelta) float64 {
	c := sp.cfg()
	st := make([]float64, len(proto.Stat_name))
	ps := make([]float64, len(proto.PseudoStat_name))
	for _, d := range ds {
		if d.pseudo {
			ps[d.stat] += d.amount
		} else {
			st[d.stat] += d.amount
		}
	}
	res := core.RunRaidSimConcurrent(&proto.RaidSimRequest{
		Raid:       epRaid(c, &proto.UnitStats{Stats: st, PseudoStats: ps}),
		Encounter:  epEnc(c),
		SimOptions: &proto.SimOptions{Iterations: int32(*epIters), RandomSeed: 101},
	})
	if res.Error != nil {
		t.Fatalf("%s: %s", sp.name, res.Error.Message)
	}
	return res.RaidMetrics.Dps.Avg
}

func epSheet(sp epSpec) {
	c := sp.cfg()
	cs := core.ComputeStats(&proto.ComputeStatsRequest{Raid: epRaid(c, nil), Encounter: epEnc(c)})
	if cs.ErrorResult != "" {
		fmt.Printf("== %s ComputeStats error: %s\n", sp.name, cs.ErrorResult)
		return
	}
	ps := cs.RaidStats.Parties[0].Players[0]
	g, tl, f := ps.GearStats, ps.TalentsStats, ps.FinalStats
	fmt.Printf("== %s setup: class %v race %v prof %v/%v gear %q talents %q rotation %q spec %q iters %d\n", sp.name,
		c.Class, c.Race, core.Ternary(c.Profession1 != proto.Profession_ProfessionUnknown, c.Profession1, proto.Profession_Engineering), c.Profession2,
		c.GearSet.Label, c.Talents, c.Rotation.Label, c.SpecOptions.Label, *epIters)
	hr, xr, cr := S(proto.Stat_StatHitRating), S(proto.Stat_StatExpertiseRating), S(proto.Stat_StatCritRating)
	fmt.Printf("== %s sheet: hit rating gear %.0f talents %.0f final %.0f | expertise rating gear %.0f final %.0f | crit rating gear %.0f final %.0f\n", sp.name,
		g.Stats[hr], tl.Stats[hr], f.Stats[hr], g.Stats[xr], f.Stats[xr], g.Stats[cr], f.Stats[cr])
	pp := func(u *proto.UnitStats, s proto.PseudoStat) float64 {
		if int(s) < len(u.PseudoStats) {
			return u.PseudoStats[s]
		}
		return 0
	}
	fmt.Printf("== %s pseudo final: meleeHit%% %.3f spellHit%% %.3f expertise%% %.3f meleeCrit%% %.3f spellCrit%% %.3f | talents: meleeHit%% %.3f spellHit%% %.3f expertise%% %.3f\n", sp.name,
		pp(f, proto.PseudoStat_PseudoStatMeleeHitPercent), pp(f, proto.PseudoStat_PseudoStatSpellHitPercent), pp(f, proto.PseudoStat_PseudoStatExpertisePercent),
		pp(f, proto.PseudoStat_PseudoStatMeleeCritPercent), pp(f, proto.PseudoStat_PseudoStatSpellCritPercent),
		pp(tl, proto.PseudoStat_PseudoStatMeleeHitPercent), pp(tl, proto.PseudoStat_PseudoStatSpellHitPercent), pp(tl, proto.PseudoStat_PseudoStatExpertisePercent))
	sch := ""
	for _, s := range []proto.PseudoStat{proto.PseudoStat_PseudoStatSchoolHitPercentArcane, proto.PseudoStat_PseudoStatSchoolHitPercentFire,
		proto.PseudoStat_PseudoStatSchoolHitPercentFrost, proto.PseudoStat_PseudoStatSchoolHitPercentHoly,
		proto.PseudoStat_PseudoStatSchoolHitPercentNature, proto.PseudoStat_PseudoStatSchoolHitPercentShadow} {
		sch += fmt.Sprintf(" %s %.3f/%.3f", strings.TrimPrefix(s.String(), "PseudoStatSchoolHitPercent"), pp(f, s), pp(tl, s))
	}
	fmt.Printf("== %s school hit%% final/talents:%s\n", sp.name, sch)
	fmt.Printf("== %s final: str %.0f agi %.0f int %.0f spi %.0f ap %.0f sp %.0f haste %.0f armpen %.0f mp5 %.0f\n", sp.name,
		f.Stats[proto.Stat_StatStrength], f.Stats[proto.Stat_StatAgility], f.Stats[proto.Stat_StatIntellect], f.Stats[proto.Stat_StatSpirit],
		f.Stats[proto.Stat_StatAttackPower], f.Stats[proto.Stat_StatSpellDamage], f.Stats[proto.Stat_StatHasteRating],
		f.Stats[proto.Stat_StatArmorPenetration], f.Stats[proto.Stat_StatMP5])
	ws := ""
	for _, w := range ps.WeaponSkills {
		ws += fmt.Sprintf(" %s=%d", strings.TrimPrefix(w.Skill.String(), "WeaponSkill"), w.Value)
	}
	fmt.Printf("== %s weapon skills:%s\n", sp.name, ws)
}

func TestZZEP(t *testing.T) {
	want := map[string]bool{}
	for _, n := range strings.Split(*epOnly, ",") {
		if n != "" {
			want[n] = true
		}
	}
	for _, sp := range epSpecs() {
		if len(want) > 0 && !want[sp.name] {
			continue
		}
		t0 := time.Now()
		epSheet(sp)
		var ds []epDelta
		var num epDelta
		if sp.caster {
			num = sd("spell dmg", proto.Stat_StatSpellDamage, 60)
			ds = []epDelta{
				sd("int", proto.Stat_StatIntellect, 60), sd("spi", proto.Stat_StatSpirit, 60),
				num, sd("healing", proto.Stat_StatHealingPower, 60),
			}
			if sp.school != proto.Stat_StatStamina {
				ds = append(ds, sd(strings.TrimPrefix(sp.school.String(), "Stat"), sp.school, 60))
			}
			ds = append(ds, sd("hit 1%", proto.Stat_StatHitRating, 10), sd("crit 2%", proto.Stat_StatCritRating, 28),
				sd("haste 2%", proto.Stat_StatHasteRating, 20), sd("mp5", proto.Stat_StatMP5, 20),
				sd("spellpen", proto.Stat_StatSpellPiercing, 20))
		} else {
			num = sd("ap", proto.Stat_StatAttackPower, 120)
			ds = []epDelta{
				sd("str", proto.Stat_StatStrength, 60), sd("agi", proto.Stat_StatAgility, 60),
				sd("int", proto.Stat_StatIntellect, 60), sd("spi", proto.Stat_StatSpirit, 60), num,
			}
			if sp.feral {
				ds = append(ds, sd("feral ap", proto.Stat_StatFeralAttackPower, 120))
			}
			ds = append(ds, sd("spell dmg", proto.Stat_StatSpellDamage, 60))
			if sp.school != proto.Stat_StatStamina {
				ds = append(ds, sd(strings.TrimPrefix(sp.school.String(), "Stat"), sp.school, 60))
			}
			ds = append(ds, sd("hit 1%", proto.Stat_StatHitRating, 10), sd("crit 2%", proto.Stat_StatCritRating, 28),
				sd("haste 2%", proto.Stat_StatHasteRating, 20), sd("exp 2%(20r)", proto.Stat_StatExpertiseRating, 20),
				sd("armpen", proto.Stat_StatArmorPenetration, 200), sd("mp5", proto.Stat_StatMP5, 20),
				epDelta{"mh dps", P(proto.PseudoStat_PseudoStatMainHandDps), true, 10})
			if sp.dw {
				ds = append(ds, epDelta{"oh dps", P(proto.PseudoStat_PseudoStatOffHandDps), true, 10})
			}
		}
		ds = append(ds, sp.extra...)
		base := epRun(t, sp)
		numv := (epRun(t, sp, num) - base) / num.amount
		fmt.Printf("== %s  base %.2f DPS  numeraire %s: 1 pt = %.5f DPS\n", sp.name, base, num.label, numv)
		for _, d := range ds {
			dps := (epRun(t, sp, d) - base) / d.amount
			fmt.Printf("  %-14s %+5.0f  %+9.5f DPS/pt  EP %7.4f\n", d.label, d.amount, dps, dps/numv)
		}
		hit := S(proto.Stat_StatHitRating)
		prev := 0.0
		// 1% steps from 4% below the gear total (negative totals clamp to 0) up to
		// well past the cap: 12% above for melee, 18% above for casters.
		kmax := core.Ternary(sp.caster, 18, 12)
		for k := -4; k <= kmax; k++ {
			v := base
			if k != 0 {
				v = epRun(t, sp, epDelta{"hit", hit, false, float64(k * 10)})
			}
			if k > -4 {
				fmt.Printf("  hit sweep %+d%%→%+d%%: %+9.5f DPS/rating  EP %7.4f  (dps %.2f)\n", k-1, k, (v-prev)/10, (v-prev)/10/numv, v)
			}
			prev = v
		}
		if sp.melee {
			exp := S(proto.Stat_StatExpertiseRating)
			prev = base
			for k := 1; k <= 8; k++ {
				v := epRun(t, sp, epDelta{"exp", exp, false, float64(k * 10)})
				fmt.Printf("  exp sweep +%d%%→+%d%%: %+9.5f DPS/rating  EP %7.4f  (dps %.2f)\n", k-1, k, (v-prev)/10, (v-prev)/10/numv, v)
				prev = v
			}
		}
		fmt.Printf("== %s done in %s\n", sp.name, time.Since(t0).Round(time.Second))
	}
}
