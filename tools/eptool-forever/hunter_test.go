//go:build ignore

// eptool-forever: direct mean-DPS differences for Forever hunters on the
// pinned sim (never core.StatWeights). Run from the root of a
// wowsims/forever clone at pinnedForeverCommit (internal/db/loadforever.go):
//
//	GOBIN=$PWD/bin go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.36.12
//	PATH=$PWD/bin:$PATH protoc -I=./proto \
//	    --go_opt=Mgoogle/protobuf/descriptor.proto=google.golang.org/protobuf/types/descriptorpb \
//	    --go_out=./sim/core ./proto/*.proto
//	cp <this file> sim/hunter/zz_ep_test.go   # drop the build tag
//	go test -tags with_db ./sim/hunter -run TestZZEP -v -count=1 -timeout 2h -iters 20000
//
// It lives in the hunter package to reuse that package's test fixtures
// (gear set, talents, consumables, pet options, rotations). with_db is
// required: the presets are SIML files that need the name table.
// Results and the profile they produced: internal/game/forever/profiles.go.
package hunter

import (
	"flag"
	"fmt"
	"testing"

	"github.com/wowsims/forever/sim/core"
	"github.com/wowsims/forever/sim/core/proto"
)

var epIters = flag.Int("iters", 10000, "iterations per variant")

type epDelta struct {
	label  string
	stat   int  // proto.Stat index, or pseudo index when pseudo
	pseudo bool
	amount float64
}

func epPlayer(tree string) *proto.Player {
	p := &proto.Player{
		Name: "ep", Race: proto.Race_RaceOrc, Class: proto.Class_ClassHunter,
		Equipment:   core.GetGearSet("../../ui/specs/hunter/dps/gear_sets", "prebis_weave").GearSet,
		Consumables: DefaultConsumables, Buffs: core.FullIndividualBuffs,
		Profession1: proto.Profession_Engineering,
	}
	switch tree {
	case "BM":
		rot := core.GetAplRotation("../../ui/specs/hunter/dps/apls", "default")
		stopWeaving(rot.Rotation)
		p.TalentsString, p.Rotation = DefaultBMTalents, rot.Rotation
		p.Spec = petOptions()[0].SpecOptions.(*proto.Player_Hunter)
	case "MM":
		p.TalentsString, p.Spec = DefaultMMTalents, DefaultOptions
		p.Rotation = core.GetAplRotation("../../ui/specs/hunter/dps/apls", "default").Rotation
	case "SV":
		p.TalentsString, p.Spec = DefaultSVTalents, DefaultOptions
		p.Rotation = core.GetAplRotation("../../ui/specs/hunter/dps/apls", "default").Rotation
	}
	return p
}

func epRun(t *testing.T, tree string, ds ...epDelta) float64 {
	p := epPlayer(tree)
	st := make([]float64, len(proto.Stat_name))
	ps := make([]float64, len(proto.PseudoStat_name))
	for _, d := range ds {
		if d.pseudo {
			ps[d.stat] += d.amount
		} else {
			st[d.stat] += d.amount
		}
	}
	p.BonusStats = &proto.UnitStats{Stats: st, PseudoStats: ps}
	res := core.RunRaidSimConcurrent(&proto.RaidSimRequest{
		Raid:       core.SinglePlayerRaidProto(p, core.FullPartyBuffs, core.FullRaidBuffs, core.FullDebuffs),
		Encounter:  core.MakeSingleTargetEncounter(0),
		SimOptions: &proto.SimOptions{Iterations: int32(*epIters), RandomSeed: 101},
	})
	if res.Error != nil {
		t.Fatalf("%s: %s", tree, res.Error.Message)
	}
	return res.RaidMetrics.Dps.Avg
}

func TestZZEP(t *testing.T) {
	S := func(s proto.Stat) int { return int(s) }
	P := func(s proto.PseudoStat) int { return int(s) }
	deltas := []epDelta{
		{"agi", S(proto.Stat_StatAgility), false, 60},
		{"str", S(proto.Stat_StatStrength), false, 60},
		{"int", S(proto.Stat_StatIntellect), false, 60},
		{"spi", S(proto.Stat_StatSpirit), false, 60},
		{"ap", S(proto.Stat_StatAttackPower), false, 120},
		{"rap", S(proto.Stat_StatRangedAttackPower), false, 120},
		{"crit 2%", S(proto.Stat_StatCritRating), false, 28},
		{"haste 2%", S(proto.Stat_StatHasteRating), false, 20},
		{"mp5", S(proto.Stat_StatMP5), false, 20},
		{"armpen", S(proto.Stat_StatArmorPenetration), false, 200},
		{"mh dps", P(proto.PseudoStat_PseudoStatMainHandDps), true, 10},
		{"rng dps", P(proto.PseudoStat_PseudoStatRangedDps), true, 10},
	}
	hit := S(proto.Stat_StatHitRating)
	for _, tree := range []string{"BM", "MM", "SV"} {
		cs := core.ComputeStats(&proto.ComputeStatsRequest{
			Raid:      core.SinglePlayerRaidProto(epPlayer(tree), core.FullPartyBuffs, core.FullRaidBuffs, core.FullDebuffs),
			Encounter: core.MakeSingleTargetEncounter(0),
		})
		ps := cs.RaidStats.Parties[0].Players[0]
		fmt.Printf("== %s sheet: gear hit %.0f, final hit %.0f, final crit %.0f, final agi %.0f, final rap %.0f\n", tree,
			ps.GearStats.Stats[proto.Stat_StatHitRating], ps.FinalStats.Stats[proto.Stat_StatHitRating],
			ps.FinalStats.Stats[proto.Stat_StatCritRating], ps.FinalStats.Stats[proto.Stat_StatAgility],
			ps.FinalStats.Stats[proto.Stat_StatRangedAttackPower])
		base := epRun(t, tree)
		rap := (epRun(t, tree, epDelta{"rap", S(proto.Stat_StatRangedAttackPower), false, 120}) - base) / 120
		fmt.Printf("== %s  base %.1f DPS  (1 RAP = %.4f DPS)\n", tree, base, rap)
		for _, d := range deltas {
			dps := (epRun(t, tree, d) - base) / d.amount
			fmt.Printf("  %-8s %+8.4f DPS/pt  EP(rap) %6.3f\n", d.label, dps, dps/rap)
		}
		// hit sweep, 1% steps around the gear's own total
		prev := 0.0
		for k := -3; k <= 2; k++ {
			v := epRun(t, tree, epDelta{"hit", hit, false, float64(k * 10)})
			if k > -3 {
				fmt.Printf("  hit %+d%%→%+d%%: %+.4f DPS/rating  EP(rap) %6.3f\n", k-1, k, (v-prev)/10, (v-prev)/10/rap)
			}
			prev = v
		}
	}
}

func TestZZHit(t *testing.T) {
	hit := int(proto.Stat_StatHitRating)
	for _, tree := range []string{"BM", "MM"} {
		prev := 0.0
		for k := 1; k <= 5; k++ {
			v := epRun(t, tree, epDelta{"hit", hit, false, float64(k * 10)})
			if k > 1 {
				fmt.Printf("%s hit +%d%%→+%d%%: %+.4f DPS/rating\n", tree, k-1, k, (v-prev)/10)
			}
			prev = v
		}
	}
}
