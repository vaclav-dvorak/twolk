/* twolk armory export
 *
 * The armory's JSON API omits gems and enchants, but the HTML summary
 * page carries them in each item link's rel attribute
 * ("item=X&ench=Y&gems=A:B:C"). This snippet merges both.
 *
 * Usage: open your character's armory SUMMARY page in the browser,
 * open the devtools console (F12), paste this whole file, press enter.
 * The enriched JSON is printed and copied to the clipboard — paste it
 * into twolk's import modal (i).
 */
(async () => {
  const m = location.pathname.match(/\/character\/([^/]+)\/([^/]+)/);
  if (!m) {
    console.error('twolk: open your character summary page first');
    return;
  }
  const resp = await fetch(
    `/api/character/${encodeURIComponent(m[1])}/${encodeURIComponent(m[2])}/summary`);
  const api = await resp.json();

  // collect per-item detail from the equipped-item links; duplicates
  // (two rings of the same id) queue up in DOM order
  const queues = {};
  for (const a of document.querySelectorAll('a[rel*="item="]')) {
    const p = new URLSearchParams(a.getAttribute('rel'));
    const id = p.get('item');
    if (!id) continue;
    const graw = p.get('gems');
    (queues[id] ||= []).push({
      ench: Number(p.get('ench') || 0),
      gems: graw ? graw.split(':').map(Number) : [],
    });
  }
  for (const it of api.equipment || []) {
    const q = queues[it.item];
    const d = q && q.length ? q.shift() : { ench: 0, gems: [] };
    it.ench = d.ench;
    it.gems = d.gems;
  }

  api.origin = location.origin; // lets twolk offer a ready-made API URL
  const out = JSON.stringify(api);
  console.log(out);
  try {
    await navigator.clipboard.writeText(out);
    console.log('twolk: enriched export copied to clipboard ✓');
  } catch {
    console.log('twolk: clipboard blocked — select and copy the JSON above');
  }
})();
