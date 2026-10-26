# Whitemane.gg Armory API — Reverse-Engineered Reference

> **Purpose:** Findings for building an integration/client against the Whitemane private-server armory.
> **Status:** Reverse-engineered from observed network traffic (HAR capture) on 2026-07-24. This is an **undocumented, unofficial** API. Endpoints marked *Confirmed* returned live JSON during testing; those marked *Inferred* are deductions and must be verified before relying on them.
> **Reference character used for discovery:** `Examplechar` on realm `gilneas` (guid `100001`).
> **2026-07-24 update:** re-verified against a **level-85** character on `gilneas` — reforge shape, populated glyphs, and the professions endpoint are now Confirmed (§3.2, §3.3, §3.8); tab-slug probes recorded in §5.

---

## 1. Overview

The public site (`https://whitemane.gg`) is a **Next.js (App Router)** application. The visible page at
`https://whitemane.gg/en/armory/gilneas/character/examplechar` is server-rendered; the static parts (profile, equipment, stats, talents) come through Next.js RSC, while dynamic tables (achievements, boss kills, PvE logs, arena history) are loaded **client-side via XHR** from a separate JSON API host.

- **Website host (HTML/RSC, behind Cloudflare bot challenge):** `https://whitemane.gg`
- **JSON API host (the useful one):** `https://api.whitemane.gg`

**Important:** The `api.whitemane.gg` host returned clean JSON when called directly from outside the browser — it was **not** blocked by the Cloudflare "are you human" challenge that guards the main site. This makes it suitable for a server-side integration. (Do not rely on this; Cloudflare posture can change. See §7.)

There is **no** usable REST API under `https://whitemane.gg/api/...` — that path returns the full HTML SPA, not JSON. All data endpoints live on the `api.whitemane.gg` subdomain.

---

## 2. Base URL & path structure

```
https://api.whitemane.gg/armory/{realm}/character/{characterRef}[/{subresource}]
https://api.whitemane.gg/armory/{realm}/pvp/{guid}/{subresource}
```

- `{realm}` — lowercase realm slug. Known realms: `gilneas`, `maelstrom`, `frostmourne` (also referenced: `stormstout`, `sunwell`). Verify per realm.
- `{characterRef}` — either the character **name** (case-insensitive, e.g. `examplechar`) **or** the numeric **guid** (e.g. `100001`).
  - The **name** works for the base profile lookup and is the entry point: it returns the `guid`.
  - Sub-resources were observed being called with the **guid**. (Name may also work for sub-resources — *inferred*, verify.)

**Recommended flow for an integration:**
1. Resolve name → profile via the base endpoint, read `guid`.
2. Use `guid` for all sub-resource calls.

---

## 3. Confirmed endpoints

All responses are `Content-Type: application/json; charset=utf-8`.

### 3.1 Character profile (name or guid) — *Confirmed*

```
GET /armory/{realm}/character/{nameOrGuid}
```

Example: `GET https://api.whitemane.gg/armory/gilneas/character/examplechar`

```json
{
  "guid": 100001,
  "name": "Examplechar",
  "race": 8,
  "class": 3,
  "gender": 0,
  "level": 20,
  "playedTime": 27618,
  "playedTimeLevel": 1676,
  "faction": "Horde",
  "chosenTitle": 0,
  "achievementPoints": 40,
  "averageItemLevel": 9
}
```

Field notes:
- `guid` — stable numeric character id; the primary key for sub-resources.
- `race`, `class`, `gender` — numeric enums. These match **standard WoW client IDs** (*inferred*). For the reference char: `race:8` = Troll, `class:3` = Hunter, `gender:0` = male. Build a lookup table and verify against the game's Cataclysm-era (`cata`) data. See §6.
- `faction` — string (`"Horde"` / `"Alliance"`).
- `playedTime`, `playedTimeLevel` — seconds (*inferred*).
- `chosenTitle` — title id, `0` = none.
- `averageItemLevel` — integer.

### 3.2 Equipment — *Confirmed*

```
GET /armory/{realm}/character/{guid}/equipment
```

```json
{
  "items": [
    {"slot": 4, "itemId": 2300, "transmogId": 0, "enchantId": 0, "sockets": [0,0,0], "reforge": null},
    {"slot": 5, "itemId": 6558, "transmogId": 0, "enchantId": 0, "sockets": [0,0,0], "reforge": null}
  ]
}
```

Field notes:
- `slot` — equipment slot index (WoW inventory slot enum, *inferred*). Observed slots for the ref char: 4,5,6,7,8,9,14,15,16,17.
- `itemId` — resolvable on Wowhead's Cataclysm dataset: `https://www.wowhead.com/cata/item={itemId}`.
- `transmogId`, `enchantId` — `0` when none.
- `sockets` — array of 3 gem ids (`0` = empty). Gem ids are **item** ids (e.g. `52212`), resolvable on Wowhead like any item.
- `reforge` — `null` when none, otherwise (*Confirmed* on a level-85 character):

  ```json
  {"source": 49, "target": 31, "value": 67}
  ```

  `value` rating points are moved from the `source` stat to the `target` stat. `source`/`target` are **ItemModType** enums (Cataclysm client): observed `31` = hit, `32` = crit, `36` = haste, `49` = mastery — consistent with reforging's rating-only rule (40% of the source stat, hence the per-item `value`).

### 3.3 Talents & glyphs — *Confirmed*

```
GET /armory/{realm}/character/{guid}/talents
```

```json
{
  "specializations": [
    {"index": 0, "specializationId": 809},
    {"index": 1, "specializationId": 0}
  ],
  "talents": {
    "0": [
      {"talentId": 9432, "spellId": 52786, "rank": 2},
      {"talentId": 9450, "spellId": 82834, "rank": 1},
      {"talentId": 9472, "spellId": 53298, "rank": 0}
    ]
  },
  "glyphs": {}
}
```

Field notes:
- `specializations` — dual-spec; `specializationId: 0` = unused spec slot.
- `talents` — object keyed by spec index (string keys `"0"`, `"1"`). Each entry has `talentId`, `spellId` (Wowhead: `https://www.wowhead.com/cata/spell={spellId}`), and `rank`. Entries with `rank: 0` (not taken) ARE included.
- `glyphs` — object keyed by spec index like `talents` (*Confirmed* on a level-85 character):

  ```json
  {"0": [{"glyphId": 353, "spellId": 56857}, {"glyphId": 357, "spellId": 56850}]}
  ```

  All glyph tiers (prime/major/minor) appear in one flat list; the tier is a property of the glyph id, not the payload.

### 3.4 Achievements (paginated) — *Confirmed endpoint, shape inferred*

```
GET /armory/{realm}/character/{guid}/achievements?limit=18&offset=0
```

Returns the standard paginated envelope (see §4). Item shape not captured (ref char had data but item fields not inspected). Achievement ids resolve on Wowhead: `https://www.wowhead.com/cata/achievement={id}`.

### 3.5 Boss kills (paginated) — *Confirmed*

```
GET /armory/{realm}/character/{guid}/boss-kills?limit=100&offset=0
```

```json
{"items": [], "total": 0, "limit": 100, "offset": 0}
```

(Ref char had none. Item shape unverified — expect boss/encounter id, difficulty, raid, timestamp.)

### 3.6 PvE logs (paginated) — *Confirmed*

```
GET /armory/{realm}/character/{guid}/pve-logs?limit=15&offset=0
```

```json
{"items": [], "total": 0, "limit": 15, "offset": 0}
```

Powers the "Latest Encounters" table (Boss / Difficulty / Raid / Duration / Date).

### 3.7 Professions — *Confirmed — bare array, NO pagination envelope*

```
GET /armory/{realm}/character/{guid}/professions
```

```json
[
  {"id": 129, "skill": 525},
  {"id": 171, "skill": 525},
  {"id": 202, "skill": 525},
  {"id": 356, "skill": 570},
  {"id": 794, "skill": 1}
]
```

Field notes:
- Top-level is a **plain array** — the only confirmed endpoint without an object wrapper.
- `id` — SkillLine ID (standard WoW enum): `129` First Aid, `171` Alchemy, `185` Cooking, `202` Engineering, `356` Fishing, `794` Archaeology. Primary AND secondary skills are mixed together; filter by id to get the two primaries.
- `skill` — current skill points (Cataclysm cap 525; Fishing can exceed via gear/buff bonuses — observed `570`).

### 3.8 Arena history (paginated) — *Confirmed — note the different path segment*

```
GET /armory/{realm}/pvp/{guid}/arena-history?limit=15&offset=0
```

⚠️ This one uses `/pvp/{guid}/` **not** `/character/{guid}/`. Powers the "Arena History" table (Result / Bracket / Map / Damage / Healing / KB / Rating / Duration / Date).

### 3.9 Character stats — *Confirmed 2026-09-26 (session 39)*

```
GET /armory/{realm}/character/{guid}/stats
```

The server's OWN computed stat block — `/statistics` 404s (§9), `/stats` does not.

```json
{
  "attributes": {"strength":101,"agility":4827,"stamina":5163,"intellect":113,"spirit":126},
  "defenses":   {"armor":14465,"dodgePct":5.01364,"parryPct":5,"blockPct":0,"resilience":0,"resiliencePct":0},
  "offense":    {"attackPower":5078,"rangedAttackPower":9804,"spellPower":0,"mastery":798,"masteryPct":4.45114},
  "ratings":    {"critPct":22.8973,"rangedCritPct":23.8973,"spellCritPct":9.46564,
                 "haste":7.89491,"rangedHaste":8.5821,
                 "hitRating":0,"spellHitRating":0,"expertise":0,"offhandExpertise":0},
  "resistances": {"holy":0,"fire":0,"nature":0,"frost":0,"shadow":0,"arcane":0},
  "power":      {"maxHealth":111059,"maxPower":[0,0,100,0,0,0,0,0,0,0,0]},
  "summary":    {"averageItemLevel":315,"achievementPoints":1120}
}
```

Field notes:
- Ratings arrive as **percentages**, not rating points — multiply by the level-85 conversions to compare with a planner (`rangedHaste 8.5821 × 128.05745 = 1099`). `mastery` is the exception: rating, with `masteryPct` beside it.
- `hitRating`, `spellHitRating`, `expertise`, `offhandExpertise` read **0 on a character that has plenty of all of them** — not populated, do not consume.
- **Melee vs ranged split is real and useful**: the reference char reads `haste 7.89491` against `rangedHaste 8.5821`, exactly the +88 of a ranged-only weapon scope. Same for `critPct` vs `rangedCritPct` (+1%, the troll bow racial).
- The block reflects the same **logout snapshot** as `/equipment` — it agreed with twolk to the point (mastery 798, armor 14465) while the player, logged in with a different main hand, saw other numbers.
- `summary.averageItemLevel` disagrees with the equipped set's mean (315 vs 356 here); treat it as unexplained, not as gear data.

---

## 4. Pagination envelope

All list endpoints (`achievements`, `boss-kills`, `pve-logs`, `arena-history`) share:

```json
{ "items": [ /* ... */ ], "total": 0, "limit": 15, "offset": 0 }
```

- `limit` / `offset` query params control the window.
- `total` gives the full count for building pagination.
- Observed default limits: achievements `18`, boss-kills `100`, pve-logs `15`, arena-history `15`.

---

## 5. Endpoints seen on the page but NOT yet confirmed via API

The rendered profile also shows: **Statistics** (attributes/offense/defense), **PvP summary** (best rating, solo/2v2/3v3), **Raid Progress**, and tabs for **Mounts, Pets, Transmog, Titles, Reputations, Professions, Quests, Achievements**.

Slug probes against a level-85 character (2026-07-24):

| Slug | Result |
|---|---|
| `professions` | **200** — promoted to Confirmed, see §3.7 |
| `reputations` | **200** — bare array `[{"faction": 21, "standing": 750}, ...]` (faction id, raw standing points; negatives possible) |
| `titles` | **200** — `[]` on the probe char (shape of entries unknown) |
| `mounts` | **200** — envelope `{"items": [{"spellId": 10796, "date": "2026-06-22T19:20:18.000Z"}, ...]}` |
| `pets` | **200** — envelope `{"items": [{"entry": 26056, "summonSpell": 46426, "date": "..."}]}` (`entry` = creature id) |
| `quests` | **200** — envelope `{"items": [{"quest": 29477, "date": "..."}]}` (recently completed quests) |
| `statistics` | **404** — no such endpoint even at max level; the page's attribute block is client-derived from equipment + talents |
| `transmog` | **404** — slug unknown |

- A PvP rating summary slug remains undiscovered (likely under `/pvp/{guid}/`).

---

## 6. Enum resolution

Numeric ids in responses correspond to WoW's **Cataclysm (4.x)** client data (the site links items/spells to `wowhead.com/cata/...`, and the wowhead tooltip calls used `dataEnv=11`).

- Items: `https://www.wowhead.com/cata/item={itemId}`
- Spells: `https://www.wowhead.com/cata/spell={spellId}`
- Achievements: `https://www.wowhead.com/cata/achievement={id}`
- `race`, `class`, `gender`, `slot`, `specializationId` — map via standard WoW enums; **build and verify** these tables rather than trusting the single reference sample.

---

## 7. Integration guidance & caveats

- **Unofficial/undocumented.** No published contract, no versioning guarantees. Field names and shapes can change without notice. Code defensively (tolerate missing/extra fields).
- **Not a documented public API** — check the site's Terms (`https://whitemane.gg/en/terms`) before building/distributing anything. Respect rate limits; add backoff and caching. Set a descriptive `User-Agent`.
- **Cloudflare:** the `api.whitemane.gg` host answered direct (non-browser) requests during testing, but the main `whitemane.gg` site is behind a Cloudflare bot challenge. If the API later starts challenging too, a headless browser that solves the challenge (or an official arrangement) would be required. Do **not** attempt to bypass bot protection.
- **Auth:** none observed — all discovery calls were unauthenticated GETs. Some endpoints returned `304 Not Modified` in the browser (ETag/conditional caching); support conditional requests / caching in the client.
- **Name vs guid:** resolve name→guid once via the base profile endpoint, then cache the guid.
- **Only GET observed.** Treat the API as read-only.

---

## 8. Client

The real client lives in `internal/armory/whitemane` (profile →
guid → equipment/talents/professions, live-verified @85) — it
supersedes the pseudocode sketch that used to sit here.

---

## 9. Endpoint cheat-sheet

| Purpose | Method & path | Status |
|---|---|---|
| Profile (name→guid) | `GET /armory/{realm}/character/{name\|guid}` | Confirmed |
| Equipment (incl. reforge) | `GET /armory/{realm}/character/{guid}/equipment` | Confirmed (reforge shape verified @85) |
| Talents & glyphs | `GET /armory/{realm}/character/{guid}/talents` | Confirmed (glyph shape verified @85) |
| Professions | `GET /armory/{realm}/character/{guid}/professions` | Confirmed (bare array, no envelope) |
| Reputations | `GET /armory/{realm}/character/{guid}/reputations` | Confirmed (bare array) |
| Mounts / Pets / Quests | `GET /armory/{realm}/character/{guid}/{mounts\|pets\|quests}` | Confirmed (items envelope) |
| Titles | `GET /armory/{realm}/character/{guid}/titles` | Confirmed endpoint, entry shape TBD |
| Achievements | `GET /armory/{realm}/character/{guid}/achievements?limit&offset` | Confirmed (item shape TBD) |
| Boss kills | `GET /armory/{realm}/character/{guid}/boss-kills?limit&offset` | Confirmed (item shape TBD) |
| PvE logs | `GET /armory/{realm}/character/{guid}/pve-logs?limit&offset` | Confirmed (item shape TBD) |
| Arena history | `GET /armory/{realm}/pvp/{guid}/arena-history?limit&offset` | Confirmed (item shape TBD) |
| Character stats | `GET /armory/{realm}/character/{guid}/stats` | Confirmed (server-computed; ratings as PERCENTAGES) |
| Statistics | `GET /armory/{realm}/character/{guid}/statistics` | **Does not exist** (404 even @85 — the block lives at `/stats`) |
| Transmog / PvP summary | *slugs unknown* | To discover |
