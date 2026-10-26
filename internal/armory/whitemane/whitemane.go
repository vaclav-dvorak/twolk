// Package whitemane fetches characters from the Whitemane armory API
// (api.whitemane.gg) — the import wiring for the future Cataclysm game
// module (game.ImportFetch; not wired into the UI until that module
// exists).
//
// The API is undocumented and was reverse-engineered from network
// traffic (see whitemane.md at the repo root): JSON over
// unauthenticated GETs on a host that is NOT behind the main site's
// bot challenge. No contract, no versioning — parse defensively and
// treat everything as read-only. Numeric enums (race, class, slot,
// specializationId) are Cataclysm-era client IDs; mapping them onto
// planner types is the cata game module's job, so this package returns
// the wire shapes nearly verbatim.
package whitemane

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// DefaultBaseURL is the JSON API host (distinct from the website host).
const DefaultBaseURL = "https://api.whitemane.gg"

// userAgent identifies twolk politely on someone else's undocumented API.
const userAgent = "twolk gear planner (+https://github.com/vaclav-dvorak/twolk)"

// Client calls the Whitemane armory API.
type Client struct {
	BaseURL string
	HTTP    *http.Client
}

// New returns a client for the live API.
func New() *Client {
	return &Client{
		BaseURL: DefaultBaseURL,
		HTTP:    &http.Client{Timeout: 15 * time.Second},
	}
}

// Profile is the base character document; the entry point that
// resolves a name to the guid every other endpoint wants.
type Profile struct {
	GUID             int    `json:"guid"`
	Name             string `json:"name"`
	Race             int    `json:"race"`   // WoW client race ID
	Class            int    `json:"class"`  // WoW client class ID
	Gender           int    `json:"gender"` // 0 male, 1 female
	Level            int    `json:"level"`
	Faction          string `json:"faction"` // "Horde" / "Alliance"
	AverageItemLevel int    `json:"averageItemLevel"`
}

// Reforge moves Value rating points from the Source stat to the Target
// stat. Source/Target are Cataclysm ItemModType enums (31 hit, 32 crit,
// 36 haste, 49 mastery, ...); mapping them is the cata module's job.
type Reforge struct {
	Source int `json:"source"`
	Target int `json:"target"`
	Value  int `json:"value"`
}

// EquippedItem is one worn item. Reforge is nil when the item is not
// reforged (null on the wire).
type EquippedItem struct {
	Slot      int      `json:"slot"` // WoW inventory slot enum
	ItemID    int      `json:"itemId"`
	EnchantID int      `json:"enchantId"`
	Sockets   []int    `json:"sockets"` // gem ids, 0 = empty
	Reforge   *Reforge `json:"reforge"`
}

// Specialization is one of the character's (dual-spec) talent specs;
// SpecializationID 0 marks an unused slot.
type Specialization struct {
	Index            int `json:"index"`
	SpecializationID int `json:"specializationId"`
}

// TalentEntry is one talent of a spec, rank 0 = not taken.
type TalentEntry struct {
	TalentID int `json:"talentId"`
	SpellID  int `json:"spellId"`
	Rank     int `json:"rank"`
}

// Glyph is one socketed glyph of a spec.
type Glyph struct {
	GlyphID int `json:"glyphId"`
	SpellID int `json:"spellId"`
}

// Profession is one learned skill line (SkillLine IDs: 171 alchemy,
// 202 engineering, ...) — secondary skills included.
type Profession struct {
	ID    int `json:"id"`
	Skill int `json:"skill"`
}

// Character is everything the planner needs from one armory lookup.
type Character struct {
	Profile
	Realm           string
	Items           []EquippedItem
	Specializations []Specialization
	// Talents and Glyphs are keyed by spec index ("0", "1"), as on
	// the wire.
	Talents     map[string][]TalentEntry
	Glyphs      map[string][]Glyph
	Professions []Profession
}

// Character fetches a character: profile by name (resolving the guid),
// then equipment and talents by guid.
func (c *Client) Character(ctx context.Context, realm, name string) (*Character, error) {
	// realm appears in paths as a lowercase slug
	slug := url.PathEscape(strings.ToLower(realm))

	var prof Profile
	if err := c.get(ctx, fmt.Sprintf("/armory/%s/character/%s", slug, url.PathEscape(name)), &prof); err != nil {
		return nil, fmt.Errorf("character %s@%s: %w", name, realm, err)
	}
	if prof.GUID == 0 {
		return nil, fmt.Errorf("character %s@%s: profile carries no guid", name, realm)
	}

	var equip struct {
		Items []EquippedItem `json:"items"`
	}
	if err := c.get(ctx, fmt.Sprintf("/armory/%s/character/%d/equipment", slug, prof.GUID), &equip); err != nil {
		return nil, fmt.Errorf("equipment of %s@%s: %w", name, realm, err)
	}

	var talents struct {
		Specializations []Specialization         `json:"specializations"`
		Talents         map[string][]TalentEntry `json:"talents"`
		Glyphs          map[string][]Glyph       `json:"glyphs"`
	}
	if err := c.get(ctx, fmt.Sprintf("/armory/%s/character/%d/talents", slug, prof.GUID), &talents); err != nil {
		return nil, fmt.Errorf("talents of %s@%s: %w", name, realm, err)
	}

	// bare array, no envelope (unlike the paginated list endpoints)
	var profs []Profession
	if err := c.get(ctx, fmt.Sprintf("/armory/%s/character/%d/professions", slug, prof.GUID), &profs); err != nil {
		return nil, fmt.Errorf("professions of %s@%s: %w", name, realm, err)
	}

	return &Character{
		Profile:         prof,
		Realm:           realm,
		Items:           equip.Items,
		Specializations: talents.Specializations,
		Talents:         talents.Talents,
		Glyphs:          talents.Glyphs,
		Professions:     profs,
	}, nil
}

// get performs one JSON GET against the API.
func (c *Client) get(ctx context.Context, path string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/json")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GET %s: %s", path, resp.Status)
	}
	if len(body) == 0 {
		return fmt.Errorf("GET %s: empty response", path)
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("GET %s: %w", path, err)
	}
	return nil
}
