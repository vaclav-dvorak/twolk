//go:build ignore

// eptool-cata: direct mean-DPS differences for cata warriors, mages and
// warlocks on the pinned sim
// (never core.StatWeights). Run from the root of a wowsims/cata clone at
// pinnedCataCommit (internal/db/loadcata.go):
//
//	GOBIN=$PWD/bin go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.34.2
//	PATH=$PWD/bin:$PATH protoc -I=./proto --go_out=./sim/core ./proto/*.proto
//	mkdir -p cmd/wep && cp <this file> cmd/wep/main.go   # drop the build tag
//	go build -tags with_db -o wep ./cmd/wep
//	./wep -spec tg -at hit=-400 -d str=300,hit=150,exp=150      # stat slopes
//	./wep -spec tg -breakdown -d str=1                          # damage shares, aura uptimes
//	./wep -spec tg -trinkets 65118,69167                        # trinket EP (2nd slot empty vs filled)
//	./wep -spec fire -at hit=-900 -d sp=300,hit=300             # casters: spell power is the numeraire
//
// iteration i runs on seed+i, so a replication needs a seed range that
// does not overlap the first one (e.g. -seed 1000000), not seed 202.
//
// -swap (stat-compensated gear swaps, for set bonuses) prices whatever
// the swap broke. Session 47 rejected it when a null swap read ±1%; the
// cause was the compensation handing back DERIVED totals (health, mana,
// intellect's spell power) on top of the stats that derive them. With
// those left out (swapRun), session 48's null swaps read exactly 0.00%
// for casters. Warriors' strength→AP is not yet excluded: rerun a null
// control before trusting -swap on a melee spec.
//
//	./wep -spec fire -replace 14=71085 -swap 8=71435    # 4pc: one tier piece out
package main

import (
	"flag"
	"fmt"
	"strconv"
	"strings"

	_ "github.com/wowsims/cata/sim/common"
	"github.com/wowsims/cata/sim/core"
	"github.com/wowsims/cata/sim/core/proto"
	"github.com/wowsims/cata/sim/mage/arcane"
	"github.com/wowsims/cata/sim/mage/fire"
	"github.com/wowsims/cata/sim/warlock/affliction"
	"github.com/wowsims/cata/sim/warlock/demonology"
	"github.com/wowsims/cata/sim/warlock/destruction"
	"github.com/wowsims/cata/sim/warrior/arms"
	"github.com/wowsims/cata/sim/warrior/fury"
)

var consumes = &proto.ConsumesSpec{FlaskId: 58088, FoodId: 62670, PotId: 58146, PrepotId: 58146, TinkerId: 82174}

// casterConsumes: Draconic Mind, the +90 intellect fish, Volcanic Potion
// (the sim's own caster test consumes; the fire test still carries
// wotlk ones)
var casterConsumes = &proto.ConsumesSpec{FlaskId: 58086, FoodId: 62671, PotId: 58091, PrepotId: 58091, TinkerId: 82174}

var arcaneGlyphs = &proto.Glyphs{
	Prime1: int32(proto.MagePrimeGlyph_GlyphOfArcaneMissiles), Prime2: int32(proto.MagePrimeGlyph_GlyphOfArcaneBlast),
	Prime3: int32(proto.MagePrimeGlyph_GlyphOfMageArmor), Major1: int32(proto.MageMajorGlyph_GlyphOfEvocation),
	Major2: int32(proto.MageMajorGlyph_GlyphOfArcanePower), Major3: int32(proto.MageMajorGlyph_GlyphOfManaShield),
}
var fireGlyphs = &proto.Glyphs{
	Prime1: int32(proto.MagePrimeGlyph_GlyphOfFireball), Prime2: int32(proto.MagePrimeGlyph_GlyphOfPyroblast),
	Prime3: int32(proto.MagePrimeGlyph_GlyphOfMoltenArmor), Major1: int32(proto.MageMajorGlyph_GlyphOfEvocation),
	Major2: int32(proto.MageMajorGlyph_GlyphOfDragonsBreath), Major3: int32(proto.MageMajorGlyph_GlyphOfInvisibility),
}
var afflictionGlyphs = &proto.Glyphs{
	Prime1: int32(proto.WarlockPrimeGlyph_GlyphOfHaunt), Prime2: int32(proto.WarlockPrimeGlyph_GlyphOfUnstableAffliction),
	Prime3: int32(proto.WarlockPrimeGlyph_GlyphOfCorruption), Major1: int32(proto.WarlockMajorGlyph_GlyphOfFelhunter),
	Major2: int32(proto.WarlockMajorGlyph_GlyphOfShadowBolt), Major3: int32(proto.WarlockMajorGlyph_GlyphOfSoulSwap),
}
var demonologyGlyphs = &proto.Glyphs{
	Prime1: int32(proto.WarlockPrimeGlyph_GlyphOfImmolate), Prime2: int32(proto.WarlockPrimeGlyph_GlyphOfIncinerate),
	Prime3: int32(proto.WarlockPrimeGlyph_GlyphOfMetamorphosis), Major1: int32(proto.WarlockMajorGlyph_GlyphOfSoulstone),
	Major2: int32(proto.WarlockMajorGlyph_GlyphOfLifeTap), Major3: int32(proto.WarlockMajorGlyph_GlyphOfFelhunter),
}
var destructionGlyphs = &proto.Glyphs{
	Prime1: int32(proto.WarlockPrimeGlyph_GlyphOfConflagrate), Prime2: int32(proto.WarlockPrimeGlyph_GlyphOfImmolate),
	Prime3: int32(proto.WarlockPrimeGlyph_GlyphOfImp), Major1: int32(proto.WarlockMajorGlyph_GlyphOfLifeTap),
	Major2: int32(proto.WarlockMajorGlyph_GlyphOfHealthstone), Major3: int32(proto.WarlockMajorGlyph_GlyphOfSoulLink),
}

// caster reports whether the spec's numeraire is spell power (else AP)
func caster(spec string) bool {
	switch spec {
	case "arcane", "fire", "aff", "demo", "destro":
		return true
	}
	return false
}

var furyGlyphs = &proto.Glyphs{
	Prime1: int32(proto.WarriorPrimeGlyph_GlyphOfBloodthirst), Prime2: int32(proto.WarriorPrimeGlyph_GlyphOfRagingBlow),
	Prime3: int32(proto.WarriorPrimeGlyph_GlyphOfSlam), Major1: int32(proto.WarriorMajorGlyph_GlyphOfColossusSmash),
	Major2: int32(proto.WarriorMajorGlyph_GlyphOfCleaving), Major3: int32(proto.WarriorMajorGlyph_GlyphOfDeathWish),
	Minor1: int32(proto.WarriorMinorGlyph_GlyphOfBerserkerRage), Minor2: int32(proto.WarriorMinorGlyph_GlyphOfBattle),
	Minor3: int32(proto.WarriorMinorGlyph_GlyphOfCommand),
}
var armsGlyphs = &proto.Glyphs{
	Prime1: int32(proto.WarriorPrimeGlyph_GlyphOfMortalStrike), Prime2: int32(proto.WarriorPrimeGlyph_GlyphOfOverpower),
	Prime3: int32(proto.WarriorPrimeGlyph_GlyphOfSlam), Major1: int32(proto.WarriorMajorGlyph_GlyphOfColossusSmash),
	Major2: int32(proto.WarriorMajorGlyph_GlyphOfShieldWall), Major3: int32(proto.WarriorMajorGlyph_GlyphOfRapidCharge),
	Minor1: int32(proto.WarriorMinorGlyph_GlyphOfBerserkerRage),
}

var (
	gearFlag  = flag.String("gear", "", "gear set name override (e.g. p1_fury_tg)")
	swapFlag  = flag.String("swap", "", "slotIndex=itemID,... replaced in the gear (reforge dropped)")
	trinkFlag = flag.String("trinkets", "", "itemIDs: DPS of each in the 2nd trinket slot vs the slot empty, in AP-equivalent EP")
	replFlag  = flag.String("replace", "", "slotIndex=itemID,... swapped into the gear for every run (e.g. 14=71085: no Dragonwrath)")
)

func gearName(def string) string {
	if *gearFlag != "" {
		return *gearFlag
	}
	return def
}

// applySwaps returns a copy of the equipment with the -swap items in place.
func applySwaps(eq *proto.EquipmentSpec, swaps string) *proto.EquipmentSpec {
	out := &proto.EquipmentSpec{}
	for _, it := range eq.Items {
		c := *it
		out.Items = append(out.Items, &c)
	}
	for _, kv := range strings.Split(swaps, ",") {
		if kv == "" {
			continue
		}
		k, v, _ := strings.Cut(kv, "=")
		i, _ := strconv.Atoi(k)
		id, _ := strconv.Atoi(v)
		out.Items[i].Id = int32(id)
		out.Items[i].Reforging = 0
	}
	return out
}

func gearStats(p *proto.Player) []float64 {
	cs := core.ComputeStats(&proto.ComputeStatsRequest{
		Raid:      core.SinglePlayerRaidProto(p, core.FullPartyBuffs, core.FullRaidBuffs, core.FullDebuffs),
		Encounter: core.MakeSingleTargetEncounter(0),
	})
	return cs.RaidStats.Parties[0].Players[0].GearStats.Stats
}

func player(spec string) *proto.Player {
	opts := &proto.WarriorOptions{StartingRage: 0}
	p := &proto.Player{Race: proto.Race_RaceTroll, Class: proto.Class_ClassWarrior, Consumables: consumes,
		Buffs: core.FullIndividualBuffs, Profession1: proto.Profession_Engineering, ReactionTimeMs: 100, ChannelClipDelayMs: 50}
	var specOpts interface{}
	switch spec {
	case "arms":
		p.Equipment = core.GetGearSet("ui/warrior/arms/gear_sets", gearName("p3_arms_bis")).GearSet
		p.TalentsString, p.Glyphs = "32120303120212312201-0322-3", armsGlyphs
		p.Rotation = core.GetAplRotation("ui/warrior/arms/apls", "arms").Rotation
		specOpts = &proto.Player_ArmsWarrior{ArmsWarrior: &proto.ArmsWarrior{Options: &proto.ArmsWarrior_Options{ClassOptions: opts}}}
	case "tg", "smf":
		p.Equipment = core.GetGearSet("ui/warrior/fury/gear_sets", gearName("p3_fury_"+spec)).GearSet
		p.TalentsString = map[string]string{"tg": "302003-03222203130110122321-2", "smf": "302003-032222031301101223201-2"}[spec]
		p.Glyphs = furyGlyphs
		p.Rotation = core.GetAplRotation("ui/warrior/fury/apls", spec).Rotation
		specOpts = &proto.Player_FuryWarrior{FuryWarrior: &proto.FuryWarrior{Options: &proto.FuryWarrior_Options{ClassOptions: opts}}}
	case "arcane", "fire":
		p.Race, p.Class, p.Consumables, p.DistanceFromTarget = proto.Race_RaceTroll, proto.Class_ClassMage, casterConsumes, 20
		mo := &proto.MageOptions{}
		if spec == "arcane" {
			p.Equipment = core.GetGearSet("ui/mage/arcane/gear_sets", gearName("p3")).GearSet
			p.TalentsString, p.Glyphs = "303322021230122210121-23-03", arcaneGlyphs
			p.Rotation = core.GetAplRotation("ui/mage/arcane/apls", "arcane").Rotation
			specOpts = &proto.Player_ArcaneMage{ArcaneMage: &proto.ArcaneMage{Options: &proto.ArcaneMage_Options{ClassOptions: mo, FocusMagicPercentUptime: 90}}}
		} else {
			p.Equipment = core.GetGearSet("ui/mage/fire/gear_sets", gearName("p3_fire")).GearSet
			p.TalentsString, p.Glyphs = "003-230330221120121213231-03", fireGlyphs
			p.Rotation = core.GetAplRotation("ui/mage/fire/apls", "fire").Rotation
			specOpts = &proto.Player_FireMage{FireMage: &proto.FireMage{Options: &proto.FireMage_Options{ClassOptions: mo}}}
		}
	case "aff", "demo", "destro":
		p.Race, p.Class, p.Consumables, p.DistanceFromTarget = proto.Race_RaceOrc, proto.Class_ClassWarlock, casterConsumes, 25
		wo := func(pet proto.WarlockOptions_Summon) *proto.WarlockOptions { return &proto.WarlockOptions{Summon: pet} }
		switch spec {
		case "aff":
			p.Equipment = core.GetGearSet("ui/warlock/affliction/gear_sets", gearName("p3")).GearSet
			p.TalentsString, p.Glyphs = "223222003013321321-03-33", afflictionGlyphs
			p.Rotation = core.GetAplRotation("ui/warlock/affliction/apls", "default").Rotation
			specOpts = &proto.Player_AfflictionWarlock{AfflictionWarlock: &proto.AfflictionWarlock{Options: &proto.AfflictionWarlock_Options{ClassOptions: wo(proto.WarlockOptions_Felhunter)}}}
		case "demo":
			p.Equipment = core.GetGearSet("ui/warlock/demonology/gear_sets", gearName("p3")).GearSet
			p.TalentsString, p.Glyphs = "003-3312222300310212211-03202", demonologyGlyphs
			p.Rotation = core.GetAplRotation("ui/warlock/demonology/apls", "incinerate").Rotation
			specOpts = &proto.Player_DemonologyWarlock{DemonologyWarlock: &proto.DemonologyWarlock{Options: &proto.DemonologyWarlock_Options{ClassOptions: wo(proto.WarlockOptions_Felguard)}}}
		case "destro":
			p.Equipment = core.GetGearSet("ui/warlock/destruction/gear_sets", gearName("p3")).GearSet
			p.TalentsString, p.Glyphs = "003-03202-3320202312201312211", destructionGlyphs
			p.Rotation = core.GetAplRotation("ui/warlock/destruction/apls", "default").Rotation
			specOpts = &proto.Player_DestructionWarlock{DestructionWarlock: &proto.DestructionWarlock{Options: &proto.DestructionWarlock_Options{ClassOptions: wo(proto.WarlockOptions_Imp)}}}
		}
	default:
		panic("spec must be arms, tg, smf, arcane, fire, aff, demo or destro")
	}
	if *replFlag != "" {
		p.Equipment = applySwaps(p.Equipment, *replFlag)
	}
	return core.WithSpec(p, specOpts)
}

// swapRun measures the -swap gear against the unswapped gear with the
// gear-stat difference handed back as bonus stats: what is left is the
// value of whatever the swap broke (a set bonus).
func swapRun(spec string, iters int, seed int64) {
	base := player(spec)
	swapped := player(spec)
	swapped.Equipment = applySwaps(base.Equipment, *swapFlag)
	for _, pl := range []*proto.Player{base, swapped} {
		cs := core.ComputeStats(&proto.ComputeStatsRequest{
			Raid:      core.SinglePlayerRaidProto(pl, core.FullPartyBuffs, core.FullRaidBuffs, core.FullDebuffs),
			Encounter: core.MakeSingleTargetEncounter(0),
		})
		ps := cs.RaidStats.Parties[0].Players[0]
		var gems []int32
		for _, it := range pl.Equipment.Items {
			gems = append(gems, it.Gems...)
		}
		fmt.Printf("  sets %v gems %v\n", ps.Sets, gems[:min(len(gems), 3)])
	}
	gb, gs := gearStats(base), gearStats(swapped)
	comp := map[int]float64{}
	for i := range gb {
		if d := gb[i] - gs[i]; d != 0 {
			comp[i] = d
		}
	}
	// gear stats carry DERIVED totals: health and mana from stamina and
	// intellect, spell power including the intellect's. Handing those
	// back as bonus stats counts them twice (session 47's null swaps
	// failed at ±1% this way), so only the item's own stats go back.
	delete(comp, 24)
	delete(comp, 25)
	if caster(spec) {
		comp[14] -= comp[3]
	}
	b := run(spec, nil, iters, seed)
	r := runPlayer(swapped, comp, iters, seed)
	fmt.Printf("%s %s swap {%s}: base %.1f, swapped+compensated %.1f, lost %.1f dps (%.2f%%)  comp %v\n",
		spec, gearName(""), *swapFlag, b.dps, r.dps, b.dps-r.dps, 100*(b.dps-r.dps)/b.dps, comp)
}

type res struct{ dps, aps float64 }

var breakdown = flag.Bool("breakdown", false, "print per-action damage shares and aura uptimes of the base run")

func run(spec string, bonus map[int]float64, iters int, seed int64) res {
	return runPlayer(player(spec), bonus, iters, seed)
}

func runPlayer(p *proto.Player, bonus map[int]float64, iters int, seed int64) res {
	st := make([]float64, 27)
	for k, v := range bonus {
		st[k] = v
	}
	p.BonusStats = &proto.UnitStats{Stats: st}
	rsr := &proto.RaidSimRequest{
		Raid:       core.SinglePlayerRaidProto(p, core.FullPartyBuffs, core.FullRaidBuffs, core.FullDebuffs),
		Encounter:  core.MakeSingleTargetEncounter(0),
		SimOptions: &proto.SimOptions{Iterations: int32(iters), RandomSeed: seed},
	}
	r := core.RunRaidSimConcurrent(rsr)
	if r.Error != nil {
		panic(r.Error.Message)
	}
	u := r.RaidMetrics.Parties[0].Players[0]
	var hits float64
	spell := p.Class == proto.Class_ClassMage || p.Class == proto.Class_ClassWarlock
	for _, a := range u.Actions {
		if a.IsMelee == spell {
			continue
		}
		for _, t := range a.Targets {
			// casters: landed casts and periodic ticks alike — cata
			// spell-damage procs roll on both
			hits += float64(t.Hits + t.Crits + t.Glances + t.Ticks + t.CritTicks)
		}
	}
	if *breakdown {
		var total float64
		dmg := map[string]float64{}
		for _, a := range u.Actions {
			k := fmt.Sprintf("%v/%d", a.Id.RawId, a.Id.Tag)
			for _, t := range a.Targets {
				dmg[k] += t.Damage
				total += t.Damage
			}
		}
		for k, v := range dmg {
			if v/total > 0.005 {
				fmt.Printf("    dmg %-40s %5.1f%%\n", k, 100*v/total)
			}
		}
		for _, a := range u.Auras {
			if a.UptimeSecondsAvg > 1 {
				fmt.Printf("    aura %-39v %5.1f%% uptime, %.1f procs\n", a.Id.RawId, 100*a.UptimeSecondsAvg/300, a.ProcsAvg)
			}
		}
		*breakdown = false
	}
	return res{u.Dps.Avg, hits / float64(iters) / 300}
}

var names = map[string]int{"str": 0, "agi": 1, "int": 3, "spi": 4, "hit": 5, "crit": 6, "haste": 7, "exp": 8,
	"mastery": 11, "ap": 12, "sp": 14}

// parse reads "hit=-500,exp=-300" into bonus stats.
func parse(s string) map[int]float64 {
	m := map[int]float64{}
	for _, kv := range strings.Split(s, ",") {
		if kv == "" {
			continue
		}
		k, v, _ := strings.Cut(kv, "=")
		idx, ok := names[k]
		if !ok {
			panic("unknown stat " + k)
		}
		f, _ := strconv.ParseFloat(v, 64)
		m[idx] = f
	}
	return m
}

func main() {
	spec := flag.String("spec", "arms", "arms|tg|smf|arcane|fire|aff|demo|destro")
	at := flag.String("at", "", "base offsets, e.g. hit=-400")
	d := flag.String("d", "str=300", "deltas, each measured separately from the base")
	iters := flag.Int("iters", 50000, "")
	seed := flag.Int64("seed", 101, "")
	flag.Parse()
	arms.RegisterArmsWarrior()
	fury.RegisterFuryWarrior()
	arcane.RegisterArcaneMage()
	fire.RegisterFireMage()
	affliction.RegisterAfflictionWarlock()
	demonology.RegisterDemonologyWarlock()
	destruction.RegisterDestructionWarlock()
	numeraire, numName := 12, "AP" // attack power, or spell power for casters
	if caster(*spec) {
		numeraire, numName = 14, "SP"
	}
	if *trinkFlag != "" {
		empty := player(*spec)
		empty.Equipment = applySwaps(empty.Equipment, "13=0")
		e := runPlayer(empty, nil, *iters, *seed)
		ap := runPlayer(empty, map[int]float64{numeraire: 600}, *iters, *seed)
		perAP := (ap.dps - e.dps) / 600
		fmt.Printf("%s %s: trinket slot empty %.1f dps, %.4f dps per %s, %.3f landings/s\n", *spec, gearName(""), e.dps, perAP, numName, e.aps)
		for _, id := range strings.Split(*trinkFlag, ",") {
			p := player(*spec)
			p.Equipment = applySwaps(p.Equipment, "13="+id)
			r := runPlayer(p, nil, *iters, *seed)
			fmt.Printf("  trinket %-6s %+8.1f dps = %7.1f EP\n", id, r.dps-e.dps, (r.dps-e.dps)/perAP)
		}
		return
	}
	if *swapFlag != "" {
		swapRun(*spec, *iters, *seed)
		return
	}
	base := parse(*at)
	{
		p := player(*spec)
		cs := core.ComputeStats(&proto.ComputeStatsRequest{
			Raid:      core.SinglePlayerRaidProto(p, core.FullPartyBuffs, core.FullRaidBuffs, core.FullDebuffs),
			Encounter: core.MakeSingleTargetEncounter(0),
		})
		ps := cs.RaidStats.Parties[0].Players[0]
		g, f := ps.GearStats.Stats, ps.FinalStats.Stats
		fmt.Printf("%s gear: str %.0f int %.0f spi %.0f hit %.0f exp %.0f crit %.0f haste %.0f mastery %.0f sp %.0f | final: str %.0f int %.0f hit %.0f exp %.0f ap %.0f sp %.0f\n",
			*spec, g[0], g[3], g[4], g[5], g[8], g[6], g[7], g[11], g[14], f[0], f[3], f[5], f[8], f[12], f[14])
	}
	b := run(*spec, base, *iters, *seed)
	fmt.Printf("%s at {%s} seed %d: base %.1f dps, %.3f landings/s\n", *spec, *at, *seed, b.dps, b.aps)
	for _, kv := range strings.Split(*d, ",") {
		pt := map[int]float64{}
		for k, v := range base {
			pt[k] = v
		}
		var amt float64
		for k, v := range parse(kv) {
			pt[k] += v
			amt = v
		}
		r := run(*spec, pt, *iters, *seed)
		fmt.Printf("  %-12s %+8.1f dps  %.4f dps/pt\n", kv, r.dps-b.dps, (r.dps-b.dps)/amt)
	}
}
