// Package config persists twolk state in a per-user config file.
package config

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// UpgradeFilter is the persisted upgrade-proposal filter, modeling a
// progressively-unlocked server: ServerPhase is a content CEILING (1..5,
// T7..Ruby Sanctum) applied to every item regardless of source — later-
// phase raids, dungeons, emblem tiers and craft recipes simply don't
// exist yet. The flags gate acquisition routes the player can access
// within the open content.
type UpgradeFilter struct {
	ServerPhase int  `json:"server_phase"` // 1..5 content ceiling
	Raids       bool `json:"raids"`        // BoP raid drops
	RaidBoE     bool `json:"raid_boe"`     // BoE raid drops (AH-buyable)
	Dungeon     bool `json:"dungeon"`      // 5-man dungeon drops
	Vendor      bool `json:"vendor"`       // emblem + other vendors
	Crafted     bool `json:"crafted"`
	Other       bool `json:"other"` // quests and unknown sources
	// PvP includes resilience gear in proposals. Unlike the flags above
	// it defaults to false: PvP items are noise for a PvE planner.
	PvP bool `json:"pvp"`
	// Locked lists the game's in-phase unlocks (game.Unlock slugs) the
	// realm hasn't reached yet; absent = open.
	Locked []string `json:"locked,omitempty"`
}

// StoredCharacter is one imported character: where it plays (all three
// identifiers are stable slugs/names from the game-module server
// registry) plus the raw armory export, base64-encoded so the config
// file stays a small, flat JSON document regardless of what the export
// contains.
type StoredCharacter struct {
	Game   string `json:"game"`   // game-version slug ("wotlk")
	Server string `json:"server"` // server slug ("warmane")
	Realm  string `json:"realm"`  // realm name ("Onyxia")
	RawB64 string `json:"raw_b64"`
	// ImportedAt is when the export was pasted — armory data only ages
	// from there. Zero for entries predating the field.
	ImportedAt time.Time `json:"imported_at,omitempty"`
}

// Raw returns the decoded armory export, nil when it fails to decode.
func (s StoredCharacter) Raw() []byte {
	raw, err := base64.StdEncoding.DecodeString(s.RawB64)
	if err != nil {
		return nil
	}
	return raw
}

// Config is the on-disk shape of the twolk config file.
type Config struct {
	// Chars holds the imported characters; ActiveChar indexes the
	// current one.
	Chars      []StoredCharacter `json:"characters,omitempty"`
	ActiveChar int               `json:"active_char,omitempty"`
	// Filters restores the upgrade filter across runs, keyed by
	// "game/server/realm" — free-server realms progress INDEPENDENTLY
	// (one realm can sit at P2 while its sibling is at endgame), so the
	// phase ceiling and route choices are realm-local state.
	Filters map[string]UpgradeFilter `json:"filters,omitempty"`
	// TalentPoints holds assumed points in hit-affecting talents, keyed
	// by class/talent slugs (e.g. "hunter/focused_aim"). The armory
	// export doesn't reveal individual talents, so these are user-set.
	TalentPoints map[string]int `json:"talent_points,omitempty"`
	// RaidDebuff records, per game slug, that the raid provides the
	// spell-hit debuff (Misery / Improved Faerie Fire): every
	// spell-hit-capped profile's effective cap drops by the version's
	// debuff percentage. Raid composition, so per game, not per
	// character.
	RaidDebuff map[string]bool `json:"raid_debuff,omitempty"`
	// Weights holds user stat-weight overrides per profile key (e.g.
	// "hunter/survival"), keyed by stat slugs ("hit", "agility",
	// "melee_dps"). Only values differing from the defaults are stored.
	Weights map[string]map[string]float64 `json:"weights,omitempty"`
}

// Path returns the config file location — ~/.twolk/config.json, the
// classic cross-platform dotdir convention — creating the directory if
// needed.
func Path() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolving home dir: %w", err)
	}
	dir := filepath.Join(home, ".twolk")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("creating config dir: %w", err)
	}
	return filepath.Join(dir, "config.json"), nil
}

// Load reads the config file, returning an empty Config if none exists yet.
func Load() (*Config, error) {
	path, err := Path()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return &Config{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading config: %w", err)
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}
	if cfg.ActiveChar < 0 || cfg.ActiveChar >= len(cfg.Chars) {
		cfg.ActiveChar = 0
	}
	return &cfg, nil
}

// Save writes the config file atomically (write temp file, then rename).
func (c *Config) Save() error {
	path, err := Path()
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding config: %w", err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("writing config: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("replacing config: %w", err)
	}
	return nil
}

// UpsertCharacter validates raw, wraps it with its origin metadata and
// stores it at index i (append when i is out of range), making it the
// active character.
func (c *Config) UpsertCharacter(raw []byte, meta StoredCharacter, i int) error {
	if !json.Valid(raw) {
		return fmt.Errorf("not valid JSON")
	}
	meta.RawB64 = base64.StdEncoding.EncodeToString(raw)
	meta.ImportedAt = time.Now()
	if i >= 0 && i < len(c.Chars) {
		c.Chars[i] = meta
		c.ActiveChar = i
	} else {
		c.Chars = append(c.Chars, meta)
		c.ActiveChar = len(c.Chars) - 1
	}
	return nil
}

func (c *Config) activeIndex() int {
	if c.ActiveChar >= 0 && c.ActiveChar < len(c.Chars) {
		return c.ActiveChar
	}
	return -1
}

// Character returns the decoded active armory JSON, or nil if none.
func (c *Config) Character() ([]byte, error) {
	i := c.activeIndex()
	if i < 0 {
		return nil, nil
	}
	raw := c.Chars[i].Raw()
	if raw == nil {
		return nil, fmt.Errorf("decoding stored character")
	}
	return raw, nil
}

// Characters returns all stored exports, decoded. Entries that fail to
// decode come back nil.
func (c *Config) Characters() [][]byte {
	out := make([][]byte, len(c.Chars))
	for i, s := range c.Chars {
		out[i] = s.Raw()
	}
	return out
}

// SetActive switches the active character.
func (c *Config) SetActive(i int) {
	if i >= 0 && i < len(c.Chars) {
		c.ActiveChar = i
	}
}

// RemoveCharacter deletes a stored character, adjusting the active index.
func (c *Config) RemoveCharacter(i int) {
	if i < 0 || i >= len(c.Chars) {
		return
	}
	c.Chars = append(c.Chars[:i], c.Chars[i+1:]...)
	if c.ActiveChar >= len(c.Chars) {
		c.ActiveChar = max(len(c.Chars)-1, 0)
	} else if c.ActiveChar > i {
		c.ActiveChar--
	}
}
