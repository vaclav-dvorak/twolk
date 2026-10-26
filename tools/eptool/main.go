//go:build ignore

// eptool measures a spec's stat EP against the SIM ITSELF, at a chosen
// gear point — the numbers profiles.go cannot take from ui/<spec>/sim.ts
// because one published weight per stat cannot describe a stat whose
// value moves: armor penetration rises toward its cap, and hit and
// expertise are worth a lot BELOW their caps and nothing above, so a
// table measured on capped preset gear reports the wrong side of the
// cap for exactly the stats the planner has to reason about pre-cap.
//
// It is not part of the twolk build (hence the ignore tag): it runs
// inside a clone of the pinned WoWSims repo. Recipe:
//
//	git clone https://github.com/wowsims/wotlk && cd wotlk
//	git checkout <pinnedCommit from internal/db/load.go>
//	go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.31.0
//	protoc -I=./proto --go_out=./sim/core ./proto/*.proto
//	mkdir -p cmd/eptool && cp <this file> cmd/eptool/main.go   # drop the build tag
//	go build -tags with_db -o eptool ./cmd/eptool               # with_db embeds the item db
//	./eptool -list
//	./eptool -spec warrior_fury -stat hit -offsets -200,-150,-100 -iters 30000
//
// Each row sweeps the spec's own test preset off the measured stat with
// Player.BonusStats and asks core.StatWeights — the sim's own EP
// routine, normalized to the same stat the shipped table uses — so
// every row is a LOCAL slope at a known total. Rows whose finite
// difference straddles a cap read low and must not be shipped as
// slopes; that is the cap, which the engine already models.
//
// The spec table below is transcribed from the sim's own
// sim/<class>/*_test.go presets (talents, glyphs, options, consumes,
// gear set, APL) — the gear point the published weights describe.
//
// NUMERAIRE WARNING (session 39): check that your reference stat
// actually moves DPS before trusting any EP built on it. Ranged attack
// power does NOT — `BonusStats[RangedAttackPower]` leaves hunter DPS
// unchanged to the decimal in both sims, and in the CATA sim
// core.StatWeights shares the blind spot — so anything normalized to it
// there comes out ~2x high. Normalize to agility or crit instead: the
// published cata hunter table reproduces inside 5% that way
// (agility/crit 2.93 measured against its 2.79). The `-ep` mode takes
// `-ref` for exactly this.
//
// Sessions 32/33/35 used it for the warrior ArP curve and for the
// below-cap hit and expertise slopes of every WotLK melee and ranged
// DPS spec. Pick offsets that land the swept stat well clear of the
// spec's TALENT-REDUCED cap (rogue, DK, ret and enhancement presets all
// sit above their own hit caps): sweeping near a cap measures the cap,
// not the slope.

package main

import (
	"flag"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/wowsims/wotlk/sim"
	"github.com/wowsims/wotlk/sim/core"
	"github.com/wowsims/wotlk/sim/core/proto"
	"github.com/wowsims/wotlk/sim/core/stats"
)

var (
	specName = flag.String("spec", "warrior_fury", "spec to measure (see -list)")
	list     = flag.Bool("list", false, "list the known specs and exit")
	iters    = flag.Int("iters", 30000, "iterations per sim")
	statArg  = flag.String("stat", "hit", "stat to offset: hit|arp|exp|str|agi|none")
	offsets  = flag.String("offsets", "0", "comma-separated rating offsets")
)

var idx = map[string]stats.Stat{
	"str": stats.Strength, "agi": stats.Agility, "ap": stats.AttackPower,
	"hit": stats.MeleeHit, "spellhit": stats.SpellHit, "crit": stats.MeleeCrit,
	"haste": stats.MeleeHaste, "arp": stats.ArmorPenetration, "exp": stats.Expertise,
}

var report = []struct {
	name string
	st   stats.Stat
}{
	{"str", stats.Strength}, {"agi", stats.Agility}, {"ap", stats.AttackPower},
	{"rap", stats.RangedAttackPower},
	{"hit", stats.MeleeHit}, {"crit", stats.MeleeCrit}, {"haste", stats.MeleeHaste},
	{"arp", stats.ArmorPenetration}, {"exp", stats.Expertise},
}

// spec is one measurable build: the sim's own test preset (gear, talents,
// glyphs, rotation, consumes), which is the gear point the shipped
// weights claim to describe.
type spec struct {
	class   proto.Class
	race    proto.Race
	talents string
	glyphs  *proto.Glyphs
	// refStat is the EP numeraire the spec's shipped weights use (the
	// sim.ts table's 1.0 entry): attack power for melee, RANGED attack
	// power for hunters — normalizing a hunter to melee AP divides by a
	// stat its damage never reads and every EP comes out zero.
	refStat  proto.Stat
	gearDir  string
	gearSet  string
	aplDir   string
	apl      string
	consumes *proto.Consumes
	options  func() *proto.Player // fills the spec oneof
}

var meleeConsumes = &proto.Consumes{
	Flask:         proto.Flask_FlaskOfEndlessRage,
	DefaultPotion: proto.Potions_PotionOfSpeed,
	PrepopPotion:  proto.Potions_PotionOfSpeed,
	Food:          proto.Food_FoodFishFeast,
}

var hunterConsumes = &proto.Consumes{
	Flask:           proto.Flask_FlaskOfRelentlessAssault,
	DefaultPotion:   proto.Potions_HastePotion,
	DefaultConjured: proto.Conjured_ConjuredFlameCap,
	PetFood:         proto.PetFood_PetFoodKiblersBits,
}

var warriorOptions = &proto.Warrior_Options{
	StartingRage: 50, UseRecklessness: true, UseShatteringThrow: true,
	Shout: proto.WarriorShout_WarriorShoutBattle,
}

var hunterOptions = &proto.Hunter_Options{
	Ammo:    proto.Hunter_Options_SaroniteRazorheads,
	PetType: proto.Hunter_Options_Wolf,
	PetTalents: &proto.HunterPetTalents{
		CobraReflexes: 2, Dive: true, SpikedCollar: 3, BoarsSpeed: true,
		CullingTheHerd: 3, SpidersBite: 3, Rabid: true, CallOfTheWild: true, WildHunt: 1,
	},
	PetUptime: 0.9, TimeToTrapWeaveMs: 2000, SniperTrainingUptime: 0.8, UseHuntersMark: true,
}

var specs = map[string]spec{
	// dual-wield fury: white swings miss at 27% before hit, specials at 8%
	"warrior_fury": {
		class: proto.Class_ClassWarrior, race: proto.Race_RaceOrc,
		refStat: proto.Stat_StatAttackPower,
		talents: "302023102331-305053000520310053120500351",
		glyphs: &proto.Glyphs{
			Major1: int32(proto.WarriorMajorGlyph_GlyphOfWhirlwind),
			Major2: int32(proto.WarriorMajorGlyph_GlyphOfHeroicStrike),
			Major3: int32(proto.WarriorMajorGlyph_GlyphOfRending),
			Minor1: int32(proto.WarriorMinorGlyph_GlyphOfShatteringThrow)},
		gearDir: "ui/warrior/gear_sets", gearSet: "p4_fury_horde",
		aplDir: "ui/warrior/apls", apl: "fury", consumes: meleeConsumes,
		options: func() *proto.Player {
			return &proto.Player{Spec: &proto.Player_Warrior{Warrior: &proto.Warrior{Options: warriorOptions}}}
		},
	},
	// two-handed arms: no dual-wield white-miss penalty, so hit below the
	// cap is worth measuring separately from fury
	"warrior_arms": {
		class: proto.Class_ClassWarrior, race: proto.Race_RaceOrc,
		refStat: proto.Stat_StatAttackPower,
		talents: "3022032023335100102012213231251-305-2033",
		glyphs: &proto.Glyphs{
			Major1: int32(proto.WarriorMajorGlyph_GlyphOfRending),
			Major2: int32(proto.WarriorMajorGlyph_GlyphOfMortalStrike),
			Major3: int32(proto.WarriorMajorGlyph_GlyphOfExecution),
			Minor1: int32(proto.WarriorMinorGlyph_GlyphOfShatteringThrow)},
		gearDir: "ui/warrior/gear_sets", gearSet: "p4_arms_horde",
		aplDir: "ui/warrior/apls", apl: "arms", consumes: meleeConsumes,
		options: func() *proto.Player {
			return &proto.Player{Spec: &proto.Player_Warrior{Warrior: &proto.Warrior{Options: warriorOptions}}}
		},
	},
	"hunter_sv": {
		class: proto.Class_ClassHunter, race: proto.Race_RaceOrc,
		refStat: proto.Stat_StatRangedAttackPower,
		talents: "-015305101-5000032500033330532135301311",
		glyphs: &proto.Glyphs{
			Major1: int32(proto.HunterMajorGlyph_GlyphOfSerpentSting),
			Major2: int32(proto.HunterMajorGlyph_GlyphOfExplosiveShot),
			Major3: int32(proto.HunterMajorGlyph_GlyphOfKillShot)},
		gearDir: "ui/hunter/gear_sets", gearSet: "p1_sv",
		aplDir: "ui/hunter/apls", apl: "sv", consumes: hunterConsumes,
		options: func() *proto.Player {
			return &proto.Player{Spec: &proto.Player_Hunter{Hunter: &proto.Hunter{Options: hunterOptions}}}
		},
	},
	"hunter_mm": {
		class: proto.Class_ClassHunter, race: proto.Race_RaceOrc,
		refStat: proto.Stat_StatRangedAttackPower,
		talents: "502-035335131030013233035031051-5000002",
		glyphs: &proto.Glyphs{
			Major1: int32(proto.HunterMajorGlyph_GlyphOfSerpentSting),
			Major2: int32(proto.HunterMajorGlyph_GlyphOfSteadyShot),
			Major3: int32(proto.HunterMajorGlyph_GlyphOfChimeraShot)},
		gearDir: "ui/hunter/gear_sets", gearSet: "p1_mm",
		aplDir: "ui/hunter/apls", apl: "mm", consumes: hunterConsumes,
		options: func() *proto.Player {
			return &proto.Player{Spec: &proto.Player_Hunter{Hunter: &proto.Hunter{Options: hunterOptions}}}
		},
	},
	"hunter_bm": {
		class: proto.Class_ClassHunter, race: proto.Race_RaceOrc,
		refStat: proto.Stat_StatRangedAttackPower,
		talents: "51200201515012233110531351-005305-5",
		glyphs: &proto.Glyphs{
			Major1: int32(proto.HunterMajorGlyph_GlyphOfBestialWrath),
			Major2: int32(proto.HunterMajorGlyph_GlyphOfSteadyShot),
			Major3: int32(proto.HunterMajorGlyph_GlyphOfSerpentSting)},
		gearDir: "ui/hunter/gear_sets", gearSet: "p1_sv",
		aplDir: "ui/hunter/apls", apl: "bm", consumes: hunterConsumes,
		options: func() *proto.Player {
			return &proto.Player{Spec: &proto.Player_Hunter{Hunter: &proto.Hunter{Options: hunterOptions}}}
		},
	},
	"rogue_combat": {
		class: proto.Class_ClassRogue, race: proto.Race_RaceOrc,
		refStat: proto.Stat_StatAttackPower,
		talents: "00532000523-0252051050035010223100501251",
		glyphs: &proto.Glyphs{
			Major1: int32(proto.RogueMajorGlyph_GlyphOfKillingSpree),
			Major2: int32(proto.RogueMajorGlyph_GlyphOfTricksOfTheTrade),
			Major3: int32(proto.RogueMajorGlyph_GlyphOfRupture)},
		gearDir: "ui/rogue/gear_sets", gearSet: "p1_combat",
		aplDir: "ui/rogue/apls", apl: "combat_expose",
		consumes: &proto.Consumes{Flask: proto.Flask_FlaskOfEndlessRage,
			DefaultPotion:   proto.Potions_PotionOfSpeed,
			DefaultConjured: proto.Conjured_ConjuredRogueThistleTea},
		options: func() *proto.Player {
			return &proto.Player{Spec: &proto.Player_Rogue{Rogue: &proto.Rogue{
				Options: &proto.Rogue_Options{
					MhImbue: proto.Rogue_Options_DeadlyPoison,
					OhImbue: proto.Rogue_Options_InstantPoison}}}}
		},
	},
	"deathknight_frost": {
		class: proto.Class_ClassDeathknight, race: proto.Race_RaceOrc,
		refStat: proto.Stat_StatAttackPower,
		talents: "23050005-32005350352203012300033101351",
		glyphs: &proto.Glyphs{
			Major1: int32(proto.DeathknightMajorGlyph_GlyphOfFrostStrike),
			Major2: int32(proto.DeathknightMajorGlyph_GlyphOfObliterate),
			Major3: int32(proto.DeathknightMajorGlyph_GlyphOfDisease)},
		gearDir: "ui/deathknight/gear_sets", gearSet: "p3_frost",
		aplDir: "ui/deathknight/apls", apl: "frost_bl_pesti",
		consumes: &proto.Consumes{Flask: proto.Flask_FlaskOfEndlessRage,
			DefaultPotion: proto.Potions_PotionOfSpeed,
			PrepopPotion:  proto.Potions_PotionOfSpeed,
			Food:          proto.Food_FoodDragonfinFilet},
		options: func() *proto.Player {
			return &proto.Player{Spec: &proto.Player_Deathknight{Deathknight: &proto.Deathknight{
				Options: &proto.Deathknight_Options{
					UnholyFrenzyTarget: &proto.UnitReference{Type: proto.UnitReference_Player, Index: 0},
					DrwPestiApply:      true, PetUptime: 1}}}}
		},
	},
	"retribution_paladin": {
		class: proto.Class_ClassPaladin, race: proto.Race_RaceBloodElf,
		refStat: proto.Stat_StatAttackPower,
		talents: "050501-05-05232051203331302133231331",
		gearDir: "ui/retribution_paladin/gear_sets", gearSet: "p1",
		aplDir: "ui/retribution_paladin/apls", apl: "default",
		consumes: &proto.Consumes{Flask: proto.Flask_FlaskOfRelentlessAssault,
			DefaultPotion:   proto.Potions_HastePotion,
			DefaultConjured: proto.Conjured_ConjuredDarkRune,
			Food:            proto.Food_FoodRoastedClefthoof, ThermalSapper: true},
		options: func() *proto.Player {
			return &proto.Player{Spec: &proto.Player_RetributionPaladin{
				RetributionPaladin: &proto.RetributionPaladin{
					Options: &proto.RetributionPaladin_Options{
						Judgement: proto.PaladinJudgement_JudgementOfWisdom,
						Seal:      proto.PaladinSeal_Vengeance,
						Aura:      proto.PaladinAura_RetributionAura}}}}
		},
	},
	"enhancement_shaman": {
		class: proto.Class_ClassShaman, race: proto.Race_RaceOrc,
		refStat: proto.Stat_StatAttackPower,
		talents: "053030152-30405003105021333031131031051",
		glyphs: &proto.Glyphs{
			Major1: int32(proto.ShamanMajorGlyph_GlyphOfFireNova),
			Major2: int32(proto.ShamanMajorGlyph_GlyphOfFlametongueWeapon),
			Major3: int32(proto.ShamanMajorGlyph_GlyphOfFeralSpirit)},
		gearDir: "ui/enhancement_shaman/gear_sets", gearSet: "p1",
		aplDir: "ui/enhancement_shaman/apls", apl: "default_ft",
		consumes: &proto.Consumes{DefaultConjured: proto.Conjured_ConjuredFlameCap},
		options: func() *proto.Player {
			return &proto.Player{Spec: &proto.Player_EnhancementShaman{
				EnhancementShaman: &proto.EnhancementShaman{
					Options: &proto.EnhancementShaman_Options{
						Shield:   proto.ShamanShield_LightningShield,
						SyncType: proto.ShamanSyncType_Auto,
						ImbueMh:  proto.ShamanImbue_FlametongueWeaponDownrank,
						ImbueOh:  proto.ShamanImbue_FlametongueWeapon,
						Totems: &proto.ShamanTotems{
							Earth:            proto.EarthTotem_StrengthOfEarthTotem,
							Air:              proto.AirTotem_WindfuryTotem,
							Water:            proto.WaterTotem_ManaSpringTotem,
							Fire:             proto.FireTotem_MagmaTotem,
							UseFireElemental: true}}}}}
		},
	},
	"feral_druid": {
		class: proto.Class_ClassDruid, race: proto.Race_RaceTauren,
		refStat: proto.Stat_StatAttackPower,
		talents: "-503202132322010053120230310511-205503012",
		glyphs: &proto.Glyphs{
			Major1: int32(proto.DruidMajorGlyph_GlyphOfOmenOfClarity),
			Major2: int32(proto.DruidMajorGlyph_GlyphOfShred),
			Major3: int32(proto.DruidMajorGlyph_GlyphOfBerserk),
			Minor1: int32(proto.DruidMinorGlyph_GlyphOfTheWild)},
		gearDir: "ui/feral_druid/gear_sets", gearSet: "p3",
		aplDir: "ui/feral_druid/apls", apl: "default",
		consumes: &proto.Consumes{BattleElixir: proto.BattleElixir_ElixirOfMajorAgility,
			GuardianElixir: proto.GuardianElixir_ElixirOfMajorMageblood,
			Food:           proto.Food_FoodGrilledMudfish, DefaultPotion: proto.Potions_HastePotion},
		options: func() *proto.Player {
			return &proto.Player{Spec: &proto.Player_FeralDruid{FeralDruid: &proto.FeralDruid{
				Options: &proto.FeralDruid_Options{
					InnervateTarget: &proto.UnitReference{}, LatencyMs: 100, AssumeBleedActive: true},
				Rotation: &proto.FeralDruid_Rotation{
					RotationType:  proto.FeralDruid_Rotation_SingleTarget,
					BearWeaveType: proto.FeralDruid_Rotation_None,
					UseRake:       true, UseBite: true, MinCombosForRip: 5, MinCombosForBite: 5,
					BiteTime: 4.0, MaintainFaerieFire: true, BerserkBiteThresh: 25.0,
					BerserkFfThresh: 15.0, MaxFfDelay: 0.7, MinRoarOffset: 24.0,
					RipLeeway: 3, RaidTargets: 30, PrePopOoc: true}}}}
		},
	},
}

func player(sp spec, bonus stats.Stats) *proto.Player {
	p := sp.options()
	p.Race = sp.race
	p.Class = sp.class
	p.TalentsString = sp.talents
	p.Glyphs = sp.glyphs
	p.Consumes = sp.consumes
	p.Equipment = core.GetGearSet(sp.gearDir, sp.gearSet).GearSet
	p.Rotation = core.GetAplRotation(sp.aplDir, sp.apl).Rotation
	p.Buffs = core.FullIndividualBuffs
	p.BonusStats = &proto.UnitStats{Stats: bonus[:]}
	return p
}

func main() {
	flag.Parse()
	if *list {
		var names []string
		for n := range specs {
			names = append(names, n)
		}
		sort.Strings(names)
		fmt.Println(strings.Join(names, "\n"))
		return
	}
	sim.RegisterAll()

	sp, ok := specs[*specName]
	if !ok {
		panic("unknown spec " + *specName)
	}
	st, known := idx[*statArg]
	sweep := *statArg != "none"
	if sweep && !known {
		panic("unknown stat " + *statArg)
	}

	fmt.Printf("spec=%s gear=%s iters=%d sweep=%s ref=%v\n", *specName, sp.gearSet, *iters, *statArg, sp.refStat)
	fmt.Printf("%8s %10s %10s %10s  %s\n", "offset", "total-hit", "total-exp", "total-arp", "EP (AP-normalized)")
	for _, f := range strings.Split(*offsets, ",") {
		off, err := strconv.ParseFloat(strings.TrimSpace(f), 64)
		if err != nil {
			panic(err)
		}
		var bonus stats.Stats
		if sweep {
			bonus[st] = off
		}
		p := player(sp, bonus)

		cs := core.ComputeStats(&proto.ComputeStatsRequest{
			Raid: core.SinglePlayerRaidProto(p, core.FullPartyBuffs, core.FullRaidBuffs, core.FullDebuffs),
		})
		final := cs.RaidStats.Parties[0].Players[0].FinalStats.Stats

		res := core.StatWeights(&proto.StatWeightsRequest{
			Player:     player(sp, bonus),
			RaidBuffs:  core.FullRaidBuffs,
			PartyBuffs: core.FullPartyBuffs,
			Debuffs:    core.FullDebuffs,
			Encounter: &proto.Encounter{
				Duration: 300,
				Targets:  []*proto.Target{core.NewDefaultTarget()},
			},
			SimOptions: &proto.SimOptions{Iterations: int32(*iters), IsTest: true, RandomSeed: 101},
			StatsToWeigh: []proto.Stat{proto.Stat_StatStrength, proto.Stat_StatAgility,
				proto.Stat_StatAttackPower, proto.Stat_StatRangedAttackPower,
				proto.Stat_StatMeleeHit, proto.Stat_StatMeleeCrit,
				proto.Stat_StatMeleeHaste, proto.Stat_StatArmorPenetration, proto.Stat_StatExpertise},
			EpReferenceStat: sp.refStat,
		})
		ep := res.Dps.EpValues.Stats
		var b strings.Builder
		for _, r := range report {
			fmt.Fprintf(&b, "%s=%.2f ", r.name, ep[r.st])
		}
		fmt.Printf("%8.0f %10.1f %10.1f %10.1f  %s\n", off,
			final[stats.MeleeHit], final[stats.Expertise], final[stats.ArmorPenetration], b.String())
	}
}
