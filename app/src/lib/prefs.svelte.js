// The user's saved preferences (GET/PUT /me/preferences): they follow the user to every device.
// country/provider are the defaults a new device opens with; currency and screen_unit prefill
// the log form; stale_days is applied by the server to the Up next stages.
import { api } from './api.js'
import { hasSavedScope, setScope } from './scope.svelte.js'

export const prefs = $state({ loaded: false, values: {}, updatedAt: null })

export async function loadPrefs() {
  const r = await api.preferences()
  prefs.values = r.preferences ?? {}
  prefs.updatedAt = r.updated_at ?? null
  prefs.loaded = true
  // A browser that never picked a country opens on the user's default.
  if (!hasSavedScope && prefs.values.country) {
    setScope({ country: prefs.values.country, ...(prefs.values.provider ? { provider: prefs.values.provider } : {}) })
  }
}

export async function savePrefs(values) {
  const r = await api.setPreferences(values)
  prefs.values = r.preferences ?? {}
  prefs.updatedAt = r.updated_at ?? null
  return prefs.values
}

/** Change some preferences, keeping the rest (PUT replaces them as a whole). */
export const updatePrefs = (patch) => savePrefs({ ...prefs.values, ...patch })
