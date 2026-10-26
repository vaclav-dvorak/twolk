package db

import (
	"compress/gzip"
	"embed"
	"fmt"
	"io"
)

// The item database ships inside the binary: data/ holds the upstream
// WoWSims files vendored at the pinned commit (db.json gzipped, plus
// the two sim source files the trinket-effect extractor reads). twolk
// therefore never touches the network at runtime — the game is frozen
// at 3.3.5 and so is the data. To bump the pin, run
// tools/fetch_upstream.sh and update the commit here.
const pinnedCommit = "02e0d9fba5acb5c0a3d9248cb82285641540a1a9"

//go:embed data
var upstream embed.FS

// Load builds the item database from the embedded upstream data: parse,
// effect extraction, curated overlays. Takes ~100ms; callers run it off
// the UI thread.
func Load() (*Database, error) {
	raw, err := readGzip("data/db.json.gz", "embedded db")
	if err != nil {
		return nil, err
	}
	d, err := Parse(raw)
	if err != nil {
		return nil, err
	}
	procs, err := upstream.ReadFile("data/stat_bonus_procs.go.txt")
	if err != nil {
		return nil, fmt.Errorf("embedded effects: %w", err)
	}
	cds, err := upstream.ReadFile("data/stat_bonus_cds.go.txt")
	if err != nil {
		return nil, fmt.Errorf("embedded effects: %w", err)
	}
	stacking, err := upstream.ReadFile("data/stat_bonus_stacking.go.txt")
	if err != nil {
		return nil, fmt.Errorf("embedded effects: %w", err)
	}
	d.Effects = map[int][]*Effect{}
	for _, e := range append(append(ParseProcEffects(procs),
		ParseOnUseEffects(cds)...), ParseStackingEffects(stacking)...) {
		d.Effects[e.ItemID] = append(d.Effects[e.ItemID], e)
	}
	d.applyCurated()
	d.Rules = WotlkRules()
	d.pool = d.buildPool()
	return d, nil
}

// readGzip reads one gzipped file from the embedded upstream data.
func readGzip(name, label string) ([]byte, error) {
	gz, err := upstream.Open(name)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", label, err)
	}
	defer gz.Close()
	zr, err := gzip.NewReader(gz)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", label, err)
	}
	raw, err := io.ReadAll(zr)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", label, err)
	}
	return raw, nil
}
