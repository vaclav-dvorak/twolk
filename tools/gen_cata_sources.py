#!/usr/bin/env python3
"""Generate internal/db/catasources_gen.go — sources, phase overlay and
PvP classification for the Cataclysm database.

Everything comes from BULK dumps the WoWSims cata repo vendors at our
pinned commit (no scraping, matching the project's data etiquette):
  - assets/database/db.json                     the item pool
  - assets/db_inputs/wowhead_gearplannerdb.txt  per-item wowhead source
    metadata (source codes + sourcemore npc/zone/profession/season)
  - assets/db_inputs/wowhead_item_tooltips.csv  rendered tooltips —
    the only dump carrying the Unique-Equipped lines

Emitted tables (applied in ParseCata):
  - cataSupplementalSources: one true source per item the upstream data
    cannot source (~3.8k of 4.9k orphans) — vendor/drop/quest/crafted.
  - cataPvPItems: PvP-currency gear (wowhead source code 3, plus the
    elite Gladiator variants wowhead leaves sourceless).
  - cataUnobtainable: items neither the upstream nor wowhead can source
    and that are not PvP sets — sim test items ("Bland Blade"), data
    leftovers; dropped from the pool at load.
  - cataPhaseOverride: content phase per item. The upstream tags nearly
    everything phase 1, so this is derived: raid/dungeon ZONE of any
    source (ZG/ZA -> 2, Firelands -> 3, Dragon Soul + 4.3 heroics -> 4,
    Baradin Hold by ilvl), PvP SEASON name (Vicious -> 1, Ruthless -> 3,
    Cataclysmic -> 4), then an ilvl fallback for vendor/crafted gear.
  - cataUniqueItems: per-item Unique-Equipped flags the upstream data
    is missing (its own unique field covers most but not all).
  - cataUniqueGroups: "Unique-Equipped: <family> (1)" tooltip lines —
    at most one worn across the family's LFR/normal/heroic variants
    (the BiS solver would otherwise wear two Starcatcher Compasses).

Usage:
    python3 tools/gen_cata_sources.py > internal/db/catasources_gen.go

Env:
    WOWSIMS_CATA_DB   path to a local db.json (default: fetch pinned URL)
    WOWHEAD_GPDB      path to a local wowhead_gearplannerdb.txt (default: fetch)
    WOWHEAD_TOOLTIPS  path to a local wowhead_item_tooltips.csv (default: fetch)
"""

import json
import os
import re
import sys
import urllib.request

PINNED = "70d87383a9b92f30fb9e370c4676d3ce33b6e6b6"
BASE = f"https://raw.githubusercontent.com/wowsims/cata/{PINNED}"
DB_URL = f"{BASE}/assets/database/db.json"
GPDB_URL = f"{BASE}/assets/db_inputs/wowhead_gearplannerdb.txt"
TOOLTIPS_URL = f"{BASE}/assets/db_inputs/wowhead_item_tooltips.csv"

MIN_ILVL = 272  # keep in sync with the cata module's MinUsefulILvl

# zone id -> content phase (Whitemane Gilneas progression:
# P1 T11 raids, P2 Rise of the Zandalari, P3 Firelands, P4 Dragon Soul).
# The launch raids are listed EXPLICITLY as phase 1: their heroic loot
# (ilvl 372) would otherwise fall through to the ilvl fallback and be
# mis-phased into P3 — the zone rule must decide first.
ZONE_PHASE = {
    5094: 1,  # Blackwing Descent (normal 359, heroic 372)
    5334: 1,  # The Bastion of Twilight
    5638: 1,  # Throne of the Four Winds
    1977: 2,  # Zul'Gurub
    3805: 2,  # Zul'Aman
    5723: 3,  # Firelands
    5788: 4,  # Well of Eternity
    5789: 4,  # End Time
    5844: 4,  # Hour of Twilight
    5892: 4,  # Dragon Soul
}
BARADIN_HOLD = 5600  # per-boss phases; classified by ilvl instead

# vendors whose whole stock belongs to a phase their zone tag can't
# show (wowhead gives them no zone): Lurah Wrathvine trades normal
# Firelands items + a Crystallized Firestone for the heroic version at
# the raid's entrance — 4.2 content, whose 391 ilvl the fallback below
# would otherwise read as Dragon Soul
VENDOR_PHASE = {54402: 3}

# PvP season name markers -> phase the season opened in
SEASON_PHASE = [
    ("Cataclysmic Gladiator", 4),  # S11 (4.3)
    ("Ruthless Gladiator", 3),     # S10 (4.2)
    ("Vicious Gladiator", 1),      # S9 (4.0)
    ("Bloodthirsty", 1),           # crafted honor filler
]

# wowhead crafted-source SkillLine id -> CraftedSource profession enum
# (keep in sync with db.CraftedSource.ProfessionName)
SKILL_PROFESSION = {
    171: 1, 164: 2, 333: 3, 202: 4, 182: 5, 773: 6,
    755: 7, 165: 8, 186: 9, 393: 10, 197: 11,
}


def fetch(url, env, binary=False):
    path = os.environ.get(env)
    if path:
        with open(path, "rb" if binary else "r") as f:
            return f.read()
    print(f"fetching {url}", file=sys.stderr)
    with urllib.request.urlopen(url) as r:
        data = r.read()
        return data if binary else data.decode()


def load_gearplanner(raw):
    """The dump is a series of WH.setPageData("wow.gearPlanner.cata.item",
    {...}) statements; each payload is JSON with trailing commas."""
    out = {}
    for m in re.finditer(r'WH\.setPageData\("wow\.gearPlanner\.cata\.item", ', raw):
        s = m.end()
        depth = 0
        for j in range(s, len(raw)):
            if raw[j] == "{":
                depth += 1
            elif raw[j] == "}":
                depth -= 1
                if depth == 0:
                    out.update(json.loads(re.sub(r",\s*}", "}", raw[s : j + 1])))
                    break
    return {int(k): v for k, v in out.items()}


def load_tooltips(raw):
    """The dump is CSV-shaped but the tooltip payloads embed unescaped
    commas and newlines; records reliably start at line-begin 'id,{'."""
    parts = re.split(r"(?m)^(\d+),(?=\{)", raw)
    return {int(parts[i]): parts[i + 1] for i in range(1, len(parts) - 1, 2)}


def uniqueness(tooltip):
    """Parse the Unique-Equipped line: ('group', family) for the
    name-grouped form ("Unique-Equipped: Starcatcher Compass (1)"),
    ('item', None) for the plain forms, (None, None) when absent."""
    if m := re.search(r"Unique-Equipped: ([^<(]+?) \((\d+)\)", tooltip):
        if m.group(2) != "1":  # no (N>1) item family exists today
            print(f"skipping {m.group(1)}: allows {m.group(2)}", file=sys.stderr)
            return (None, None)
        return ("group", m.group(1))
    if "Unique-Equipped" in tooltip or re.search(r"(?:ue-->|<br>)Unique<", tooltip):
        return ("item", None)
    return (None, None)


def ilvl(it):
    return it.get("scalingOptions", {}).get("0", {}).get("ilvl", 0)


def season_phase(name):
    for marker, phase in SEASON_PHASE:
        if marker in name:
            return phase
    return None


def pick_source(gpe):
    """Choose the single best supplemental source from a gearplanner
    entry: drop > vendor > quest > crafted (mirrors the wotlk table's
    one-source-per-item shape).

    sourcemore entries align positionally with the source-code array
    when the lengths match; when they differ the mapping is lost and a
    t=1 (NPC) entry on a drop+vendor item is ambiguous between the
    dropper and the seller — claiming it for the drop mislabeled 632
    vendors as droppers ("Drops: Weapons Vendor"), claiming it for the
    vendor would mislabel bosses as sellers ("Sold by: Argaloth"). The
    tie-breaker is the NPC id: Cataclysm-Classic-added ids (>= 200000,
    the generic 4.4 catch-up "Weapons/Accessories Vendor" entities)
    are never droppers, classic-era ids drop.

    Those catch-up vendors are PHANTOMS: wowhead's gearplanner data
    lists them (and their npc pages exist, "found in Stormwind City"),
    but an in-game sweep of the Whitemane capitals found no such NPCs
    and no gearing guide has ever mentioned them — dead client data.
    Any other route (drop, quest, crafted) outranks them and they are
    never NAMED as the seller; items with no other claim keep a bare
    unnamed vendor source."""
    codes = gpe.get("source") or []
    more = gpe.get("sourcemore") or []
    by_code = dict(zip(codes, more)) if codes and len(codes) == len(more) else {}

    def first(t_wanted=None):
        for sm in more:
            if t_wanted is None or sm.get("t") == t_wanted:
                return sm
        return None

    def classic_added(sm):
        return sm and sm.get("t") == 1 and sm.get("ti", 0) >= 200000

    if 2 in codes:  # drop
        sm = by_code.get(2)
        if sm is None and not by_code:
            cand = first(1) or first()
            if not (5 in codes and classic_added(cand)):
                sm = cand
        if sm and sm.get("t") == 1:
            return ("drop", sm.get("ti", 0), sm.get("n", ""), sm.get("z", 0))
        if sm and sm.get("n") and sm.get("t") in (None, 2, 3):  # object/container
            return ("object", 0, sm.get("n", ""), sm.get("z", 0))
        if 5 not in codes and 4 not in codes and 1 not in codes:
            return ("world", 0, "", 0)
        # drop details unknowable, but another route can be named:
        # fall through to the vendor/quest/crafted branches
    if 5 in codes:  # vendor
        sm = by_code.get(5)
        if sm is None and not by_code:
            cand = first(1)
            if cand and (2 not in codes or classic_added(cand)):
                sm = cand
        if classic_added(sm) or sm is None:
            # phantom (or unknown) seller: prefer quest/crafted routes
            if 4 in codes or 1 in codes:
                sm = None
            elif 2 in codes:
                # drop+vendor with only a phantom seller claim: the 4.4
                # "drops from any boss" BoE gear — a world drop
                return ("world", 0, "", 0)
            else:
                return ("vendor", 0, "", 0)
        if sm:
            return ("vendor", sm.get("ti", 0), sm.get("n", ""), sm.get("z", 0))
    if 4 in codes:  # quest reward: sourcemore t=5 names the quest
        sm = first(5) or first()
        if sm and sm.get("t") == 5:
            return ("quest", sm.get("ti", 0), sm.get("n", ""), sm.get("z", 0))
        return ("quest", 0, "", (sm or {}).get("z", 0))
    if 1 in codes:  # crafted
        sm = first()
        prof = SKILL_PROFESSION.get((sm or {}).get("s", 0), 0)
        # the zone slot carries the craft spell id (t=6 spell sourcemore)
        spell = (sm or {}).get("ti", 0) if (sm or {}).get("t") == 6 else 0
        return ("crafted", prof, "", spell)
    return None


def item_phase(it, gpe, name):
    """Phase for one item, or None to leave the upstream tag alone."""
    if it.get("phase", 1) > 1:
        return None  # the upstream actually knows better here
    if (p := season_phase(name)) is not None:
        return p if p > 1 else None
    lvl = ilvl(it)
    zones = set()
    for src in it.get("sources") or []:
        d = src.get("drop") or {}
        if d.get("zoneId"):
            zones.add(d["zoneId"])
    for sm in (gpe or {}).get("sourcemore") or []:
        if sm.get("z"):
            zones.add(sm["z"])
        if sm.get("t") == 1 and sm.get("ti") in VENDOR_PHASE:
            return VENDOR_PHASE[sm["ti"]]
    for z in zones:
        if z in ZONE_PHASE:
            return ZONE_PHASE[z]
    if BARADIN_HOLD in zones:
        return 4 if lvl >= 384 else 3 if lvl >= 371 else None
    # vendor/crafted/valor fallback by ilvl tier: 353 = ZA/ZG loot,
    # 372 = EXCLUSIVELY heroic T11 gear (launch content — the token
    # vendor pieces have no zone for the zone rule to catch, so the
    # ilvl must except them), 365..383 = Molten Front / 4.2 valor /
    # Firelands normal, 384+ = 4.3 content (DS LFR 384/390, Darkmoon
    # Faire trinkets, valor 397, DS normal/heroic) — FL heroic (391)
    # is caught by the Firelands ZONE rule before this fallback runs
    if lvl == 353:
        return 2
    if lvl == 372:
        return None
    if 365 <= lvl <= 383:
        return 3
    if lvl >= 384:
        return 4
    return None


def main():
    db = json.loads(fetch(DB_URL, "WOWSIMS_CATA_DB"))
    gp = load_gearplanner(fetch(GPDB_URL, "WOWHEAD_GPDB"))
    tooltips = load_tooltips(fetch(TOOLTIPS_URL, "WOWHEAD_TOOLTIPS"))

    items = {it["id"]: it for it in db["items"]}
    sources, pvp, unobtainable, phases = {}, [], [], {}
    unique_items, unique_groups, boe = [], {}, []
    stats = {"sourced": 0, "pvp": 0, "dropped": 0, "phased": 0}

    for iid in sorted(items):
        it = items[iid]
        name = it.get("name", "")
        gpe = gp.get(iid)

        if (p := item_phase(it, gpe, name)) is not None:
            phases[iid] = p
            stats["phased"] += 1

        # uniqueness matters only for candidates the solver may double up
        if ilvl(it) >= MIN_ILVL:
            if "Binds when equipped" in tooltips.get(iid, ""):
                boe.append(iid)
            kind, family = uniqueness(tooltips.get(iid, ""))
            if kind == "group":
                unique_groups[iid] = family
            elif kind == "item" and not it.get("unique"):
                unique_items.append(iid)

        is_pvp = bool(gpe and 3 in (gpe.get("source") or []))
        if not it.get("sources"):
            picked = pick_source(gpe) if gpe else None
            if picked:
                sources[iid] = picked
                stats["sourced"] += 1
            elif is_pvp or "Gladiator's" in name:
                is_pvp = True  # elite PvP variants: sourceless on wowhead
            elif ilvl(it) >= MIN_ILVL:
                unobtainable.append(iid)
                stats["dropped"] += 1
        if is_pvp:
            pvp.append(iid)
            stats["pvp"] += 1

    print(f"sourced {stats['sourced']}, pvp {stats['pvp']}, "
          f"dropped {stats['dropped']}, phase overrides {stats['phased']}, "
          f"unique {len(unique_items)}, grouped {len(unique_groups)}, boe {len(boe)}",
          file=sys.stderr)

    w = sys.stdout.write
    w("// Code generated by tools/gen_cata_sources.py from the wowhead\n")
    w("// dumps vendored in wowsims/cata @ " + PINNED[:8] + "; DO NOT EDIT.\n\n")
    w("package db\n\n")

    w("var cataSupplementalSources = map[int]genSource{\n")
    for iid in sorted(sources):
        kind, npc, nm, zone = sources[iid]
        w(f"\t{iid}: {{{json.dumps(kind)}, {npc}, {json.dumps(nm)}, {zone}}},\n")
    w("}\n\n")

    w("// cataPvPItems is gear obtainable only through PvP currencies\n")
    w("// (honor/conquest), including the elite Gladiator variants.\n")
    w("var cataPvPItems = map[int]bool{\n")
    for iid in sorted(set(pvp)):
        w(f"\t{iid}: true,\n")
    w("}\n\n")

    w("var cataUnobtainable = []int{\n")
    for iid in sorted(unobtainable):
        w(f"\t{iid}, // {items[iid]['name']}\n")
    w("}\n\n")

    w("// cataPhaseOverride corrects the upstream phase tags (nearly\n")
    w("// everything is tagged phase 1 upstream).\n")
    w("var cataPhaseOverride = map[int]int{\n")
    for iid in sorted(phases):
        w(f"\t{iid}: {phases[iid]},\n")
    w("}\n\n")

    w("// cataUniqueItems marks Unique-Equipped items the upstream data\n")
    w("// leaves unflagged (tooltip-derived; pool items only).\n")
    w("var cataUniqueItems = []int{\n")
    for iid in sorted(unique_items):
        w(f"\t{iid}, // {items[iid]['name']}\n")
    w("}\n\n")

    w('// cataUniqueGroups maps items to their "Unique-Equipped: <family>\n')
    w("// (1)\" tooltip family: at most one worn across the family's LFR/\n")
    w("// normal/heroic variants (RuleSet.UniqueGroups).\n")
    w("var cataUniqueGroups = map[int]string{\n")
    for iid in sorted(unique_groups):
        w(f"\t{iid}: {json.dumps(unique_groups[iid])},\n")
    w("}\n\n")

    w("// cataBoEItems marks pool items that bind on equip (tooltip-\n")
    w("// derived): AH-buyable, so binding changes the acquisition route\n")
    w("// (BoE raid drops leave the raid gate, world drops are gold gear).\n")
    w("var cataBoEItems = map[int]bool{\n")
    for iid in sorted(boe):
        w(f"\t{iid}: true, // {items[iid]['name']}\n")
    w("}\n")


if __name__ == "__main__":
    main()
