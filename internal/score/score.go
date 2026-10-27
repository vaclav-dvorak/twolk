// Package score turns items into numbers for a given build: AMR-style
// equivalence-point scoring driven by per-build stat weights.
//
// The model, and its stated assumptions:
//   - every socket holds the best matching gem of the endgame tier
//     (epic cuts; rare diamonds for metas), so socket bonuses always apply
//     — except unique-equipped gems (Nightmare Tear), which fill at most
//     one socket per item (set-wide uniqueness enforced only where the
//     whole set is visible: GemTips and OptimizeGems);
//   - proc/on-use effects count as uptime-averaged passive stats;
//   - stat caps (e.g. hit) apply to gear totals, so scoring a set is the
//     primary operation and an item's value is context-dependent —
//     exactly why upgrade ranking works on set deltas, not item scores.
package score

import (
	"cmp"
	"math"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/vaclav-dvorak/twolk/internal/db"
)

// Global assumptions for converting proc parameters into average stats.
// Deliberately crude; tune freely — they only need to be fair across
// items compared against each other.
const (
	// defaultProcInterval approximates how often a proc gets a chance to
	// fire (attack/cast cadence).
	defaultProcInterval = 2 * time.Second
	// assumedCritRate discounts procs that only fire on critical strikes.
	assumedCritRate = 0.35
	// assumedFightLength amortizes a stacking buff's ramp-up: stacks
	// build once per trigger, so full value only holds after
	// MaxStacks×wait seconds of a fight this long.
	assumedFightLength = 5 * time.Minute
)

// BuckleName is the Eternal Belt Buckle (item 41611): blacksmith-crafted
// but usable by ANYONE, it permanently adds one colorless socket to any
// waist item. Cheap and universal endgame practice, so the assumption
// model gives EVERY waist item the extra socket (candidates and
// detail-less worn belts) — the same stance as the best-enchant
// assumption. An enriched import whose belt shows no gem beyond its own
// sockets scores bare; that asymmetry is what drives SocketTips. The
// buckle itself is not in the item database (neither gear, gem nor
// enchant) — only its socket is modeled. Blacksmiths get the same
// treatment on bracers and gloves (Socket Bracer / Socket Gloves are
// self-only, so those two ARE profession-gated).
const BuckleName = "Eternal Belt Buckle"

// ExtraSocketName names the enhancement that adds an item's extra
// socket: the version's belt buckle (anyone) or the blacksmith-only
// bracer/glove sockets. "" for slots with no such enhancement.
func (r *Rules) ExtraSocketName(slot db.Slot) string {
	switch slot {
	case db.SlotWaist:
		return r.BuckleName
	case db.SlotWrist:
		return "Socket Bracer (Blacksmithing)"
	case db.SlotHands:
		return "Socket Gloves (Blacksmithing)"
	}
	return ""
}

// AssumesExtraSocket reports whether the assumption model grants the
// item an extra socket: every belt takes the buckle; bracers and gloves
// only when the character is a blacksmith (Socket Bracer/Gloves are
// self-cast). A worn extra gem still COUNTS without the profession —
// sockets outlive unlearning it — it just stops being assumed/advised.
func (s *Scorer) AssumesExtraSocket(it *db.Item) bool {
	switch it.Slot {
	case db.SlotWaist:
		return true
	case db.SlotWrist, db.SlotHands:
		return s.professions["Blacksmithing"]
	}
	return false
}

// assumedSockets is the socket list the assumption model works with:
// the item's own sockets, plus the extra enhancement socket where
// AssumesExtraSocket applies. The extra socket is modeled as prismatic
// — any gem fits and it never breaks the socket-bonus match, exactly
// the in-game behavior (the gem's own color still counts toward meta
// activation).
func (s *Scorer) assumedSockets(it *db.Item) []db.GemColor {
	if !s.AssumesExtraSocket(it) {
		return it.GemSockets
	}
	return append(append([]db.GemColor(nil), it.GemSockets...), db.GemPrismatic)
}

// HasExtraGem reports whether an enriched worn instance carries an
// enhancement socket's extra gem: a nonzero gem beyond the item's own
// sockets (the armory export pads the gem list with zeros, so length
// alone means nothing). Caveat: an applied-but-EMPTY extra socket is
// indistinguishable from a missing one in the armory data.
func HasExtraGem(w Worn) bool {
	for i := len(w.Item.GemSockets); i < len(w.Gems); i++ {
		if w.Gems[i] != 0 {
			return true
		}
	}
	return false
}

// StatCap marks a stat worthless beyond a threshold (gear totals).
// A nonzero SoftWeight makes it a SOFT cap instead: points past the
// threshold are worth SoftWeight each rather than nothing (rotation
// breakpoints — e.g. cata SV haste at 757 fits the Cobra Shot filler
// cycle exactly; past it haste only speeds autos and focus regen).
type StatCap struct {
	Stat       db.Stat
	Cap        float64
	SoftWeight float64
}

// StatCurve prices a stat whose marginal value is NOT constant, as a
// measured curve of EP-per-point against the character's TOTAL of that
// stat. Armor penetration is the WotLK case: its marginal value rises
// as the total climbs (each point removes a larger share of what armor
// is left), so one weight is wrong everywhere except the gear point it
// was measured at — and a flat weight makes the solver undervalue an
// ArP piece exactly when the build is closest to wanting it.
//
// Points are marginal EP at a total, ascending; between them the slope
// interpolates linearly, outside the range the nearest point holds. A
// total's value is the INTEGRAL from zero, so the curve stays additive
// across items and every existing delta (MarginalValue, set deltas)
// keeps working unchanged. Curves are measured against the sim, never
// assumed — see profiles.go for the harness and the numbers.
type StatCurve []CurvePoint

// CurvePoint is one measurement: at a stat total of At, the next point
// of that stat is worth Weight EP.
type CurvePoint struct {
	At     float64
	Weight float64
}

// curveFor resolves the curve in force for a stat: the set-dependent
// override where Calibrate computed one, else the profile's measured
// curve.
func (s *Scorer) curveFor(st db.Stat) (StatCurve, bool) {
	if c, ok := s.curves[st]; ok {
		return c, true
	}
	c, ok := s.prof.Curves[st]
	return c, ok
}

// weightAt is the curve's marginal EP at a stat total.
func (c StatCurve) weightAt(v float64) float64 {
	if len(c) == 0 {
		return 0
	}
	if v <= c[0].At {
		return c[0].Weight
	}
	for i := 1; i < len(c); i++ {
		if v <= c[i].At {
			p, q := c[i-1], c[i]
			if q.At == p.At {
				return q.Weight
			}
			return p.Weight + (q.Weight-p.Weight)*(v-p.At)/(q.At-p.At)
		}
	}
	return c[len(c)-1].Weight
}

// integral is the total value of holding v points of the stat: the
// area under the marginal-EP curve from zero to v.
func (c StatCurve) integral(v float64) float64 {
	if len(c) == 0 || v <= 0 {
		return 0
	}
	var sum, x float64
	for i := range c {
		if v <= c[i].At {
			break
		}
		if i == 0 {
			sum += c[0].Weight * c[0].At // flat below the first point
			x = c[0].At
			continue
		}
		p, q := c[i-1], c[i]
		sum += (p.Weight + q.Weight) / 2 * (q.At - p.At) // trapezoid
		x = q.At
	}
	if v > x {
		// the segment v falls in (or past the last point, flat)
		sum += (c.weightAt(x) + c.weightAt(v)) / 2 * (v - x)
	}
	return sum
}

// deflateAbove returns the curve with every point from `at` upward
// scaled by factor, with the step inserted as a one-point discontinuity.
// It models a stat whose top range is only worth a SHARE of its slope —
// armor penetration above the ceiling a passive ArP proc leaves, where
// the points only pay while the proc is down (see Calibrate).
func (c StatCurve) deflateAbove(at, factor float64) StatCurve {
	if len(c) == 0 || factor >= 1 {
		return c
	}
	out := make(StatCurve, 0, len(c)+2)
	for _, p := range c {
		if p.At < at {
			out = append(out, p)
		}
	}
	// the knee: full value up to `at`, the deflated share just past it
	out = append(out,
		CurvePoint{At: at, Weight: c.weightAt(at)},
		CurvePoint{At: at + 1, Weight: c.weightAt(at) * factor})
	for _, p := range c {
		if p.At > at+1 {
			out = append(out, CurvePoint{At: p.At, Weight: p.Weight * factor})
		}
	}
	return out
}

// Profile describes how one build values stats: weights per stat
// point, in the spec's sim-native scale (DPS-derived for damage specs,
// HPS for healers, survival/threat for tanks — see profiles.go).
//
// Unified-rating note: items duplicate hit/crit/haste into spell and
// melee slots, and AP into AttackPower/RangedAttackPower. Single-role
// specs weight one side (the one their build reads); hybrid specs
// weight BOTH sides deliberately — the rating's full value is the sum
// of what each side contributes.
type Profile struct {
	Name    string
	Class   db.Class
	Tree    string // talent tree name as the armory exports it
	Weights db.Stats
	Caps    []StatCap   // static caps (e.g. ArP); see also HitCap
	HitCap  *HitCapSpec // hit cap, adjusted per character via EffectiveCaps

	// Curves overrides Weights for stats whose marginal EP depends on
	// the character's total of that stat (ArP). Weights still carries a
	// linear stand-in for the same stat — Calibrate refreshes it to the
	// curve's LOCAL slope at the solved set — because the gem search
	// and the weights screen need one number per stat.
	Curves map[db.Stat]StatCurve

	// Weapon damage value, in EP per weapon DPS, for the melee and
	// ranged slots. For physical DPS builds weapon DPS dwarfs the stats
	// on the weapon itself (a hunter's ranged DPS ≈ 5+ EP per point);
	// casters/healers leave these zero — they only care about the
	// weapon's stats.
	WeaponDPSWeight float64
	RangedDPSWeight float64

	// EPPerDPS converts flat damage-per-second into EP, for damage
	// procs the stat weights cannot price (Effect.BonusDPS). It is the
	// reciprocal of the spec's DPS per point of its EP numeraire, and
	// so must be MEASURED against the sim, not assumed; zero leaves
	// damage procs unpriced rather than guessed at.
	EPPerDPS float64

	// ProcInterval is the assumed time between chances for a proc to
	// fire — how often the build lands an attack the trigger watches.
	// Zero falls back to defaultProcInterval. Specs differ by a lot
	// (an SV hunter lands ~1.2 attacks/sec), and the assumption drives
	// every chance-based proc's uptime, so set it from measurement.
	ProcInterval time.Duration

	// NoPeriodicDamage marks a rotation that deals no periodic damage
	// (arcane mage: Arcane Blast and Missiles never tick in the sim), so
	// procs that fire ONLY on periodic damage never fire for it.
	NoPeriodicDamage bool

	// Filters for upgrade proposals (zero values = no filter).
	ArmorType   db.ArmorType // only this armor class on armor slots
	WeaponTypes []db.WeaponType
	RangedTypes []db.RangedWeaponType

	// Weapon layouts the build can play; BestSet solves each and keeps
	// the best. Empty = solve the worn shape as-is.
	Layouts []WeaponLayout

	// Rules is the game version's rule set (nil = WotLK, the founding
	// version whose tables live in this package).
	Rules *Rules

	// Role tells apart builds sharing one talent tree (cata feral: cat
	// DPS "" vs "bear" tank); it extends the config key.
	Role string

	// Conversions are stat dependencies: a share of one stat counts as
	// another (cata Balance of Power: spirit from gear is spell hit).
	// Caps are measured on the CONVERTED totals, so spirit fills the
	// hit cap; the source stat's weight must already carry its
	// converted value (spirit weighted like hit below the cap).
	Conversions []StatConversion
}

// StatConversion adds Ratio × From to To before caps are applied.
type StatConversion struct {
	From, To db.Stat
	Ratio    float64
}

// WeaponDPS is the item's average weapon damage per second (0 for
// non-weapons).
func WeaponDPS(it *db.Item) float64 {
	if it.WeaponSpeed <= 0 {
		return 0
	}
	return (it.WeaponDmgMin + it.WeaponDmgMax) / 2 / it.WeaponSpeed
}

// itemBonusValue is the EP an item carries OUTSIDE its stat line: its
// weapon DPS, plus any flat damage its effects deal. Both are priced
// per-item rather than through the stat totals, so every scoring path
// that sums stats must add this term too.
func (s *Scorer) itemBonusValue(it *db.Item) float64 {
	var v float64
	switch it.Slot {
	case db.SlotWeapon:
		v = WeaponDPS(it) * s.prof.WeaponDPSWeight
	case db.SlotRanged:
		v = WeaponDPS(it) * s.prof.RangedDPSWeight
	}
	return v + s.effectDPSValue(it)
}

// effectDPSValue prices an item's damage procs. Flat damage does not
// scale with stats, so it cannot be folded into the stat totals; it is
// converted straight to EP through the profile's measured EPPerDPS,
// and is worth nothing to a build whose attacks cannot trigger it.
func (s *Scorer) effectDPSValue(it *db.Item) float64 {
	if s.prof.EPPerDPS == 0 {
		return 0
	}
	var dps float64
	for _, e := range s.d.Effects[it.ID] {
		if e.BonusDPS != 0 && s.prof.CanTrigger(e) {
			dps += e.BonusDPS
		}
	}
	return dps * s.prof.EPPerDPS
}

// Scorer scores items and sets for one profile against one database.
type Scorer struct {
	d           *db.Database
	prof        *Profile
	race        string                 // for weapon-type racials; see SetRace
	professions map[string]bool        // gates profession-locked enchants; see SetProfessions
	serverPhase int                    // content ceiling on enchant picks (0 = none); see SetServerPhase
	enchantOK   func(*db.Enchant) bool // realm gate on enchant picks (nil = all); see SetEnchantGate
	gemWeights  db.Stats               // weights for gem/enchant assumptions; see Calibrate
	bestGem     map[db.GemColor]*db.Gem
	bestAny     *db.Gem   // best gem for a colored socket, color ignored
	bestPrism   *db.Gem   // best prismatic (meta-keeper upgrades)
	metaGems    []*db.Gem // every endgame meta, best first — activation-aware picks walk this
	// non-unique runners-up: the gems a socket falls back to when the
	// best pick is unique-equipped (Nightmare Tear) and already spent
	safeGem   map[db.GemColor]*db.Gem
	safeAny   *db.Gem
	safePrism *db.Gem
	// best jewelcrafter-only Dragon's Eye per socket color — consulted
	// only when the character is a jewelcrafter (see jc)
	bestJC    map[db.GemColor]*db.Gem
	bestJCAny *db.Gem
	eff       map[*db.Item]db.Stats // memoized effective stats per item (suffix variants share an ID)
	// baseline is the character's non-gear stats (consumables); every
	// stat total carries it so caps and curves see the buffed
	// character. See consumes.go.
	baseline db.Stats
	// curves overrides the profile's stat curves for THIS set — the ArP
	// proc ceiling, which depends on what the character wears and so
	// cannot live on the profile. Set by Calibrate.
	curves map[db.Stat]StatCurve
	// capSteps groups the profile's caps per stat, thresholds ascending
	// (a stat may carry several: a soft cap, then a hard one)
	capSteps []capStep
}

// capStep is one capped stat's ascending cap list.
type capStep struct {
	stat db.Stat
	caps []StatCap
}

func stepCaps(caps []StatCap) []capStep {
	var out []capStep
	for _, c := range caps {
		i := slices.IndexFunc(out, func(g capStep) bool { return g.stat == c.Stat })
		if i < 0 {
			out = append(out, capStep{stat: c.Stat})
			i = len(out) - 1
		}
		out[i].caps = append(out[i].caps, c)
	}
	for i := range out {
		slices.SortFunc(out[i].caps, func(a, b StatCap) int { return cmp.Compare(a.Cap, b.Cap) })
	}
	return out
}

// Derived applies the profile's stat conversions to a total — the
// numbers caps are measured on (and the caps row displays): a balance
// druid's hit includes the spirit Balance of Power converts.
func (s *Scorer) Derived(tot db.Stats) db.Stats {
	for _, c := range s.prof.Conversions {
		tot[c.To] += c.Ratio * tot[c.From]
	}
	return tot
}

// jewelerGemLimit is the in-game "Jeweler's Gems (3)" rule: at most
// three Dragon's Eyes equipped, duplicates of the same cut allowed.
// NOT in the upstream data (all cuts carry unique=false) — enforced
// set-wide by GemTips and OptimizeGems.
const jewelerGemLimit = 3

// isJewelerGem identifies jewelcrafter-only cuts (WotLK Dragon's Eyes,
// Cata Chimera's Eyes) by the data's profession lock.
func isJewelerGem(g *db.Gem) bool {
	return g != nil && g.RequiredProfession == 7 // CraftedSource enum: Jewelcrafting
}

// jc reports whether the character can use Dragon's Eyes.
func (s *Scorer) jc() bool { return s.professions["Jewelcrafting"] }

func New(d *db.Database, prof *Profile) *Scorer {
	// a profile straight from the registry carries its hit cap in
	// HitCap (it is per-CHARACTER — talents and racials lower it) and
	// not yet in Caps, which EffectiveCaps fills in for the active
	// character. Score it without one and hit, priced at its pre-cap
	// slope, would be worth stacking forever; default to the unreduced
	// cap so every scorer caps hit somewhere.
	if prof.HitCap != nil {
		capped := false
		for _, c := range prof.Caps {
			capped = capped || c.Stat == prof.HitCap.Stat
		}
		if !capped {
			p := *prof
			p.Caps = append(append([]StatCap(nil), prof.Caps...),
				StatCap{Stat: prof.HitCap.Stat, Cap: prof.HitCap.baseRating(prof.rules())})
			prof = &p
		}
	}
	s := &Scorer{d: d, prof: prof, gemWeights: prof.Weights, eff: map[*db.Item]db.Stats{},
		capSteps: stepCaps(prof.Caps)}
	s.pickGems()
	return s
}

// SetRace enables weapon-type racials (Orc axes, Human swords/maces,
// Troll bows, Dwarf guns) as virtual stats on matching weapons.
func (s *Scorer) SetRace(race string) {
	if s.race == race {
		return
	}
	s.race = race
	s.eff = map[*db.Item]db.Stats{}
}

// SetProfessions declares the character's professions (display names),
// gating profession-locked enchants in the best-enchant assumption.
func (s *Scorer) SetProfessions(p map[string]bool) {
	s.professions = p
	s.eff = map[*db.Item]db.Stats{}
}

// SetServerPhase caps enchant picks at the server's content phase —
// the same ceiling the item filter applies, mirrored here because
// enchants never pass through the candidate filter (0 = no ceiling).
func (s *Scorer) SetServerPhase(p int) {
	if s.serverPhase == p {
		return
	}
	s.serverPhase = p
	s.eff = map[*db.Item]db.Stats{}
}

// SetEnchantGate hides enchants the realm can't get yet (a recipe
// behind a closed in-phase unlock); nil admits every enchant. Like the
// phase ceiling it must be mirrored here — enchants never pass through
// the candidate filter.
func (s *Scorer) SetEnchantGate(ok func(*db.Enchant) bool) {
	s.enchantOK = ok
	s.eff = map[*db.Item]db.Stats{}
}

// Calibrate adapts the gem and enchant assumptions to a gear set (the
// BestEnchant pick follows the same calibrated weights). Two rules per capped
// stat: (1) the set saturates the cap without any gem contribution →
// the stat's gems are worthless, re-pick with its weight zeroed; (2)
// under the cap, compare the whole-set score of both assumptions —
// keeping the stat makes EVERY socket chase the remaining gap and
// overshoot past the cap (wasted), zeroing forfeits the small gap but
// frees every socket for uncapped value — and keep the better one.
// Rule 2 is what stops a near-capped set from assuming hit gems in
// every socket of every candidate; the single gem that would land the
// cap exactly is the gem optimizer's job (OptimizeGems), not the
// assumption model's.
func (s *Scorer) Calibrate(set []*db.Item) {
	w := s.prof.Weights
	// a curved stat (ArP) has no single weight: give the gem search the
	// curve's slope AT THIS SET's total, which is the rate a gem of it
	// would actually pay here. Set valuation stays on the integral.
	for st := range s.prof.Curves {
		c, _ := s.curveFor(st)
		w[st] = c.weightAt(s.Totals(set)[st])
	}
	// The saturation test runs on the set's REAL totals minus what gems
	// contribute — the same basis everything else scores on. Item stats
	// alone are not that basis: they miss enchants, the cap-aware
	// reforge plan and the consumables baseline, which on a Cataclysm
	// character is most of the hit on the sheet. A capped set then reads
	// as having headroom and every socket of every candidate gets a hit
	// gem it cannot use (user report, session 34: a hunter at 965/961
	// hit was calibrated against 737).
	base := s.AssumedTotals(set)
	for _, it := range set {
		gems, matched := s.AssumedGems(it)
		for _, g := range gems {
			if g != nil {
				base = sub(base, g.Stats)
			}
		}
		if matched {
			base = sub(base, it.SocketBonus) // no gems, no socket bonus
		}
	}
	s.calibrateProcCeiling(set)
	saturated := map[db.Stat]bool{}
	derived := s.Derived(base)
	for _, g := range s.capSteps {
		for _, c := range g.caps { // ascending: the highest passed tier wins
			if derived[c.Stat] >= c.Cap {
				w[c.Stat] = c.SoftWeight // hard caps: zero; soft caps: residual
				saturated[c.Stat] = true
			}
		}
	}
	// a stat converting into a saturated one keeps only its own value
	for _, cv := range s.prof.Conversions {
		if saturated[cv.To] {
			w[cv.From] = max(w[cv.From]-cv.Ratio*s.prof.Weights[cv.To], 0)
		}
	}
	// under-cap policy comparison, per capped stat gems can carry;
	// ties (e.g. a socketless set) keep the stat weighted, preserving
	// the plain under-cap behavior
	var cands []StatCap
	for _, c := range s.prof.Caps {
		if !saturated[c.Stat] && w[c.Stat] > c.SoftWeight && s.gemsCarry(c.Stat) {
			cands = append(cands, c)
		}
	}
	best, bestV := w, math.Inf(-1)
	for mask := 0; mask < 1<<len(cands); mask++ {
		trial := w
		for i, c := range cands {
			if mask&(1<<i) != 0 {
				trial[c.Stat] = c.SoftWeight
			}
		}
		s.setGemWeights(trial)
		// scored on the assumed basis, so a version with reforging
		// judges the policy knowing the reforges can rebalance around it
		if v := s.AssumedSetScore(set); v > bestV+1e-9 {
			best, bestV = trial, v
		}
	}
	s.setGemWeights(best)
}

// calibrateProcCeiling lowers a curved, capped stat's usable ceiling
// when the SET carries a proc that grants it in a burst. Armor
// penetration is the case: a passive ArP proc (Mjolnir Runestone +751,
// Grim Toll, Needle-Encrusted Scorpion) pushes the character past the
// 1400 cap while it is up, so passive points above the ceiling the proc
// leaves only pay during its downtime — they are worth their
// (1 - uptime) share, not their full slope.
//
// Measured twice in session 36: feral and frost DK, whose sim presets
// both carry Mjolnir, rise convexly to 600 passive ArP and then drop to
// 71-80% of trend, exactly where 751 + passive crosses 1400.
//
// The knee sits at cap - peak*(1-uptime) because the engine's totals
// already carry the proc's uptime-AVERAGED contribution (peak*uptime);
// the knee is that same point expressed on the averaged scale. What the
// model still approximates is the proc's own average being priced on
// the passive curve rather than at its burst level — an error far
// smaller than pricing the ceiling as if the proc did not exist.
func (s *Scorer) calibrateProcCeiling(set []*db.Item) {
	s.curves = nil
	for st, base := range s.prof.Curves {
		cap, ok := s.capOf(st)
		if !ok {
			continue
		}
		peak, uptime := s.procBurst(set, st)
		if peak <= 0 || uptime <= 0 || uptime >= 1 {
			continue
		}
		knee := cap - peak*(1-uptime)
		if knee <= 0 || knee >= cap {
			continue
		}
		if s.curves == nil {
			s.curves = map[db.Stat]StatCurve{}
		}
		s.curves[st] = base.deflateAbove(knee, 1-uptime)
	}
}

// capOf returns a stat's hard cap from the profile.
func (s *Scorer) capOf(st db.Stat) (float64, bool) {
	for _, c := range s.prof.Caps {
		if c.Stat == st {
			return c.Cap, true
		}
	}
	return 0, false
}

// procBurst reports the largest BURST of a stat any proc in the set
// grants, and that proc's uptime — the peak is what overshoots a cap,
// the uptime is how much of the time it does. Stat procs already enter
// the totals uptime-averaged (effectStats), so the uptime is recovered
// from the ratio rather than recomputed.
func (s *Scorer) procBurst(set []*db.Item, st db.Stat) (peak, uptime float64) {
	for _, it := range set {
		for _, e := range s.d.Effects[it.ID] {
			if e.Kind != db.EffectProc && e.Kind != db.EffectOnUse {
				continue
			}
			burst := e.Bonus[st]
			if burst <= peak {
				continue
			}
			avg := s.effectStats(e)[st]
			if avg <= 0 {
				continue
			}
			peak, uptime = burst, avg/burst
		}
	}
	return peak, uptime
}

// setGemWeights swaps the gem-selection weights, re-picking gems and
// dropping the memoized effective stats when they actually change.
func (s *Scorer) setGemWeights(w db.Stats) {
	if s.gemWeights == w {
		return
	}
	s.gemWeights = w
	s.eff = map[*db.Item]db.Stats{}
	s.pickGems()
}

// gemsCarry reports whether any endgame colored gem carries the stat.
func (s *Scorer) gemsCarry(st db.Stat) bool {
	for _, g := range s.d.Gems {
		if g.Quality == s.rules().GemQuality && g.Color != db.GemMeta && g.Stats[st] > 0 {
			return true
		}
	}
	return false
}

// Profile returns the profile this scorer evaluates for.
func (s *Scorer) Profile() *Profile { return s.prof }

// BestGem returns the gem assumed for a socket of the given color.
func (s *Scorer) BestGem(c db.GemColor) *db.Gem { return s.bestGem[c] }

// pickGems selects, per socket color, the highest-valued gem of the
// endgame tier (per-version quality floors: see Rules.MinGemQuality).
// Jewelcrafter-only cuts are tracked separately (bestJC) and never
// enter the general picks — non-jewelcrafters can't use them.
func (s *Scorer) pickGems() {
	s.bestGem = map[db.GemColor]*db.Gem{}
	s.safeGem = map[db.GemColor]*db.Gem{}
	s.bestAny, s.safeAny = nil, nil
	s.bestPrism, s.safePrism = nil, nil
	s.bestJC = map[db.GemColor]*db.Gem{}
	s.bestJCAny = nil
	s.metaGems = nil
	sockets := []db.GemColor{db.GemMeta, db.GemRed, db.GemYellow, db.GemBlue,
		db.GemPrismatic, db.GemCogwheel} // cogwheels: cata engineering goggles only
	for _, g := range s.d.Gems {
		// deterministic tie-break: on equal value prefer the lower item
		// ID, which lands on the standard jewelcrafting cuts (Cardinal
		// Ruby et al.) over rare oddities with identical stats
		// (Stormjewels) — map iteration order must never decide
		better := func(cur *db.Gem) bool {
			if cur == nil {
				return true
			}
			if cv := s.gemDot(cur.Stats); cv != s.gemDot(g.Stats) {
				return s.gemDot(g.Stats) > cv
			}
			return g.ID < cur.ID
		}
		// jeweler-only cuts bypass the tier gate (their profession lock
		// is the gate) and never enter the general picks
		if isJewelerGem(g) {
			for _, sock := range sockets {
				if sock == db.GemMeta || !db.ColorMatches(sock, g.Color) {
					continue
				}
				if better(s.bestJC[sock]) {
					s.bestJC[sock] = g
				}
			}
			if g.Color != db.GemMeta && better(s.bestJCAny) {
				s.bestJCAny = g
			}
			continue
		}
		quality := s.rules().GemQuality
		if g.Color == db.GemMeta {
			quality = s.rules().MetaQuality
		}
		if g.Quality != quality {
			continue
		}
		if g.Color == db.GemMeta {
			s.metaGems = append(s.metaGems, g)
		}
		for _, sock := range sockets {
			if !db.ColorMatches(sock, g.Color) {
				continue
			}
			if better(s.bestGem[sock]) {
				s.bestGem[sock] = g
			}
			if !g.Unique && better(s.safeGem[sock]) {
				s.safeGem[sock] = g
			}
		}
		// any regular gem can sit in any colored socket (forfeiting
		// the bonus); track the best such gem for AssumedGems. Metas
		// and cogwheels stay out: they only fit their own socket kind.
		anyFit := g.Color != db.GemMeta && g.Color != db.GemCogwheel
		if anyFit && better(s.bestAny) {
			s.bestAny = g
		}
		if anyFit && !g.Unique && better(s.safeAny) {
			s.safeAny = g
		}
		if g.Color == db.GemPrismatic && better(s.bestPrism) {
			s.bestPrism = g
		}
		if g.Color == db.GemPrismatic && !g.Unique && better(s.safePrism) {
			s.safePrism = g
		}
	}
	// best first, ID tie-break — activation-aware meta picks take the
	// first entry whose rule the swap outcome can meet
	sort.Slice(s.metaGems, func(a, b int) bool {
		av, bv := s.gemDot(s.metaGems[a].Stats), s.gemDot(s.metaGems[b].Stats)
		if av != bv {
			return av > bv
		}
		return s.metaGems[a].ID < s.metaGems[b].ID
	})
}

// AssumedGems returns the gem package the scorer assumes for an item's
// sockets, and whether it keeps the socket bonus. Two strategies are
// compared: matching every socket color (bonus active) vs. putting the
// globally best gem in every colored socket (bonus forfeited) — the
// classic "ignore weak socket bonuses" play. Meta sockets always take
// a meta gem; only meta gems fit them. Meta ACTIVATION (per-meta
// color-count rules, set-wide) is modeled where the swap context
// carries color facts (see assumeMeta): the assumed meta is the best
// one the swap outcome can activate, or a zero-stat placeholder when
// none can. Without facts (plain imports, the BiS solver's per-item
// stance) activation is assumed; see InactiveMeta / UnknownMeta for
// the equipped-set checks.
//
// Unique-equipped gems (Nightmare Tear) fill at most ONE socket per
// item; further sockets fall back to the best non-unique gem. Against
// the worn set the rule is enforced where the context is known: the
// upgrade ranking and compare pane model "swap ONE item, the rest
// stays" via a SwapContext — uniques socketed on kept items are off
// the table, and prismatic meta-keepers freed from the replaced item
// are re-socketed on the candidate (see AssumedGemsSwap). The BiS
// solver keeps the optimistic per-item stance (which of its items gets
// the one Tear is a knapsack it doesn't model).
//
// On items with an assumed enhancement socket (belt buckle, blacksmith
// bracer/gloves) the returned package is one gem LONGER than the item's
// socket list: the last entry fills that extra socket (assumedSockets).
//
// Jewelcrafters assume the best Dragon's Eye wherever it beats the epic
// cut — per-item best case, the same stance as the Tear/enchant
// assumptions: the set-wide "Jeweler's Gems (3)" limit is enforced only
// where the whole set is visible (GemTips, OptimizeGems). No per-item
// cap: items never carry more than three sockets in practice.
func (s *Scorer) AssumedGems(it *db.Item) (gems []*db.Gem, matched bool) {
	return s.AssumedGemsSwap(it, SwapContext{})
}

// SwapContext scopes gem assumptions to replacing one slot in a worn
// set: Taken unique gems stay socketed on the KEPT items and cannot be
// assumed; Carry prismatics (meta-keepers, e.g. Nightmare Tear) come
// off the REPLACED item and must find a socket on the candidate again
// — the swap leaves the rest of the set the way it is. RestColors are
// the meta-activation color counts the KEPT items' actual gems provide
// (hybrids count both parents, prismatics all three); with HasColors
// set, a candidate's assumed meta must find its activation counts in
// RestColors plus its own package — the swap re-gems nothing else. See
// SwapContextFor.
type SwapContext struct {
	Taken      map[int]bool
	Carry      []*db.Gem
	RestColors [3]int // red, yellow, blue
	HasColors  bool
}

// SwapContextFor builds the SwapContext for replacing ws[i].
func (s *Scorer) SwapContextFor(ws []Worn, i int) SwapContext {
	ctx := SwapContext{Taken: s.UniquesWornOutside(ws, i)}
	if i >= 0 && i < len(ws) && ws[i].HasDetail {
		for _, id := range ws[i].Gems {
			if g := s.d.ResolveGem(id); g != nil && g.Color == db.GemPrismatic {
				ctx.Carry = append(ctx.Carry, g)
			}
		}
	}
	// activation colors count only where gems are facts: one kept item
	// without detail and nothing is provable — the optimistic stance
	// stays (a plain import can't convict a meta of being inactive)
	sawDetail, allDetail := false, true
	for j, w := range ws {
		if j == i || w.Item == nil || w.Item.ID == 0 {
			continue
		}
		if !w.HasDetail {
			allDetail = false
			continue
		}
		sawDetail = true
		for _, id := range w.Gems {
			if g := s.d.ResolveGem(id); g != nil {
				r, y, b := gemColorCounts(g)
				ctx.RestColors[0] += r
				ctx.RestColors[1] += y
				ctx.RestColors[2] += b
			}
		}
	}
	ctx.HasColors = sawDetail && allDetail
	return ctx
}

// AssumedGemsSwap is AssumedGems under a slot-swap context: taken
// uniques fall back to the best non-unique gem, and each carried
// prismatic is placed into the colored socket where the whole package
// (bonus included) keeps the most value — a candidate without a
// colored socket drops it.
func (s *Scorer) AssumedGemsSwap(it *db.Item, ctx SwapContext) (gems []*db.Gem, matched bool) {
	taken := ctx.Taken
	sockets := s.assumedSockets(it)
	n := len(sockets)
	if n == 0 {
		return nil, false
	}
	jcPick := func(base, jc *db.Gem) *db.Gem {
		if s.jc() && jc != nil && (base == nil || s.gemDot(jc.Stats) > s.gemDot(base.Stats)) {
			return jc
		}
		return base
	}
	avail := func(g, fb *db.Gem) *db.Gem {
		if g != nil && g.Unique && taken[g.ID] {
			return fb
		}
		return g
	}
	match := make([]*db.Gem, n)
	ignore := make([]*db.Gem, n)
	for i, c := range sockets {
		match[i] = jcPick(avail(s.bestGem[c], s.safeGem[c]), s.bestJC[c])
		g := jcPick(avail(s.bestAny, s.safeAny), s.bestJCAny)
		// meta and cogwheel sockets only take their own gem kind —
		// the ignore-bonus play doesn't exist for them
		if c == db.GemMeta || c == db.GemCogwheel {
			g = avail(s.bestGem[c], s.safeGem[c])
		}
		ignore[i] = g
	}
	s.dedupeUnique(sockets, match, false)
	s.dedupeUnique(sockets, ignore, true)
	// candidate-side meta activation: each strategy assumes the best
	// meta its own swap outcome can activate, BEFORE the strategies are
	// priced against each other (a matched package may light a meta the
	// ignore-bonus package can't)
	s.assumeMeta(ctx, sockets, match, ctx.Carry)
	s.assumeMeta(ctx, sockets, ignore, ctx.Carry)
	var mv, iv float64
	for i := range match {
		if match[i] != nil {
			mv += s.gemDot(match[i].Stats)
		}
		if ignore[i] != nil {
			iv += s.gemDot(ignore[i].Stats)
		}
	}
	mv += s.gemDot(it.SocketBonus)
	pick := ignore
	matched = false
	if mv >= iv {
		// matched still means "keeps the socket bonus" — a belt whose only
		// socket is the assumed buckle has no bonus to keep
		pick, matched = match, len(it.GemSockets) > 0
	}
	if s.placeCarry(it, sockets, pick, ctx.Carry) {
		matched = packageMatched(it, sockets, pick)
	}
	// exact activation recheck now that carries are placed (or dropped):
	// the pre-comparison pass counted every carry as if it would land
	s.assumeMeta(ctx, sockets, pick, nil)
	return pick, matched
}

// assumeMeta rewrites the package's meta socket to the best meta the
// swap outcome can ACTIVATE: the kept items' actual colors (ctx) plus
// the package's own gems and any carries still to be placed — the swap
// model re-gems nothing else, so no other colors can appear. Metas with
// unknown rules stay eligible (optimism where honesty has no data; the
// worn-side UnknownMeta message carries the gap). When no meta can
// activate, the socket keeps the best meta as a ZERO-STAT copy: in
// game you would still socket it for the socket-bonus match, it just
// would not light up — so the bonus survives but the stats are never
// priced. Without color facts (ctx.HasColors unset: plain imports, the
// BiS solver's per-item stance) the optimistic assumption stands.
func (s *Scorer) assumeMeta(ctx SwapContext, sockets []db.GemColor, pick []*db.Gem, carry []*db.Gem) {
	if !ctx.HasColors {
		return
	}
	mi := -1
	for i, c := range sockets {
		if c == db.GemMeta && pick[i] != nil {
			mi = i
			break
		}
	}
	if mi < 0 {
		return
	}
	r, y, b := ctx.RestColors[0], ctx.RestColors[1], ctx.RestColors[2]
	for i, g := range pick {
		if i == mi {
			continue
		}
		cr, cy, cb := gemColorCounts(g)
		r, y, b = r+cr, y+cy, b+cb
	}
	for _, cg := range carry {
		if cg != nil && !slices.ContainsFunc(pick, func(g *db.Gem) bool { return g != nil && g.ID == cg.ID }) {
			cr, cy, cb := gemColorCounts(cg)
			r, y, b = r+cr, y+cy, b+cb
		}
	}
	for _, g := range s.metaGems {
		cond, known := s.d.MetaConditionFor(g.ID)
		if !known || cond.Met(r, y, b) {
			pick[mi] = g
			return
		}
	}
	dead := *pick[mi]
	dead.Stats = db.Stats{}
	pick[mi] = &dead
}

// placeCarry forces each carried prismatic (not already in the package)
// into the colored socket where the resulting package — gems plus the
// bonus when every native socket matches — keeps the most value; ties
// prefer fixing a color mismatch (the prismatic matches anything).
// Reports whether the package changed; no colored socket free = the
// carry is dropped.
func (s *Scorer) placeCarry(it *db.Item, sockets []db.GemColor, pick []*db.Gem, carry []*db.Gem) bool {
	changed := false
	placed := map[int]bool{} // socket indexes already claimed by a carry
	for _, cg := range carry {
		if cg == nil || slices.ContainsFunc(pick, func(g *db.Gem) bool { return g != nil && g.ID == cg.ID }) {
			continue
		}
		best, bestV, bestFix := -1, math.Inf(-1), false
		for si, c := range sockets {
			if c == db.GemMeta || placed[si] {
				continue
			}
			old := pick[si]
			pick[si] = cg
			v := s.packageValue(it, sockets, pick)
			fix := old == nil || !db.ColorMatches(c, old.Color)
			pick[si] = old
			if v > bestV || (v == bestV && fix && !bestFix) {
				best, bestV, bestFix = si, v, fix
			}
		}
		if best < 0 {
			continue
		}
		pick[best] = cg
		placed[best] = true
		changed = true
	}
	return changed
}

// packageValue is a gem package's calibrated worth: every gem plus the
// socket bonus when the native sockets all match.
func (s *Scorer) packageValue(it *db.Item, sockets []db.GemColor, pick []*db.Gem) float64 {
	var v float64
	for _, g := range pick {
		if g != nil {
			v += s.gemDot(g.Stats)
		}
	}
	if packageMatched(it, sockets, pick) {
		v += s.gemDot(it.SocketBonus)
	}
	return v
}

// packageMatched reports whether every NATIVE socket holds a
// color-matching gem (the honest recheck after carry placement — the
// strategy comparison's matched flag predates it).
func packageMatched(it *db.Item, sockets []db.GemColor, pick []*db.Gem) bool {
	if len(it.GemSockets) == 0 {
		return false
	}
	for si := range it.GemSockets {
		if pick[si] == nil || !db.ColorMatches(sockets[si], pick[si].Color) {
			return false
		}
	}
	return true
}

// dedupeUnique enforces unique-equipped within one assumed package: a
// unique gem keeps only the socket where it beats the non-unique
// fallback by the most (ties to the first socket); the rest fall back.
// any selects the color-ignoring fallback (the ignore-bonus strategy).
func (s *Scorer) dedupeUnique(sockets []db.GemColor, gems []*db.Gem, any bool) {
	fallback := func(c db.GemColor) *db.Gem {
		if any && c != db.GemMeta {
			return s.safeAny
		}
		return s.safeGem[c]
	}
	fbVal := func(c db.GemColor) float64 {
		if f := fallback(c); f != nil {
			return s.gemDot(f.Stats)
		}
		return 0
	}
	byGem := map[int][]int{}
	for i, g := range gems {
		if g != nil && g.Unique {
			byGem[g.ID] = append(byGem[g.ID], i)
		}
	}
	for _, idxs := range byGem {
		if len(idxs) < 2 {
			continue
		}
		keep := idxs[0]
		bestGain := s.gemDot(gems[keep].Stats) - fbVal(sockets[keep])
		for _, i := range idxs[1:] {
			if gain := s.gemDot(gems[i].Stats) - fbVal(sockets[i]); gain > bestGain {
				keep, bestGain = i, gain
			}
		}
		for _, i := range idxs {
			if i != keep {
				gems[i] = fallback(sockets[i])
			}
		}
	}
}

// gemDot values stats for gem and enchant assumptions (calibrated
// weights — capped stats zeroed when the set says so; see Calibrate).
func (s *Scorer) gemDot(st db.Stats) float64 {
	var sum float64
	for i := range st {
		sum += st[i] * s.gemWeights[i]
	}
	return sum
}

// dot is the raw weighted sum, no caps.
// localWeights is prof.Weights with every CURVED stat replaced by the
// curve's slope at the given totals. Searches that need one number per
// stat (the gem DP's linear proxy) must use it: valuing a curved stat
// at its table weight makes the search trade away exactly the stat the
// character is closest to wanting. The winner is still re-scored
// exactly, on the integral.
func (s *Scorer) localWeights(tot db.Stats) db.Stats {
	w := s.prof.Weights
	for st := range s.prof.Curves {
		c, _ := s.curveFor(st)
		w[st] = c.weightAt(tot[st])
	}
	return w
}

// dotWith is dot against an explicit weight vector.
func dotWith(st, w db.Stats) float64 {
	var sum float64
	for i := range st {
		sum += st[i] * w[i]
	}
	return sum
}

func (s *Scorer) dot(st db.Stats) float64 {
	var sum float64
	for i := range st {
		sum += st[i] * s.prof.Weights[i]
	}
	return sum
}

// GearStats returns an item's passive stats as worn: own stats + best
// matching gems + socket bonus — without proc/on-use effect averages.
func (s *Scorer) GearStats(it *db.Item) db.Stats {
	return s.GearStatsSwap(it, SwapContext{})
}

// GearStatsSwap is GearStats under a slot-swap context (see
// AssumedGemsSwap).
func (s *Scorer) GearStatsSwap(it *db.Item, ctx SwapContext) db.Stats {
	st := add(it.Stats, s.rules().RacialWeaponStats(s.race, it))
	gems, matched := s.AssumedGemsSwap(it, ctx)
	for _, g := range gems {
		if g != nil {
			st = add(st, g.Stats)
		}
	}
	if matched {
		// (meta gem activation conditions are ignored)
		st = add(st, it.SocketBonus)
	}
	return st
}

// EffectiveStats returns the stats an item is assumed to contribute for
// scoring: GearStats + the best applicable enchant + uptime-averaged
// effects. The enchant assumption mirrors the gem assumption — enriched
// worn items count their REAL enchant instead (WornStats), so upgrade
// deltas compare like with like.
func (s *Scorer) EffectiveStats(it *db.Item) db.Stats {
	if it.ID == 0 {
		return db.Stats{} // EmptyHand: no stats, no assumed enchant
	}
	if st, ok := s.eff[it]; ok {
		return st
	}
	st := add(s.GearStats(it), s.assumedReforgeStats(it))
	if e := s.BestEnchant(it); e != nil {
		st = add(st, s.enchantStats(e))
	}
	for _, e := range s.d.Effects[it.ID] {
		st = add(st, s.effectStats(e))
	}
	s.eff[it] = st
	return st
}

// EffectiveStatsSwap is EffectiveStats under a slot-swap context.
// Items whose assumption the context doesn't touch keep the memoized
// fast path; the rest are rebuilt with fallbacks and carried gems.
func (s *Scorer) EffectiveStatsSwap(it *db.Item, ctx SwapContext) db.Stats {
	if !s.swapAffects(it, ctx) {
		return s.EffectiveStats(it)
	}
	st := add(s.GearStatsSwap(it, ctx), s.assumedReforgeStats(it))
	if e := s.BestEnchant(it); e != nil {
		st = add(st, s.enchantStats(e))
	}
	for _, e := range s.d.Effects[it.ID] {
		st = add(st, s.effectStats(e))
	}
	return st
}

// swapAffects reports whether the swap context would change the item's
// unconstrained assumed package: it socketes a taken unique, a carried
// gem isn't in it yet (and a colored socket could host it), or it has
// a meta socket whose activation the context's color facts must judge.
func (s *Scorer) swapAffects(it *db.Item, ctx SwapContext) bool {
	if it.ID == 0 {
		return false
	}
	if ctx.HasColors && slices.Contains(it.GemSockets, db.GemMeta) {
		return true
	}
	if len(ctx.Taken) == 0 && len(ctx.Carry) == 0 {
		return false
	}
	gems, _ := s.AssumedGems(it)
	for _, g := range gems {
		if g != nil && ctx.Taken[g.ID] {
			return true
		}
	}
	for _, cg := range ctx.Carry {
		if cg != nil && !slices.ContainsFunc(gems, func(g *db.Gem) bool { return g != nil && g.ID == cg.ID }) {
			return true
		}
	}
	return false
}

// effectStats is effectAverage gated by trigger availability: a proc
// the build can never fire (a melee-only proc on a hunter) contributes
// nothing — see Profile.CanTrigger.
func (s *Scorer) effectStats(e *db.Effect) db.Stats {
	if !s.prof.CanTrigger(e) {
		return db.Stats{}
	}
	return s.effectAverage(e)
}

// enchantStats is what an enchant adds for this profile: its stats, or
// nothing for a curated proc enchant the rotation never triggers.
func (s *Scorer) enchantStats(e *db.Enchant) db.Stats {
	if !s.prof.CanTriggerEnchant(e) {
		return db.Stats{}
	}
	return e.Stats
}

// procInterval is the profile's assumed time between proc opportunities.
func (s *Scorer) procInterval() float64 {
	if s.prof.ProcInterval > 0 {
		return s.prof.ProcInterval.Seconds()
	}
	return defaultProcInterval.Seconds()
}

// effectAverage converts an effect into average passive stats.
func (s *Scorer) effectAverage(e *db.Effect) db.Stats {
	switch e.Kind {
	case db.EffectCurated:
		return e.Bonus // already an uptime-weighted average
	case db.EffectOnUse:
		if e.Cooldown <= 0 {
			return db.Stats{}
		}
		bonus := e.Bonus
		if e.MaxStacks > 1 {
			// stacks build once per trigger inside the buff window:
			// linear ramp to the reachable count, plateau after
			wait := s.effectWait(e)
			dur := e.Duration.Seconds()
			reached := min(float64(e.MaxStacks), dur/wait)
			bonus = scale(bonus, reached*(1-reached*wait/(2*dur)))
		}
		return scale(bonus, float64(e.Duration)/float64(e.Cooldown))
	case db.EffectProc:
		wait := s.effectWait(e)
		if wait == 0 {
			return db.Stats{}
		}
		if interval := wait + e.Cooldown.Seconds(); e.MaxStacks > 1 &&
			interval < e.Duration.Seconds() {
			// every proc adds a stack AND refreshes the window, so a
			// stack cadence (trigger wait + ICD) faster than the
			// duration holds the buff at full stacks; only the initial
			// ramp (MaxStacks×cadence, amortized over a fight)
			// discounts it. A cadence too slow to stack falls through
			// to the single-stack uptime model.
			ramp := float64(e.MaxStacks) * interval
			fight := assumedFightLength.Seconds()
			stacks := float64(e.MaxStacks) * (1 - ramp/(2*fight))
			if ramp > fight { // never reaches full within the fight
				stacks = float64(e.MaxStacks) * fight / (2 * ramp)
			}
			return scale(e.Bonus, stacks)
		}
		uptime := e.Duration.Seconds() / (e.Cooldown.Seconds() + wait)
		return scale(e.Bonus, min(uptime, 1))
	}
	return db.Stats{}
}

// effectWait is the expected seconds between an effect's trigger
// firings (ICD excluded), from its PPM or proc chance; 0 = unknown.
// On-use stacking windows carry the same trigger fields, defaulting to
// the bare trigger cadence when no chance is stated.
func (s *Scorer) effectWait(e *db.Effect) float64 {
	switch {
	case e.PPM > 0:
		return 60 / e.PPM
	case e.ProcChance > 0:
		p := e.ProcChance
		if strings.Contains(e.Outcome, "Crit") {
			p *= assumedCritRate
		}
		return s.procInterval() / p
	case e.Kind == db.EffectOnUse:
		return s.procInterval()
	}
	return 0
}

// Worn is one equipped item instance. When HasDetail is set, Gems (item
// IDs, 0 = empty socket; extra entries are blacksmith/buckle sockets) and
// EnchantID (enchantment effect ID) describe what is actually on the
// item; otherwise the best-case assumptions apply. Reforge is the
// actually applied reforge (nil = none) — like gems and enchants,
// actuals count on detailed worn items while candidates get the
// assumed optimal reforge (AssumedReforge).
type Worn struct {
	Item      *db.Item
	Gems      []int
	EnchantID int
	HasDetail bool
	Reforge   *db.Reforge
}

// WornGearStats returns the gear-only stats an equipped instance
// provides — real gems (socket bonus only when every socket is filled
// and matched) and the real enchant, but NO averaged effects. Without
// detail it falls back to the assumptions (gems, best enchant).
func (s *Scorer) WornGearStats(w Worn) db.Stats {
	if !w.HasDetail {
		st := add(s.GearStats(w.Item), s.assumedReforgeStats(w.Item))
		if e := s.BestEnchant(w.Item); e != nil {
			st = add(st, s.enchantStats(e))
		}
		return st
	}
	st := add(w.Item.Stats, s.rules().RacialWeaponStats(s.race, w.Item))
	st = add(st, reforgeDelta(w.Reforge))
	matched := true
	n := max(len(w.Item.GemSockets), len(w.Gems))
	for i := 0; i < n; i++ {
		var g *db.Gem
		if i < len(w.Gems) && w.Gems[i] != 0 {
			g = s.d.ResolveGem(w.Gems[i])
		}
		if g != nil {
			st = add(st, g.Stats)
		}
		if i < len(w.Item.GemSockets) && (g == nil || !db.ColorMatches(w.Item.GemSockets[i], g.Color)) {
			matched = false
		}
	}
	if matched && len(w.Item.GemSockets) > 0 {
		st = add(st, w.Item.SocketBonus)
	}
	if e := s.d.EnchantByEffect(w.EnchantID, w.Item.Slot); e != nil {
		st = add(st, s.enchantStats(e))
	}
	return st
}

// WornStats returns the stats an equipped instance actually provides:
// WornGearStats plus averaged effects. Without detail it falls back to
// the assumption-based EffectiveStats.
func (s *Scorer) WornStats(w Worn) db.Stats {
	if !w.HasDetail {
		return s.EffectiveStats(w.Item)
	}
	st := s.WornGearStats(w)
	for _, e := range s.d.Effects[w.Item.ID] {
		st = add(st, s.effectStats(e))
	}
	return st
}

// BonusActive reports whether a worn item's socket bonus is active:
// every socket filled with a color-matching gem (detail-less items are
// assumed optimal and always count as active when socketed).
func (s *Scorer) BonusActive(w Worn) bool {
	if len(w.Item.GemSockets) == 0 {
		return false
	}
	if !w.HasDetail {
		_, matched := s.AssumedGems(w.Item)
		return matched
	}
	for i, c := range w.Item.GemSockets {
		var g *db.Gem
		if i < len(w.Gems) && w.Gems[i] != 0 {
			g = s.d.ResolveGem(w.Gems[i])
		}
		if g == nil || !db.ColorMatches(c, g.Color) {
			return false
		}
	}
	return true
}

// TotalsWorn sums the actual stats of an equipped set.
func (s *Scorer) TotalsWorn(ws []Worn) db.Stats {
	tot := s.baseline
	for _, w := range ws {
		tot = add(tot, s.WornStats(w))
	}
	return tot
}

// SetScoreWorn is the score of a whole equipped set (actual gems/
// enchants), including active tier-set bonuses.
func (s *Scorer) SetScoreWorn(ws []Worn) float64 {
	score := s.Value(s.TotalsWorn(ws)) + s.SetBonusScore(ws)
	for _, w := range ws {
		score += s.itemBonusValue(w.Item)
	}
	return score
}

// ContributionWorn is how much ws[i] adds to the equipped set score.
func (s *Scorer) ContributionWorn(ws []Worn, i int) float64 {
	tot := s.TotalsWorn(ws)
	return s.Value(tot) - s.Value(sub(tot, s.WornStats(ws[i]))) + s.itemBonusValue(ws[i].Item)
}

// wrapWorn lifts a plain item list into detail-less instances.
func wrapWorn(set []*db.Item) []Worn {
	ws := make([]Worn, len(set))
	for i, it := range set {
		ws[i] = Worn{Item: it}
	}
	return ws
}

// Totals sums the effective stats of a set.
func (s *Scorer) Totals(set []*db.Item) db.Stats {
	tot := s.baseline
	for _, it := range set {
		tot = add(tot, s.EffectiveStats(it))
	}
	return tot
}

// Value scores a stat total, applying the profile's caps: hard-capped
// surplus is worthless, soft-capped surplus falls to the cap's
// SoftWeight.
//
// Caps are stepped and measured on the DERIVED totals: points past each
// threshold are re-priced from the previous rate (the stat's weight,
// then each cap's SoftWeight) to the cap's own — a soft cap then a hard
// one on the same stat (a bear's expertise) stack naturally, and a cap
// fed by a conversion (spirit → hit) loses its surplus at the capped
// stat's rate, whichever stat supplied it.
func (s *Scorer) Value(tot db.Stats) float64 {
	t, eff := tot, s.Derived(tot)
	var adj float64
	for _, g := range s.capSteps {
		x, first := eff[g.stat], g.caps[0].Cap
		if x <= first {
			continue
		}
		// each tier prices the points between its cap and the next
		for i, c := range g.caps {
			if x <= c.Cap {
				break
			}
			hi := x
			if i+1 < len(g.caps) {
				hi = min(x, g.caps[i+1].Cap)
			}
			adj += c.SoftWeight * (hi - c.Cap)
		}
		if x == tot[g.stat] {
			t[g.stat] = first // plain stat: clamp (exact, no residue)
		} else {
			// fed by a conversion: the surplus may be the source stat's
			// points, so take it back at the capped stat's rate
			adj -= s.prof.Weights[g.stat] * (x - first)
		}
	}
	v := s.dot(t) + adj
	// curved stats are priced by their integral instead of the linear
	// stand-in dot() just charged for them
	for st := range s.prof.Curves {
		c, _ := s.curveFor(st)
		v += c.integral(t[st]) - s.prof.Weights[st]*t[st]
	}
	return v
}

// MarginalValue is the cap-aware score of changing one displayed stat
// line from `from` to `to` points on top of the rest-of-set totals in
// base: each stat's points only count up to its profile cap. A merged
// unified-rating row carries both twin stats in sts; each side stacks
// on its own base total.
func (s *Scorer) MarginalValue(base db.Stats, sts []db.Stat, from, to float64) float64 {
	d := s.Derived(base)
	var sum float64
	for _, st := range sts {
		sum += s.statValue(st, d[st]+to) - s.statValue(st, d[st]+from)
		// a converted stat also moves its target (spirit → hit): price
		// that share on the target's cap, the source's own weight minus
		// the converted value it already counts
		for _, c := range s.prof.Conversions {
			if c.From == st {
				sum += s.statValue(c.To, d[c.To]+c.Ratio*to) - s.statValue(c.To, d[c.To]+c.Ratio*from) -
					c.Ratio*s.prof.Weights[c.To]*(to-from)
			}
		}
	}
	return sum
}

// statValue is one stat total's cap-aware score contribution: full
// weight to the cap, the cap's SoftWeight (zero for hard caps) past it.
func (s *Scorer) statValue(st db.Stat, v float64) float64 {
	curve, curved := s.curveFor(st)
	at := func(x float64) float64 {
		if curved {
			return curve.integral(x)
		}
		return s.prof.Weights[st] * x
	}
	for _, g := range s.capSteps {
		if g.stat != st || v <= g.caps[0].Cap {
			continue
		}
		// full value to the first cap, then each tier's SoftWeight up
		// to the next threshold (zero for hard caps)
		val := at(g.caps[0].Cap)
		for i, c := range g.caps {
			if v <= c.Cap {
				break
			}
			hi := v
			if i+1 < len(g.caps) {
				hi = min(v, g.caps[i+1].Cap)
			}
			val += c.SoftWeight * (hi - c.Cap)
		}
		return val
	}
	return at(v)
}

// SetScore is the score of a whole gear set.
func (s *Scorer) SetScore(set []*db.Item) float64 {
	score := s.Value(s.Totals(set))
	for _, it := range set {
		score += s.itemBonusValue(it)
	}
	return score
}

// Contribution is how much set[i] adds to the set score (with caps, so
// e.g. hit past the cap contributes nothing).
func (s *Scorer) Contribution(set []*db.Item, i int) float64 {
	tot := s.Totals(set)
	return s.Value(tot) - s.Value(sub(tot, s.EffectiveStats(set[i]))) + s.itemBonusValue(set[i])
}

// Upgrade is one proposed replacement and its set-score improvement.
//
// Delta is the honest number: what the set is worth with this item in
// the slot once every reforge is re-picked around it, against a
// baseline that got the same treatment. Frozen is the same swap with
// the rest of the set's reforges left exactly as they are — the two
// diverge when a candidate's ratings force the set to re-balance, most
// visibly when a hit-capped character swaps out a piece carrying hit.
// Versions without reforging leave the two identical.
type Upgrade struct {
	Item   *db.Item
	Delta  float64
	Frozen float64
}

// ReforgeDependent reports whether re-planning is what decides this
// upgrade: the verdict flips, or the value moves enough to matter.
// Displays mark these rows — the value is only real after a reforge
// pass.
func (u Upgrade) ReforgeDependent() bool {
	return (u.Delta > 0.5) != (u.Frozen > 0.5) ||
		math.Abs(u.Delta-u.Frozen) > reforgeMarkEP
}

// reforgeMarkEP is how far a re-plan must move a candidate's value
// before the row is worth marking — below this the reforge assumption
// is a rounding detail, not a decision.
const reforgeMarkEP = 25

// Upgrades ranks database items that would improve the set if they
// replaced set[i], best first, at most limit entries. allow, when non-nil,
// pre-filters candidates (e.g. content phase / source filters).
func (s *Scorer) Upgrades(set []*db.Item, i int, limit int, allow func(*db.Item) bool) []Upgrade {
	return s.UpgradesWorn(wrapWorn(set), i, limit, allow)
}

// UpgradesWorn is Upgrades against an equipped set with actual gems and
// enchants: the current side is valued as worn, candidates keep the
// best-case assumptions (they arrive ungemmed, after all).
func (s *Scorer) UpgradesWorn(ws []Worn, i int, limit int, allow func(*db.Item) bool) []Upgrade {
	return s.rankWorn(ws, i, limit, allow, false)
}

// DowngradesWorn lists the nearest worse-or-equal alternatives for a
// slot (delta ≤ 0), best first — context for what sits just below the
// equipped item.
func (s *Scorer) DowngradesWorn(ws []Worn, i int, limit int, allow func(*db.Item) bool) []Upgrade {
	return s.rankWorn(ws, i, limit, allow, true)
}

func (s *Scorer) rankWorn(ws []Worn, i int, limit int, allow func(*db.Item) bool, worse bool) []Upgrade {
	cur := ws[i].Item
	tot := s.TotalsWorn(ws)
	base := s.Value(tot)
	woCur := sub(tot, s.WornStats(ws[i]))
	counts := setCounts(ws)
	// swap one item, keep the rest: uniques on kept items are not
	// assumable, the replaced item's prismatics carry over
	ctx := s.SwapContextFor(ws, i)

	set := make([]*db.Item, len(ws))
	for j, w := range ws {
		set[j] = w.Item
	}
	roles := s.prof.wornWeaponRoles(set)

	// the reforge-aware basis: every slot's unreforged stats, plus the
	// worn set re-planned as the baseline both sides are measured from.
	// Without reforging replan is a no-op, so this collapses to the
	// frozen number and nothing below changes.
	unreforged := make([]db.Stats, len(ws))
	for j, w := range ws {
		unreforged[j] = s.unreforgedWorn(w)
	}
	baseTot, _ := s.replan(unreforged, set)
	baseReplan := s.Value(baseTot)
	trialStats := make([]db.Stats, len(ws))
	trialSet := make([]*db.Item, len(set))

	var ups []Upgrade
	for _, cand := range s.d.Pool() {
		if cand.ID == cur.ID || cand.Slot != cur.Slot || !cand.UsableBy(s.prof.Class) {
			continue
		}
		if !roleAllows(roles[i], cand) {
			// hand-illegal for this row: a two-hander next to a kept
			// off-hand, a main-hand-only blade in the off hand, a
			// weapon in a shield row
			continue
		}
		if cand.Unique && wornElsewhere(set, i, cand.ID) {
			// unique-equipped and already worn in the paired slot
			// (other ring/trinket, or the other hand when dual-wielding)
			continue
		}
		if g := s.d.UniqueGroup(cand.ID); g != "" && s.groupWornElsewhere(set, i, g) {
			// category exclusivity: e.g. only one Ashen Verdict band may
			// be worn, regardless of which one
			continue
		}
		if !s.typeAllowed(cand) {
			continue
		}
		if allow != nil && !allow(cand) {
			continue
		}
		// the candidate's reforge is picked cap-aware against this very
		// set (reforging into hit pays under the cap — see swapEffective)
		swapDelta := s.itemBonusValue(cand) - s.itemBonusValue(cur) +
			s.setBonusSwapDelta(counts, cur.SetName, cand.SetName)
		eff, _ := s.swapEffective(cand, ctx, woCur)
		frozen := s.Value(add(woCur, eff)) - base + swapDelta

		// the same swap with the whole set free to re-reforge around it
		copy(trialStats, unreforged)
		copy(trialSet, set)
		trialStats[i] = sub(s.EffectiveStatsSwap(cand, ctx), s.assumedReforgeStats(cand))
		trialSet[i] = cand
		tot, _ := s.replan(trialStats, trialSet)
		delta := s.Value(tot) - baseReplan + swapDelta

		if worse == (delta <= 0.5) {
			ups = append(ups, Upgrade{Item: cand, Delta: delta, Frozen: frozen})
		}
	}
	sort.Slice(ups, func(a, b int) bool { return ups[a].Delta > ups[b].Delta })
	// one row per item: a random-suffix item competes as its best roll
	seen := map[int]bool{}
	ups = slices.DeleteFunc(ups, func(u Upgrade) bool {
		dup := seen[u.Item.ID]
		seen[u.Item.ID] = true
		return dup
	})
	if len(ups) > limit {
		ups = ups[:limit]
	}
	return ups
}

// wornElsewhere reports whether the item is equipped in a slot other
// than i (the slot being evaluated).
func wornElsewhere(set []*db.Item, i int, itemID int) bool {
	for j, worn := range set {
		if j != i && worn.ID == itemID {
			return true
		}
	}
	return false
}

// groupWornElsewhere reports whether any slot other than i wears an item
// of the given category-exclusivity group.
func (s *Scorer) groupWornElsewhere(set []*db.Item, i int, group string) bool {
	for j, worn := range set {
		if j != i && s.d.UniqueGroup(worn.ID) == group {
			return true
		}
	}
	return false
}

// typeAllowed applies the profile's armor/weapon type proposal filters.
func (s *Scorer) typeAllowed(it *db.Item) bool {
	if s.prof.ArmorType != db.ArmorAny && it.ArmorType != db.ArmorAny && it.ArmorType != s.prof.ArmorType {
		return false
	}
	// shields and held frills bypass the weapon-type list: they are
	// gated by the hand role of the row they'd occupy instead
	if it.Slot == db.SlotWeapon && len(s.prof.WeaponTypes) > 0 &&
		!heldOrShield(it) && !contains(s.prof.WeaponTypes, it.WeaponType) {
		return false
	}
	if it.Slot == db.SlotRanged && len(s.prof.RangedTypes) > 0 &&
		!contains(s.prof.RangedTypes, it.RangedType) {
		return false
	}
	return true
}

func contains[T comparable](xs []T, x T) bool {
	for _, v := range xs {
		if v == x {
			return true
		}
	}
	return false
}

func add(a, b db.Stats) db.Stats {
	for i := range a {
		a[i] += b[i]
	}
	return a
}

func sub(a, b db.Stats) db.Stats {
	for i := range a {
		a[i] -= b[i]
	}
	return a
}

func scale(a db.Stats, f float64) db.Stats {
	for i := range a {
		a[i] *= f
	}
	return a
}
