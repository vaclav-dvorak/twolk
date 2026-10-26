#!/usr/bin/env python3
"""Generate internal/db/cataprotocol_gen.go — the Elemental Rune
Dungeon ("heroic+") fragment economy of Cataclysm CLASSIC.

The protocols are Classic-only additions (4.4.1/4.4.2) and therefore
invisible to both of the other cata vendor upstreams: TCPP TDB is
original 4.3.4 (the system does not exist there) and the wowhead
gearplanner dump tags the affected items as bare source-code-5
"vendor". The authoritative sources are two VENDORED wowhead sells
snapshots (tools/data/*.json, extracted from browser-saved NPC pages
— wowhead cannot be fetched headlessly; re-save the pages to
refresh). The warcraft.wiki.gg Kyanite table was the initial source
and taught a lesson: its plain item names hid WHICH variant the
vendor stocks — the wowhead ids revealed heroic-372 armor/trinkets
where the wiki names had resolved to normal-359.

  - Protocol Inferno (patch 4.4.1 — planner phase 3): bosses in the
    nine launch heroics drop Fissure Stone Fragments; Kyanite
    Stonetender sells T11-era raid gear for them.
  - Protocol Twilight (patch 4.4.2 — planner phase 4): Obsidian
    Fragments + Dragon Soul LFR drops. Its vendor stock is derived at
    LOAD TIME from the item pool (internal/db/loadcata.go), not here —
    the wiki lists categories, not an itemized table.

Emitted tables (applied in ParseCata):
  - cataFissurePrices: item id -> Fissure Stone Fragment cost, from
    tools/data/kyanite_sells.json (npc=234135, saved 2026-07-27).
    The stock: normal-359 weapons (15-30), HEROIC-372 armor and
    trinkets (50-60), heroic-only Sinestra 379 pieces (80), normal
    T11 head/shoulder tokens (32, expanded per class color) and the
    Throne of the Four Winds satchels (12-21, resolved to the
    contained item's lowest variant — ToFW gear carries no heroic
    flag upstream, so 359-vs-372 satchel contents is an ASSUMPTION;
    settle it from a satchel tooltip if one ever surfaces).
  - cataTwilightDungeons: dedicated Protocol Twilight final-boss
    drops -> the dungeon they drop in (tools/data/twilight_drops.json,
    the wowhead Protocol Twilight guide's two loot tables; the guide
    notes they were compiled from PTR data). One LFR weapon + one
    accessory per launch heroic; names resolve to the Raid Finder
    ilvl variant. Everything else in the LFR pool (tier via tokens,
    armor) is a random/vendor drop and keeps the generic label.
  - cataObsidianPrices: item id -> Obsidian Fragment cost, from a
    VENDORED wowhead snapshot (tools/data/zarstrasza_sells.json — the
    sells listview of npc=241468 on wowhead.com/cata, extracted from
    a browser-saved page on 2026-07-27; wowhead cannot be fetched
    headlessly). The snapshot is the complete 134-row stock with
    exact item ids and costs: normal FL weapons, Ragnaros loot,
    heroic FL/Ragnaros trinkets, all Dragon Soul Raid Finder gear,
    tier tokens and the Flickering satchels. Token rows ("of the
    Fiery <color>" = normal T12 378 head/shoulder, "of the Corrupted
    <color>" = T13 Raid Finder 384, five slots) expand to the pieces
    they buy; satchel rows to the heroic 391 "Flickering" contents;
    the Commendation and Crystallized Firestone rows are skipped
    (not equippable).

Usage:
    python3 tools/gen_cata_protocol.py > internal/db/cataprotocol_gen.go

Env:
    WOWSIMS_CATA_DB   path to a local db.json (default: fetch pinned URL)
    WOWHEAD_KYA_SELLS path to the kyanite sells snapshot
                      (default: tools/data/kyanite_sells.json)
    WOWHEAD_ZAR_SELLS path to the zarstrasza sells snapshot
                      (default: tools/data/zarstrasza_sells.json)
    TWILIGHT_DROPS    path to the dedicated-drop mapping
                      (default: tools/data/twilight_drops.json)
"""

import html as htmllib
import json
import os
import re
import sys
import urllib.parse
import urllib.request

PINNED = "70d87383a9b92f30fb9e370c4676d3ce33b6e6b6"
DB_URL = f"https://raw.githubusercontent.com/wowsims/cata/{PINNED}/assets/database/db.json"
KYA_SELLS = os.path.join(os.path.dirname(__file__), "data", "kyanite_sells.json")
ZAR_SELLS = os.path.join(os.path.dirname(__file__), "data", "zarstrasza_sells.json")
TWI_DROPS = os.path.join(os.path.dirname(__file__), "data", "twilight_drops.json")

# wowsims proto class ids -> the tier-token color that buys the class's
# set pieces (Conqueror/Protector/Vanquisher; same colors T11..T13)
TOKEN_CLASSES = {
    "Conqueror": [4, 5, 8],      # paladin, priest, warlock
    "Protector": [9, 2, 7],      # warrior, hunter, shaman
    "Vanquisher": [6, 10, 3, 1], # rogue, death knight, mage, druid
}
TYPE_HEAD, TYPE_SHOULDER, TYPE_CHEST, TYPE_HANDS, TYPE_LEGS = 1, 3, 5, 7, 9
# T13 Raid Finder token slot words -> wowsims item type
T13_SLOTS = {"Crown": TYPE_HEAD, "Shoulders": TYPE_SHOULDER, "Chest": TYPE_CHEST,
             "Gauntlets": TYPE_HANDS, "Leggings": TYPE_LEGS}

def fetch(url, env):
    path = os.environ.get(env)
    if path:
        return open(path, errors="ignore").read()
    print(f"fetching {url}", file=sys.stderr)
    with urllib.request.urlopen(url) as r:
        return r.read().decode()


def ilvl(it):
    return (it.get("scalingOptions") or {}).get("0", {}).get("ilvl") or 0


def main():
    db = json.loads(fetch(DB_URL, "WOWSIMS_CATA_DB"))
    by_name = {}
    for it in db["items"]:
        by_name.setdefault(it["name"], []).append(it)

    def resolve(name, want_ilvl=None):
        """Item name -> entry; want_ilvl filters, else prefer the
        lowest non-heroic variant."""
        cands = by_name.get(name, [])
        if want_ilvl is not None:
            cands = [c for c in cands if ilvl(c) == want_ilvl]
        cands = sorted(cands, key=lambda i: (i.get("heroic", False), ilvl(i)))
        return cands[0] if cands else None

    # tier-set expansion shared by both vendors: every set piece of the
    # given ilvl/slot whose single class the token color buys
    def expand_token(color, want_type, ilvl_want, cost, note, into):
        n = 0
        for it in db["items"]:
            if (ilvl(it) == ilvl_want and it.get("setName") and
                    "Gladiator" not in it["setName"] and
                    it.get("type") == want_type and
                    len(it.get("classAllowlist", [])) == 1 and
                    it["classAllowlist"][0] in TOKEN_CLASSES[color]):
                into[it["id"]] = (cost, it["name"] + note)
                n += 1
        return n

    # ---- the Fissure (Protocol Inferno) vendor: wowhead snapshot ----
    kya = json.loads(open(os.environ.get("WOWHEAD_KYA_SELLS", KYA_SELLS)).read())
    prices = {}  # id -> (cost, comment)
    unresolved = []
    pool_ids = {it["id"] for it in db["items"]}
    for row in kya:
        name, cost = row["name"], row["cost"]
        if row["id"] in pool_ids:
            prices[row["id"]] = (cost, name)
            continue
        if m := re.match(r"(?:Helm|Mantle) of the Forlorn (\w+)", name):
            want = TYPE_HEAD if name.startswith("Helm") else TYPE_SHOULDER
            if not expand_token(m.group(1), want, 359, cost, f" — via {name}", prices):
                unresolved.append(name)
            continue
        if m := re.match(r"Satchel of (?:the )?(.+)", name):
            it = resolve(m.group(1))
            if it:
                prices[it["id"]] = (cost, m.group(1) + " — satchel, random enchant")
            else:
                unresolved.append(name)
            continue
        if name == "Commendation of Service":
            continue  # a Justice Points conversion, not gear
        unresolved.append(name)
    if len(prices) < 100:
        sys.exit(f"suspiciously few fissure prices: {len(prices)}")

    # ---- the Obsidian (Protocol Twilight) vendor: wowhead snapshot ----
    zar = json.loads(open(os.environ.get("WOWHEAD_ZAR_SELLS", ZAR_SELLS)).read())
    obsidian = {}  # id -> (cost, comment)
    wt_unresolved = []
    for row in zar:
        name, cost = row["name"], row["cost"]
        if row["id"] in pool_ids:
            obsidian[row["id"]] = (cost, name)
            continue
        if m := re.match(r"(?:Helm|Mantle) of the Fiery (\w+)", name):
            want = TYPE_HEAD if name.startswith("Helm") else TYPE_SHOULDER
            if not expand_token(m.group(1), want, 378, cost, f" — via {name}", obsidian):
                wt_unresolved.append(name)
            continue
        if m := re.match(r"(\w+) of the Corrupted (\w+)", name):
            if m.group(1) in T13_SLOTS:
                if not expand_token(m.group(2), T13_SLOTS[m.group(1)], 384,
                                    cost, f" — via {name}", obsidian):
                    wt_unresolved.append(name)
                continue
        if m := re.match(r"Satchel of (?:the )?(.+)", name):
            cands = [i2 for i2 in by_name.get(m.group(1), []) if ilvl(i2) == 391]
            if cands:
                obsidian[cands[0]["id"]] = (cost, m.group(1) + " — satchel, random enchant")
            else:
                wt_unresolved.append("Satchel: " + name)
            continue
        if name in ("Commendation of Service", "Crystallized Firestone"):
            continue  # currency conversion / cost item, not gear
        wt_unresolved.append(name)
    if len(obsidian) < 120:
        sys.exit(f"suspiciously few obsidian prices: {len(obsidian)}")

    for name in unresolved:
        print(f"UNRESOLVED (fissure): {name}", file=sys.stderr)
    for name in wt_unresolved:
        print(f"UNRESOLVED (obsidian): {name}", file=sys.stderr)

    print(f"""// Code generated by tools/gen_cata_protocol.py — DO NOT EDIT.
//
// Provenance: Cataclysm CLASSIC (4.4.x) only — the Elemental Rune
// Dungeon fragment economy does not exist in original 4.3.4 data.
// Sources: vendored wowhead sells snapshots (tools/data/
// kyanite_sells.json npc=234135, zarstrasza_sells.json npc=241468,
// both saved 2026-07-27), resolved against wowsims/cata db.json
// @ {PINNED}.

package db

// cataFissurePrices: Fissure Stone Fragment cost per item at Kyanite
// Stonetender (Protocol Inferno vendor, phase 3): normal weapons,
// heroic-372 armor/trinkets, Sinestra 379 pieces; tier-token rows are
// expanded to the pieces the token buys, satchel rows to the
// contained item.
var cataFissurePrices = map[int]int{{""")
    for id in sorted(prices):
        cost, comment = prices[id]
        print(f"\t{id}: {cost}, // {comment}")
    print("}")
    print("""
// cataObsidianPrices: Obsidian Fragment cost per item at Zarstrasza/
// Sylstrasza (Protocol Twilight vendor, phase 4). Same expansion rules
// as the fissure table; plain rows resolve to the Raid Finder ilvl
// variant. The vendor's normal-Firelands weapon stock is not itemized
// by any bulk source yet — those entries stay cost-unknown.
var cataObsidianPrices = map[int]int{""")
    for id in sorted(obsidian):
        cost, comment = obsidian[id]
        print(f"\t{id}: {cost}, // {comment}")
    print("}")

    twi = json.loads(open(os.environ.get("TWILIGHT_DROPS", TWI_DROPS)).read())
    dungeons = {}  # id -> (dungeon, name)
    for row in twi:
        cands = [c for c in by_name.get(row["name"], []) if ilvl(c) in (384, 390)]
        if cands:
            dungeons[cands[0]["id"]] = (row["dungeon"], row["name"])
        else:
            print(f"UNRESOLVED (twilight drop): {row['name']}", file=sys.stderr)
    print("""
// cataTwilightDungeons: the dedicated Protocol Twilight final-boss
// drops and the dungeon each is assigned to (one LFR weapon + one
// accessory per launch heroic; from the wowhead Protocol Twilight
// guide, PTR-compiled). The rest of the LFR pool drops randomly /
// sells for fragments and keeps the generic final-bosses label.
var cataTwilightDungeons = map[int]string{""")
    for id in sorted(dungeons):
        dungeon, name = dungeons[id]
        print(f"\t{id}: {json.dumps(dungeon)}, // {name}")
    print("}")


if __name__ == "__main__":
    main()
