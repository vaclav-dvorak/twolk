package db

import (
	"regexp"
	"strconv"
	"strings"
	"time"
)

// EffectKind distinguishes how an item effect activates.
type EffectKind int

const (
	// EffectProc triggers from combat events (chance or PPM based).
	EffectProc EffectKind = iota + 1
	// EffectOnUse is an activatable cooldown.
	EffectOnUse
	// EffectCurated is a hand-maintained approximation of an effect whose
	// WoWSims implementation is custom code we cannot extract. Its Bonus
	// already holds uptime-weighted *average* stats.
	EffectCurated
)

// Effect is one special item effect (trinket proc, on-use cooldown, ...)
// extracted from WoWSims sim sources. Raw parameters are kept as-is so
// scoring policy (uptime assumptions etc.) can evolve without re-fetching.
type Effect struct {
	ItemID     int
	Name       string
	Kind       EffectKind
	Bonus      Stats         // stats granted while active (EffectCurated: average)
	Duration   time.Duration // buff duration
	Cooldown   time.Duration // on-use cooldown, or proc ICD
	ProcChance float64       // fraction; 0 when PPM-based or unknown
	PPM        float64       // procs per minute; 0 when chance-based
	Callback   string        // raw trigger, e.g. "OnSpellHitDealt"
	ProcMask   string        // raw trigger mask, e.g. "MeleeOrRanged"
	Outcome    string        // raw required outcome, e.g. "Crit"
	MaxStacks  int           // stacking buffs: Bonus is PER STACK; 0 = non-stacking

	// BonusDPS is flat damage-per-second the effect adds directly,
	// for procs that deal damage rather than grant stats (EffectCurated
	// only — the upstream db carries no damage-proc data). It is an
	// uptime-weighted average like Bonus, and it is priced through the
	// profile's EPPerDPS rather than any stat weight, because the
	// damage is flat: it neither scales with the character's stats nor
	// benefits from raid buffs.
	BonusDPS float64
}

// statIdents maps WoWSims stats-package identifiers to our Stat indices.
var statIdents = map[string]Stat{
	"Strength": Strength, "Agility": Agility, "Stamina": Stamina,
	"Intellect": Intellect, "Spirit": Spirit, "SpellPower": SpellPower,
	"MP5": MP5, "SpellHit": SpellHit, "SpellCrit": SpellCrit,
	"SpellHaste": SpellHaste, "SpellPenetration": SpellPenetration,
	"AttackPower": AttackPower, "MeleeHit": MeleeHit, "MeleeCrit": MeleeCrit,
	"MeleeHaste": MeleeHaste, "ArmorPenetration": ArmorPenetration,
	"Expertise": Expertise, "Mana": Mana, "Energy": Energy, "Rage": Rage,
	"Armor": Armor, "RangedAttackPower": RangedAttackPower,
	"Defense": Defense, "Block": Block, "BlockValue": BlockValue,
	"Dodge": Dodge, "Parry": Parry, "Resilience": Resilience,
	"Health": Health, "ArcaneResistance": ArcaneResistance,
	"FireResistance": FireResistance, "FrostResistance": FrostResistance,
	"NatureResistance": NatureResistance, "ShadowResistance": ShadowResistance,
	"BonusArmor": BonusArmor, "Mastery": Mastery,
	// cata sim sources name some ratings differently
	"DodgeRating": Dodge, "ParryRating": Parry, "MasteryRating": Mastery,
}

var (
	durationRe    = regexp.MustCompile(`time\.(\w+)(?:\s*\*\s*([0-9.]+))?`)
	durationPreRe = regexp.MustCompile(`([0-9.]+)\s*\*\s*time\.(\w+)`)
	statPairRe    = regexp.MustCompile(`stats\.(\w+):\s*([0-9.]+)`)

	procNameRe    = regexp.MustCompile(`Name:\s*"([^"]*)"`)
	procIDRe      = regexp.MustCompile(`(?m)^\s*(?:Item)?ID:\s*(\d+)`) // wotlk says ID:, cata ItemID:
	procBonusRe   = regexp.MustCompile(`Bonus:\s*stats\.Stats\{([^}]*)\}`)
	procDurRe     = regexp.MustCompile(`(?m)^\s*Duration:\s*(.+?),?\s*$`)
	procICDRe     = regexp.MustCompile(`(?m)^\s*ICD:\s*(.+?),?\s*$`)
	procChanceRe  = regexp.MustCompile(`ProcChance:\s*([0-9.]+)`)
	procPPMRe     = regexp.MustCompile(`PPM:\s*([0-9.]+)`)
	procCbRe      = regexp.MustCompile(`(?m)^\s*Callback:\s*(.+?),?\s*$`)
	procMaskRe    = regexp.MustCompile(`(?m)^\s*ProcMask:\s*(.+?),?\s*$`)
	procOutcomeRe = regexp.MustCompile(`(?m)^\s*Outcome:\s*(.+?),?\s*$`)

	onUseDefRe   = regexp.MustCompile(`new(\w+) := testFirstOnly\(`)
	onUseBonusRe = regexp.MustCompile(`stats\.(\w+):\s*bonus`)
	onUseCallRe  = regexp.MustCompile(`(?m)^\s*new(\w+)\((\d+),\s*([0-9.]+),\s*([^,]+),\s*([^)]+)\)\s*(?://\s*(.*))?$`)

	stackMaxRe = regexp.MustCompile(`MaxStacks:\s*(\d+)`)
	stackIcdRe = regexp.MustCompile(`(?m)^\s*Icd:\s*(.+?),?\s*$`) // cata spells it Icd, wotlk ICD
	stackCDRe  = regexp.MustCompile(`(?m)^\s*CD:\s*(.+?),?\s*$`)
	// the Dragon Soul trinkets declare all three difficulty versions in
	// one literal, indexed by version: []int32{lfr, normal, heroic} ids
	// and matching []float64 per-stack bonuses
	stackVerIDsRe   = regexp.MustCompile(`ItemID:\s*\[\]int32\{([\d,\s]+)\}`)
	stackVerBonusRe = regexp.MustCompile(`Bonus:\s*stats\.Stats\{stats\.(\w+):\s*\[\]float64\{([\d.,\s]+)\}`)
)

// parseGoDuration evaluates simple Go duration expressions as they appear
// in the WoWSims sources: "time.Second * 10", "10 * time.Second",
// "time.Minute". Unknown shapes yield 0.
func parseGoDuration(expr string) time.Duration {
	expr = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(expr), ","))
	unit, mult := "", 1.0
	if m := durationPreRe.FindStringSubmatch(expr); m != nil {
		mult, _ = strconv.ParseFloat(m[1], 64)
		unit = m[2]
	} else if m := durationRe.FindStringSubmatch(expr); m != nil {
		unit = m[1]
		if m[2] != "" {
			mult, _ = strconv.ParseFloat(m[2], 64)
		}
	}
	base := map[string]time.Duration{
		"Millisecond": time.Millisecond, "Second": time.Second,
		"Minute": time.Minute, "Hour": time.Hour,
	}[unit]
	return time.Duration(mult * float64(base))
}

// stripCorePrefixes reduces "core.CallbackOnSpellHitDealt | core.Callback..."
// style expressions to their bare enum names.
func stripCorePrefixes(expr string) string {
	for _, p := range []string{"core.Callback", "core.ProcMask", "core.Outcome"} {
		expr = strings.ReplaceAll(expr, p, "")
	}
	return strings.TrimSpace(expr)
}

// ParseProcEffects extracts ProcStatBonusEffect declarations from the
// WoWSims stat_bonus_procs.go source.
func ParseProcEffects(src []byte) []*Effect {
	var effects []*Effect
	lines := strings.Split(string(src), "\n")
	for i := 0; i < len(lines); i++ {
		if !strings.Contains(lines[i], "ProcStatBonusEffect{") ||
			strings.HasPrefix(strings.TrimSpace(lines[i]), "//") {
			continue
		}
		var block []string
		for ; i < len(lines); i++ {
			block = append(block, lines[i])
			if regexp.MustCompile(`^\s*\}\)`).MatchString(lines[i]) {
				break
			}
		}
		e := parseProcBlock(strings.Join(block, "\n"))
		if e != nil {
			effects = append(effects, e)
		}
	}
	return effects
}

func parseProcBlock(block string) *Effect {
	idm := procIDRe.FindStringSubmatch(block)
	if idm == nil {
		return nil
	}
	id, _ := strconv.Atoi(idm[1])
	e := &Effect{ItemID: id, Kind: EffectProc}
	if m := procNameRe.FindStringSubmatch(block); m != nil {
		e.Name = m[1]
	}
	if m := procBonusRe.FindStringSubmatch(block); m != nil {
		for _, pair := range statPairRe.FindAllStringSubmatch(m[1], -1) {
			if st, ok := statIdents[pair[1]]; ok {
				v, _ := strconv.ParseFloat(pair[2], 64)
				e.Bonus[st] = v
			}
		}
	}
	if m := procDurRe.FindStringSubmatch(block); m != nil {
		e.Duration = parseGoDuration(m[1])
	}
	if m := procICDRe.FindStringSubmatch(block); m != nil {
		e.Cooldown = parseGoDuration(m[1])
	}
	if m := procChanceRe.FindStringSubmatch(block); m != nil {
		e.ProcChance, _ = strconv.ParseFloat(m[1], 64)
	}
	if m := procPPMRe.FindStringSubmatch(block); m != nil {
		e.PPM, _ = strconv.ParseFloat(m[1], 64)
	}
	if m := procCbRe.FindStringSubmatch(block); m != nil {
		e.Callback = stripCorePrefixes(m[1])
	}
	if m := procMaskRe.FindStringSubmatch(block); m != nil {
		e.ProcMask = stripCorePrefixes(m[1])
	}
	if m := procOutcomeRe.FindStringSubmatch(block); m != nil {
		e.Outcome = stripCorePrefixes(m[1])
	}
	return e
}

// ParseStackingEffects extracts the stacking-buff declarations from the
// WoWSims stat_bonus_stacking.go source (both games' dialects: wotlk
// calls newStackingStatBonusEffect/CD with bare literals, cata calls
// shared.NewStackingStatBonusEffect/CD). Each entry's Bonus is the
// PER-STACK gain, with MaxStacks recording how high it builds; the
// ...CD variants are player-activated (EffectOnUse), building their
// stacks inside the buff window. Fully custom effects (raw
// core.NewItemEffect code) are not declarations and are skipped.
func ParseStackingEffects(src []byte) []*Effect {
	var effects []*Effect
	lines := strings.Split(string(src), "\n")
	for i := 0; i < len(lines); i++ {
		trimmed := strings.TrimSpace(lines[i])
		// only literal call sites open a block — the wotlk dialect also
		// mentions the type names in definitions and func signatures
		if strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "type ") ||
			strings.HasPrefix(trimmed, "func ") {
			continue
		}
		cd := strings.Contains(lines[i], "StackingStatBonusCD{")
		if !cd && !strings.Contains(lines[i], "StackingStatBonusEffect{") {
			continue
		}
		var block []string
		for ; i < len(lines); i++ {
			block = append(block, lines[i])
			if regexp.MustCompile(`^\s*\}\)`).MatchString(lines[i]) {
				break
			}
		}
		effects = append(effects, parseStackingBlock(strings.Join(block, "\n"), cd)...)
	}
	return effects
}

// parseStackingBlock decodes one stacking declaration. A versioned
// block (indexed []int32 item ids) yields one effect per difficulty
// version; anything without a readable item id yields none.
func parseStackingBlock(block string, cd bool) []*Effect {
	e := &Effect{Kind: EffectProc}
	if cd {
		e.Kind = EffectOnUse
	}
	if m := procNameRe.FindStringSubmatch(block); m != nil {
		e.Name = m[1]
	}
	if m := stackMaxRe.FindStringSubmatch(block); m != nil {
		e.MaxStacks, _ = strconv.Atoi(m[1])
	}
	if m := procBonusRe.FindStringSubmatch(block); m != nil {
		for _, pair := range statPairRe.FindAllStringSubmatch(m[1], -1) {
			if st, ok := statIdents[pair[1]]; ok {
				v, _ := strconv.ParseFloat(pair[2], 64)
				e.Bonus[st] = v
			}
		}
	}
	if m := procDurRe.FindStringSubmatch(block); m != nil {
		e.Duration = parseGoDuration(m[1])
	}
	switch {
	case cd: // on-use cooldown; any Icd on a CD block is irrelevant
		if m := stackCDRe.FindStringSubmatch(block); m != nil {
			e.Cooldown = parseGoDuration(m[1])
		}
	default:
		if m := procICDRe.FindStringSubmatch(block); m != nil {
			e.Cooldown = parseGoDuration(m[1])
		} else if m := stackIcdRe.FindStringSubmatch(block); m != nil {
			e.Cooldown = parseGoDuration(m[1])
		}
	}
	if m := procChanceRe.FindStringSubmatch(block); m != nil {
		e.ProcChance, _ = strconv.ParseFloat(m[1], 64)
	}
	if m := procPPMRe.FindStringSubmatch(block); m != nil {
		e.PPM, _ = strconv.ParseFloat(m[1], 64)
	}
	if m := procCbRe.FindStringSubmatch(block); m != nil {
		e.Callback = stripCorePrefixes(m[1])
	}
	if m := procMaskRe.FindStringSubmatch(block); m != nil {
		e.ProcMask = stripCorePrefixes(m[1])
	}
	if m := procOutcomeRe.FindStringSubmatch(block); m != nil {
		e.Outcome = stripCorePrefixes(m[1])
	}

	if m := procIDRe.FindStringSubmatch(block); m != nil {
		e.ItemID, _ = strconv.Atoi(m[1])
		return []*Effect{e}
	}
	ids := stackVerIDsRe.FindStringSubmatch(block)
	if ids == nil {
		return nil
	}
	var out []*Effect
	bonus := stackVerBonusRe.FindStringSubmatch(block)
	for vi, id := range splitCSV(ids[1]) {
		ve := *e
		ve.ItemID, _ = strconv.Atoi(id)
		if bonus != nil {
			if st, ok := statIdents[bonus[1]]; ok {
				if vals := splitCSV(bonus[2]); vi < len(vals) {
					v, _ := strconv.ParseFloat(vals[vi], 64)
					ve.Bonus = Stats{}
					ve.Bonus[st] = v
				}
			}
		}
		out = append(out, &ve)
	}
	return out
}

// splitCSV splits a comma-separated literal list into trimmed tokens.
func splitCSV(s string) []string {
	var out []string
	for _, t := range strings.Split(s, ",") {
		if t = strings.TrimSpace(t); t != "" {
			out = append(out, t)
		}
	}
	return out
}

// ParseOnUseEffects extracts the on-use trinket declarations from the
// WoWSims stat_bonus_cds.go source. The file defines small factory
// closures (newHasteActive, ...) that bind a stat set, then lists one
// call per item; we recover the stat set per factory, then the calls.
func ParseOnUseEffects(src []byte) []*Effect {
	text := string(src)

	// Pass 1: factory name → stats it grants. A factory's region extends
	// to the next factory definition.
	defs := onUseDefRe.FindAllStringSubmatchIndex(text, -1)
	factoryStats := map[string][]Stat{}
	for i, d := range defs {
		name := text[d[2]:d[3]]
		end := len(text)
		if i+1 < len(defs) {
			end = defs[i+1][0]
		}
		region := text[d[0]:end]
		var sts []Stat
		seen := map[Stat]bool{}
		for _, m := range onUseBonusRe.FindAllStringSubmatch(region, -1) {
			if st, ok := statIdents[m[1]]; ok && !seen[st] {
				seen[st] = true
				sts = append(sts, st)
			}
		}
		factoryStats[name] = sts
	}

	// Pass 2: the calls.
	var effects []*Effect
	for _, m := range onUseCallRe.FindAllStringSubmatch(text, -1) {
		sts := factoryStats[m[1]]
		if len(sts) == 0 {
			continue
		}
		id, _ := strconv.Atoi(m[2])
		bonus, _ := strconv.ParseFloat(m[3], 64)
		e := &Effect{
			ItemID:   id,
			Name:     strings.TrimSpace(m[6]),
			Kind:     EffectOnUse,
			Duration: parseGoDuration(m[4]),
			Cooldown: parseGoDuration(m[5]),
		}
		for _, st := range sts {
			e.Bonus[st] = bonus
		}
		effects = append(effects, e)
	}
	return effects
}
