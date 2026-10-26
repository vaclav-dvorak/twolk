package db

// IsBoE reports whether an item binds on equip — the set where
// binding changes obtainability (BoE raid drops and world drops are
// AH-buyable). Two generated tables back it: boeItems (wotlkdb.com
// 3.3.5 tooltips, raid-sourced items only) and cataBoEItems (wowhead
// cata tooltips, the whole useful-ilvl pool — the UI flags only
// drop-sourced items, so crafted BoEs stay unlabeled). Item ids are
// version-unique, so one predicate serves both games.
func IsBoE(itemID int) bool {
	return boeItems[itemID] || cataBoEItems[itemID]
}
