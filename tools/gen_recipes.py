#!/usr/bin/env python3
"""Generate the recipe-source tables — how a crafter learns the recipe
for every pool-relevant crafted item, keyed by the craft spell id the
item's crafted source carries:
  wotlk -> internal/db/recipes_gen.go       (recipeSources)
  cata  -> internal/db/catarecipes_gen.go   (cataRecipeSources)

Bulk upstreams only (no scraping):
  - the server's world SQL — wotlk: AzerothCore base tables (one file
    per table, fetched); cata: the TCPP TDB 4.3.4 full world dump (one
    file, see gen_cata_vendorcosts.py for the release):
      trainer_spell            trainer-taught craft spells
      npc_vendor               recipe vendors
      *_loot_template          recipe drops; Reference rows are walked
                               up to the creature / chest / container
      creature_template        names, loot ids, heroic difficulty entries
      creature, gameobject     spawn maps (instance name for drops)
      gameobject_template      chest loot ids
      instance_encounters      boss kill credit -> DungeonEncounter map,
                               for script-summoned bosses (no spawn row)
      quest_template           recipe quest rewards
      item_template (wotlk)    recipe items -> taught spell, reputation
  - wago.tools client tables at the version's Classic build: Map,
    Faction, DungeonEncounter, SpellEffect (which craft creates an
    enchant's carrier item — scopes, armor kits); for
    cata also ItemSparse + ItemEffect (the TDB has no item_template).

A craft the trainer teaches is "trainer" even when a recipe item also
exists. Drop routes are summarized: a few named sources ("Kel'Thuzad ·
Naxxramas"), one or two instances ("Ulduar") when many sources share
them, "world drop" when they span the open world.

Usage:
    python3 tools/gen_recipes.py wotlk > internal/db/recipes_gen.go
    python3 tools/gen_recipes.py cata  > internal/db/catarecipes_gen.go

Env:
    WOWSIMS_DB        wotlk: path to a local db.json (default: fetch)
    WOWSIMS_CATA_DB   cata: path to a local db.json (default: fetch)
    WOWHEAD_GPDB      cata: local wowhead_gearplannerdb.txt (default: fetch)
    ACORE_SQL         wotlk: directory caching AzerothCore *.sql tables
    TDB_WORLD_SQL     cata: path to TDB_full_world_434.*.sql (required)
    WAGO_CSV          directory caching wago.tools CSVs (per-build
                      subdirectories are created inside it)
"""

import collections
import csv
import io
import json
import os
import re
import sys
import urllib.request

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import gen_cata_sources  # noqa: E402  (cata crafted-source picking)

GAMES = {
    "wotlk": {
        "db_env": "WOWSIMS_DB",
        "db_url": "https://raw.githubusercontent.com/wowsims/wotlk/"
                  "02e0d9fba5acb5c0a3d9248cb82285641540a1a9/assets/database/db.json",
        "build": "3.4.3.54261",
        "min_ilvl": 130,  # keep in sync with the TUI's minUsefulILvl
        "var": "recipeSources",
        "source": "AzerothCore world SQL (3.3.5a)",
    },
    "cata": {
        "db_env": "WOWSIMS_CATA_DB",
        "db_url": gen_cata_sources.DB_URL,
        "build": "4.4.2.60895",
        "min_ilvl": gen_cata_sources.MIN_ILVL,
        "var": "cataRecipeSources",
        "source": "TCPP TDB 434.22011 world SQL",
    },
}
ACORE_BASE = ("https://raw.githubusercontent.com/azerothcore/azerothcore-wotlk/"
              "master/data/sql/base/db_world/")
WAGO = "https://wago.tools/db2/{}/csv?build={}"

# a drop with at most this many distinct named sources lists them; the
# same bound caps listed instances and vendors
MAX_NAMED = 2

STANDINGS = ["Hated", "Hostile", "Unfriendly", "Neutral", "Friendly",
             "Honored", "Revered", "Exalted"]

ITEM_SPELLTRIGGER_LEARN = 6  # item effect trigger: teaches its spell
SPELL_EFFECT_CREATE_ITEM = 24

# client map names that differ from the zone names the sim data (and so
# the rest of the UI) uses
MAP_RENAMES = {"The Battle for Mount Hyjal": "Hyjal Summit", "The Sunwell": "Sunwell Plateau"}

# Map.InstanceType: 0 = open world
OPEN_WORLD = "0"


def fetch_text(url):
    req = urllib.request.Request(url, headers={"User-Agent": "twolk-gen-recipes/1.0"})
    with urllib.request.urlopen(req, timeout=300) as resp:
        return resp.read().decode("utf-8", "replace")


class SQL:
    """Rows of world-database tables, from per-table files or one dump."""

    def __init__(self, game):
        self.game = game
        self.dump = None
        if game == "cata":
            with open(os.environ["TDB_WORLD_SQL"], encoding="utf-8", errors="replace") as f:
                self.dump = f.read()

    def _text(self, name):
        if self.dump is not None:
            i = self.dump.find(f"CREATE TABLE `{name}`")
            if i < 0:
                sys.exit(f"table {name} missing from the TDB dump")
            end = self.dump.find("CREATE TABLE", i + 1)
            return self.dump[i:end if end > 0 else len(self.dump)]
        cache = os.environ.get("ACORE_SQL")
        fp = cache and os.path.join(cache, name + ".sql")
        if fp and os.path.exists(fp):
            with open(fp, encoding="utf-8", errors="replace") as f:
                return f.read()
        txt = fetch_text(ACORE_BASE + name + ".sql")
        if fp:
            with open(fp, "w") as f:
                f.write(txt)
        return txt

    def rows(self, name):
        """Yield every row of a table as {column: str}."""
        txt = self._text(name)
        i = txt.find(f"CREATE TABLE `{name}`")
        body = txt[i:txt.find(") ENGINE", i)]
        cols = [c for c in re.findall(r"^\s*`(\w+)`\s", body, re.M) if c != name]
        n = len(txt)
        i = txt.find("INSERT INTO", i)
        while i != -1:
            j = txt.find("VALUES", i) + 6
            while j < n:
                c = txt[j]
                if c == "(":
                    vals, cur, instr = [], [], False
                    j += 1
                    while True:
                        c = txt[j]
                        if instr:
                            if c == "\\":
                                cur.append(txt[j + 1])
                                j += 2
                                continue
                            if c == "'":
                                if txt[j + 1] == "'":
                                    cur.append("'")
                                    j += 2
                                    continue
                                instr = False
                            else:
                                cur.append(c)
                            j += 1
                            continue
                        if c == "'":
                            instr = True
                        elif c == ",":
                            vals.append("".join(cur))
                            cur = []
                        elif c == ")":
                            vals.append("".join(cur))
                            j += 1
                            break
                        else:
                            cur.append(c)
                        j += 1
                    yield dict(zip(cols, vals))
                elif c == ";":
                    break
                else:
                    j += 1
            i = txt.find("INSERT INTO", j)


def load_wago(table, build):
    cache = os.environ.get("WAGO_CSV")
    fp = cache and os.path.join(cache, build, table + ".csv")
    if fp and os.path.exists(fp):
        with open(fp) as f:
            txt = f.read()
    else:
        txt = fetch_text(WAGO.format(table, build))
        if fp:
            os.makedirs(os.path.dirname(fp), exist_ok=True)
            with open(fp, "w") as f:
                f.write(txt)
    return list(csv.DictReader(io.StringIO(txt)))


def by_id(rows):
    return {int(r["ID"]): r for r in rows}


def load_db(cfg):
    path = os.environ.get(cfg["db_env"])
    if path:
        with open(path) as f:
            return json.load(f)
    return json.loads(fetch_text(cfg["db_url"]))


def pool_crafts(game, cfg, wdb):
    """craft spell -> product item id, for the pool's crafted items."""
    crafts = {}
    if game == "wotlk":
        for it in wdb["items"]:
            if it.get("ilvl", 0) < cfg["min_ilvl"]:
                continue
            for s in it.get("sources", []):
                c = s.get("crafted")
                if c and c.get("spellId"):
                    crafts[c["spellId"]] = it["id"]
        return crafts
    # cata: the upstream sources almost no crafted gear; the craft spell
    # comes from the same wowhead pick gen_cata_sources makes
    path = os.environ.get("WOWHEAD_GPDB")
    raw = open(path).read() if path else fetch_text(gen_cata_sources.GPDB_URL)
    gpdb = gen_cata_sources.load_gearplanner(raw)
    for it in wdb["items"]:
        if gen_cata_sources.ilvl(it) < cfg["min_ilvl"]:
            continue
        srcs = it.get("sources") or []
        for s in srcs:
            c = s.get("crafted")
            if c and c.get("spellId"):
                crafts[c["spellId"]] = it["id"]
        if not srcs and (gpe := gpdb.get(it["id"])):
            pick = gen_cata_sources.pick_source(gpe)
            if pick and pick[0] == "crafted" and pick[3]:
                crafts[pick[3]] = it["id"]
    return crafts


def recipe_items(game, sql, build, crafts):
    """item names, and craft spell -> [(recipe item, name, faction, rank)]."""
    names = {}
    recipes = collections.defaultdict(list)
    if game == "wotlk":
        for r in sql.rows("item_template"):
            e = int(r["entry"])
            names[e] = r["name"]
            if r["class"] != "9":
                continue
            for k in ("spellid_1", "spellid_2"):
                sp = int(r[k])
                if sp in crafts:
                    recipes[sp].append((e, r["name"], int(r["RequiredReputationFaction"]),
                                        int(r["RequiredReputationRank"])))
        return names, recipes
    sparse = by_id(load_wago("ItemSparse", build))
    names = {i: r["Display_lang"] for i, r in sparse.items()}
    for r in load_wago("ItemEffect", build):
        sp, item = int(r["SpellID"]), int(r["ParentItemID"])
        if int(r["TriggerType"]) == ITEM_SPELLTRIGGER_LEARN and sp in crafts and item in sparse:
            s = sparse[item]
            recipes[sp].append((item, s["Display_lang"], int(s["MinFactionID"]),
                                int(s["MinReputation"])))
    return names, recipes


def enchant_crafts(build, wdb):
    """enchant carrier item -> [(craft spell, item)] creating it
    (SpellEffect CREATE_ITEM), for the version's item-based enchants."""
    carriers = {e["itemId"] for e in wdb["enchants"] if e.get("itemId")}
    out = collections.defaultdict(list)
    for r in load_wago("SpellEffect", build):
        item = int(r["EffectItemType"])
        if int(r["Effect"]) == SPELL_EFFECT_CREATE_ITEM and item in carriers:
            out[item].append((int(r["SpellID"]), item))
    return out


def main():
    game = sys.argv[1] if len(sys.argv) > 1 else ""
    if game not in GAMES:
        sys.exit("usage: gen_recipes.py wotlk|cata")
    cfg = GAMES[game]
    build = cfg["build"]
    sql = SQL(game)
    wdb = load_db(cfg)
    crafts = pool_crafts(game, cfg, wdb)
    # enchant carriers (scopes, armor kits, …) are crafted items too: a
    # recipe nobody can learn yet keeps the enchant off the server
    carriers = enchant_crafts(build, wdb)
    for spells in carriers.values():
        for sp, item in spells:
            crafts.setdefault(sp, item)
    items, recipes = recipe_items(game, sql, build, crafts)
    recipe_ids = {e for v in recipes.values() for e, *_ in v}
    trained = {int(r["SpellId"]) for r in sql.rows("trainer_spell")}

    factions = by_id(load_wago("Faction", build))
    encounters = by_id(load_wago("DungeonEncounter", build))

    # boss -> instance for script-summoned bosses, from the sim's data
    zone_names = {z["id"]: z["name"] for z in wdb["zones"]}
    # map display names: the sim's name for the map's area (what the
    # rest of the UI shows), else the client map name
    maps = by_id(load_wago("Map", build))
    for m in maps.values():
        n = zone_names.get(int(m["AreaTableID"])) or m["MapName_lang"]
        m["MapName_lang"] = MAP_RENAMES.get(n, n)
    continents = {maps[m]["MapName_lang"] for m in maps
                  if maps[m]["InstanceType"] == OPEN_WORLD}
    npc_zone = {n["id"]: zone_names.get(n.get("zoneId"), "") for n in wdb["npcs"]}
    for it in wdb["items"]:  # the npcs list misses many bosses
        for s in it.get("sources") or []:
            d = s.get("drop")
            if d and d.get("npcId") and zone_names.get(d.get("zoneId")):
                npc_zone.setdefault(d["npcId"], zone_names[d["zoneId"]])

    # creatures: names, loot ids, heroic entries folded onto the base
    cname, base_of, by_loot = {}, {}, collections.defaultdict(set)
    for r in sql.rows("creature_template"):
        e = int(r["entry"])
        cname[e] = r["name"]
        by_loot[int(r["lootid"])].add(e)
        for k in ("difficulty_entry_1", "difficulty_entry_2", "difficulty_entry_3"):
            if int(r[k]):
                base_of[int(r[k])] = e
    spawn_id = "id1" if game == "wotlk" else "id"
    cmap = collections.defaultdict(set)
    czone = collections.defaultdict(set)  # spawn area ids (vendor gates)
    for r in sql.rows("creature"):
        cmap[int(r[spawn_id])].add(int(r["map"]))
        if int(r["zoneId"]):
            czone[int(r[spawn_id])].add(int(r["zoneId"]))
    for r in sql.rows("instance_encounters"):
        if r["creditType"] == "0" and int(r["entry"]) in encounters:
            cmap[int(r["creditEntry"])].add(int(encounters[int(r["entry"])]["MapID"]))
    goname, go_by_loot = {}, collections.defaultdict(set)
    for r in sql.rows("gameobject_template"):
        e = int(r["entry"])
        goname[e] = r["name"]
        if r["type"] == "3":  # chest: Data1 is the loot id
            go_by_loot[int(r["Data1"])].add(e)
    gomap = collections.defaultdict(set)
    for r in sql.rows("gameobject"):
        gomap[int(r["id"])].add(int(r["map"]))

    # loot graph: (table, entry) holders of each recipe item, and the
    # parents of every reference table, walked up to real looters
    holders = collections.defaultdict(set)
    ref_parents = collections.defaultdict(set)
    for t in ("creature_loot_template", "gameobject_loot_template",
              "item_loot_template", "reference_loot_template"):
        for r in sql.rows(t):
            if r.get("IsCurrency", "0") != "0":
                continue
            ent, ref = int(r["Entry"]), int(r["Reference"])
            if ref:
                ref_parents[ref].add((t, ent))
            elif int(r["Item"]) in recipe_ids:
                holders[int(r["Item"])].add((t, ent))

    def zones_of(mapids, npc=0):
        names = {maps[m]["MapName_lang"] for m in mapids if m in maps}
        if not names and npc_zone.get(npc):
            names.add(npc_zone[npc])
        return frozenset(names)

    def looters(item):
        """(kind, name, zones) for each creature/chest/container looting it."""
        out, seen = set(), set()
        stack = list(holders[item])
        while stack:
            t, ent = stack.pop()
            if (t, ent) in seen:
                continue
            seen.add((t, ent))
            if t == "reference_loot_template":
                stack.extend(ref_parents[ent])
            elif t == "creature_loot_template":
                for c in by_loot.get(ent, ()):
                    b = base_of.get(c, c)
                    out.add(("npc", cname.get(b, ""), zones_of(cmap[b], b)))
            elif t == "gameobject_loot_template":
                for g in go_by_loot.get(ent, ()):
                    out.add(("go", goname.get(g, ""), zones_of(gomap[g])))
            elif t == "item_loot_template":
                out.add(("item", items.get(ent, ""), frozenset()))
        return out

    def drop_where(srcs):
        """Summarize looters: named sources, one or two instances, or ""
        for a world drop. Sources without a known location (summoned
        adds) only count when nothing else locates the drop."""
        containers = sorted(n for k, n, _ in srcs if k == "item")
        if containers and len(srcs) == len(containers):
            return " / ".join(containers[:MAX_NAMED])
        located = {s for s in srcs if s[2]} or srcs
        zones = set().union(*(z for _, _, z in located))
        named = sorted({n for k, n, _ in located if k != "item"})
        zone = " / ".join(sorted(zones))
        if len(named) <= MAX_NAMED:
            return f"{' / '.join(named)} · {zone}" if zone else " / ".join(named)
        if zones & continents:
            return zone if len(zones) == 1 else ""
        return zone if len(zones) <= MAX_NAMED else ""

    vendors = collections.defaultdict(list)
    for r in sql.rows("npc_vendor"):
        i = int(r["item"])
        if i in recipe_ids and r.get("type", "1") == "1":
            vendors[i].append(int(r["entry"]))

    quests = collections.defaultdict(list)
    qcols = None
    for r in sql.rows("quest_template"):
        if qcols is None:
            qcols = [k for k in r if re.fullmatch(r"RewardItem\d|RewardChoiceItemID\d", k)]
        for k in qcols:
            v = r[k]
            if v.isdigit() and int(v) in recipe_ids:
                quests[int(v)].append(r["LogTitle"])

    table = {}
    stats = collections.Counter()
    for sp in sorted(crafts):
        rs = recipes.get(sp)
        if sp in trained:  # some trained crafts also have a (dead) recipe item
            table[sp] = ("", "RecipeTrainer", "", "", 0)
            stats["trainer"] += 1
            continue
        if not rs:
            stats["unknown"] += 1
            if not any(sp in [x for x, _ in v] for v in carriers.values()):
                print(f"no route: spell {sp} (item {crafts[sp]})", file=sys.stderr)
            continue
        name = rs[0][1]
        rep = ""
        for _, _, fac, rank in rs:
            if fac:
                rep = f"{factions[fac]['Name_lang']} — {STANDINGS[rank]}"
        vend = [v for e, *_ in rs for v in vendors.get(e, [])]
        if vend:
            names = sorted({cname.get(n, "") for n in vend})
            # the vendors' one shared zone (gates like the Molten Front
            # key on it); 0 when they stand in different places
            zones = set().union(*(czone[n] for n in vend))
            zone = zones.pop() if len(zones) == 1 else 0
            table[sp] = (name, "RecipeVendor", " / ".join(names[:MAX_NAMED]), rep, zone)
            stats["vendor"] += 1
            continue
        qs = sorted({q for e, *_ in rs for q in quests.get(e, [])})
        if qs:
            table[sp] = (name, "RecipeQuest", qs[0], rep, 0)
            stats["quest"] += 1
            continue
        srcs = set().union(*(looters(e) for e, *_ in rs))
        if srcs:
            where = drop_where(srcs)
            kind = "RecipeDrop" if where else "RecipeWorldDrop"
            table[sp] = (name, kind, where, rep, 0)
            stats[kind] += 1
            continue
        # a recipe item no vendor, quest or loot table hands out
        # (removed / promo): name the recipe, claim no route
        table[sp] = (name, "RecipeUnknown", "", rep, 0)
        stats["recipe-no-route"] += 1

    def go_str(s):
        return json.dumps(s, ensure_ascii=False)

    print(f"// Code generated by tools/gen_recipes.py from the {cfg['source']}")
    print(f"// + client tables (wago.tools, build {build}); DO NOT EDIT.")
    print()
    print("package db")
    print()
    print("// craft spell id -> how the recipe is learned")
    print(f"var {cfg['var']} = map[int]RecipeSource{{")
    for sp in sorted(table):
        name, kind, where, rep, zone = table[sp]
        print(f"\t{sp}: {{{go_str(name)}, {kind}, {go_str(where)}, {go_str(rep)}, {zone}}},")
    print("}")
    print()
    # carrier item -> its craft spell: the routed spell (a test or GM
    # duplicate creating the same item never has one)
    print("// enchant carrier item id -> craft spell id")
    print(f"var {cfg['var'].replace('RecipeSources', 'EnchantCrafts').replace('recipeSources', 'enchantCrafts')} = map[int]int{{")
    for item in sorted(carriers):
        routed = [sp for sp, _ in carriers[item] if sp in table]
        if routed:
            print(f"\t{item}: {min(routed)},")
    print("}")
    print(f"recipes: {len(crafts)} craft spells, {dict(stats)}", file=sys.stderr)


if __name__ == "__main__":
    main()
