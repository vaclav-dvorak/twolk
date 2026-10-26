//go:build ignore

// eptool-cata: direct mean-DPS differences for cata warriors on the pinned sim
// (never core.StatWeights). Run from the root of a wowsims/cata clone at
// pinnedCataCommit (internal/db/loadcata.go):
//
//	GOBIN=$PWD/bin go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.34.2
//	PATH=$PWD/bin:$PATH protoc -I=./proto --go_out=./sim/core ./proto/*.proto
//	mkdir -p cmd/wep && cp <this file> cmd/wep/main.go   # drop the build tag
//	go build -tags with_db -o wep ./cmd/wep
//	./wep -spec tg -at hit=-400 -d str=300,hit=150,exp=150
package main

import (
	"flag"
	"fmt"
	"strconv"
	"strings"

	_ "github.com/wowsims/cata/sim/common"
	"github.com/wowsims/cata/sim/core"
	"github.com/wowsims/cata/sim/core/proto"
	"github.com/wowsims/cata/sim/warrior/arms"
	"github.com/wowsims/cata/sim/warrior/fury"
)

var consumes = &proto.ConsumesSpec{FlaskId: 58088, FoodId: 62670, PotId: 58146, PrepotId: 58146, TinkerId: 82174}

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

func player(spec string) *proto.Player {
	opts := &proto.WarriorOptions{StartingRage: 0}
	p := &proto.Player{Race: proto.Race_RaceTroll, Class: proto.Class_ClassWarrior, Consumables: consumes,
		Buffs: core.FullIndividualBuffs, Profession1: proto.Profession_Engineering, ReactionTimeMs: 100, ChannelClipDelayMs: 50}
	var specOpts interface{}
	switch spec {
	case "arms":
		p.Equipment = core.GetGearSet("ui/warrior/arms/gear_sets", "p3_arms_bis").GearSet
		p.TalentsString, p.Glyphs = "32120303120212312201-0322-3", armsGlyphs
		p.Rotation = core.GetAplRotation("ui/warrior/arms/apls", "arms").Rotation
		specOpts = &proto.Player_ArmsWarrior{ArmsWarrior: &proto.ArmsWarrior{Options: &proto.ArmsWarrior_Options{ClassOptions: opts}}}
	case "tg", "smf":
		p.Equipment = core.GetGearSet("ui/warrior/fury/gear_sets", "p3_fury_"+spec).GearSet
		p.TalentsString = map[string]string{"tg": "302003-03222203130110122321-2", "smf": "302003-032222031301101223201-2"}[spec]
		p.Glyphs = furyGlyphs
		p.Rotation = core.GetAplRotation("ui/warrior/fury/apls", spec).Rotation
		specOpts = &proto.Player_FuryWarrior{FuryWarrior: &proto.FuryWarrior{Options: &proto.FuryWarrior_Options{ClassOptions: opts}}}
	default:
		panic("spec must be arms, tg or smf")
	}
	return core.WithSpec(p, specOpts)
}

type res struct{ dps, aps float64 }

func run(spec string, bonus map[int]float64, iters int, seed int64) res {
	p := player(spec)
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
	for _, a := range u.Actions {
		if !a.IsMelee {
			continue
		}
		for _, t := range a.Targets {
			hits += float64(t.Hits + t.Crits)
		}
	}
	return res{u.Dps.Avg, hits / float64(iters) / 300}
}

var names = map[string]int{"str": 0, "agi": 1, "hit": 5, "crit": 6, "haste": 7, "exp": 8, "ap": 12}

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
	spec := flag.String("spec", "arms", "arms|tg|smf")
	at := flag.String("at", "", "base offsets, e.g. hit=-400")
	d := flag.String("d", "str=300", "deltas, each measured separately from the base")
	iters := flag.Int("iters", 50000, "")
	seed := flag.Int64("seed", 101, "")
	flag.Parse()
	arms.RegisterArmsWarrior()
	fury.RegisterFuryWarrior()
	base := parse(*at)
	b := run(*spec, base, *iters, *seed)
	fmt.Printf("%s at {%s} seed %d: base %.1f dps, %.3f melee landings/s\n", *spec, *at, *seed, b.dps, b.aps)
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
