// A per-browser UI choice (grid or list, a tab) that survives reloads: localStorage, like the
// mobile app's key-value store, under the same keys. Only values in `allowed` are read back,
// so a stale or hand-edited entry falls back to the default. Storage can be unavailable
// (private windows, blocked site data); the choice then lasts for the page load.
export function remembered(key, initial, allowed) {
  let value = initial
  try {
    const v = localStorage.getItem(`metareel.${key}`)
    if (v !== null && allowed.includes(v)) value = v
  } catch {
    // no storage: keep the default
  }
  const box = $state({ value })
  return {
    get value() {
      return box.value
    },
    set value(v) {
      if (!allowed.includes(v)) return
      box.value = v
      try {
        localStorage.setItem(`metareel.${key}`, v)
      } catch {
        // no storage: the choice lasts for this page load
      }
    },
  }
}
