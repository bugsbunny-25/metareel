// Marking episodes watched or unwatched, shared by Up next and the show page. Both calls
// return the show's progress payload; the library map is refreshed so a status the
// server moved (watching ↔ completed) shows everywhere.
import { api } from './api.js'
import { library } from './library.svelte.js'

const zone = () => Intl.DateTimeFormat().resolvedOptions().timeZone

/**
 * Mark a run of episodes watched as of now. precision 'minute' says "I just watched it";
 * 'day' (the default for runs) says "I've seen these", without claiming a time.
 */
export async function markWatched(id, { scope = 'episode', season, episode, precision = 'day', includeSpecials = false } = {}) {
  const res = await api.markEpisodes(id, {
    scope, season_number: season, episode_number: episode, include_specials: includeSpecials,
    ended_at: new Date().toISOString(), tz: zone(), time_precision: precision,
  })
  library.refresh('tv', id).catch(() => {})
  return res
}

/** Delete every watch of the episodes a scope selects. Permanent. */
export async function unmarkWatched(id, { scope = 'episode', season, episode, includeSpecials = false } = {}) {
  const res = await api.unmarkEpisodes(id, { scope, season, episode, include_specials: includeSpecials ? 'true' : undefined })
  library.refresh('tv', id).catch(() => {})
  return res
}

/** "S1 E3", or "Special 2" for season 0. */
export const epCode = (season, episode) => (season === 0 ? `Special ${episode}` : `S${season} E${episode}`)

/** Words for TMDB's episode_type, when it's worth a badge. */
export const EPISODE_TYPE = { finale: 'Finale', mid_season: 'Mid-season finale' }
