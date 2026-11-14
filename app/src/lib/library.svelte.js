// The signed-in user's titles, keyed "kind:tmdb_id" so chart rows can join them in O(1).
// Tracking responses are no-store, so this map is the only client-side copy; every
// write updates it from the server's reply.
import { SvelteMap } from 'svelte/reactivity'
import { api } from './api.js'

export const libKey = (kind, id) => `${kind}:${id}`

class Library {
  byKey = new SvelteMap()
  loaded = $state(false)

  async load() {
    const all = []
    for (let offset = 0; ; ) {
      const page = await api.myTitles(offset)
      all.push(...page.titles)
      offset += page.titles.length
      if (!page.titles.length || offset >= page.total) break
    }
    this.byKey.clear()
    for (const t of all) this.byKey.set(libKey(t.tmdb_kind, t.tmdb_id), t)
    this.loaded = true
  }

  get(kind, id) {
    return id ? this.byKey.get(libKey(kind, id)) ?? null : null
  }

  /** Titles you want or are in the middle of. */
  get active() {
    return [...this.byKey.values()].filter((t) => t.status === 'watchlist' || t.status === 'watching')
  }

  #apply(kind, id, res) {
    const state = res?.state
    if (!state) {
      this.byKey.delete(libKey(kind, id))
      return
    }
    const prev = this.byKey.get(libKey(kind, id)) ?? {}
    this.byKey.set(libKey(kind, id), { ...prev, ...state, title: res.title?.title ?? prev.title, poster_path: res.title?.poster_path ?? prev.poster_path })
  }

  async toggleWatchlist(kind, id) {
    const on = this.get(kind, id)?.status === 'watchlist'
    const res = on ? await api.removeFromWatchlist(kind, id) : await api.addToWatchlist(kind, id)
    if (on && !res) {
      const prev = this.get(kind, id)
      // The server keeps the row while it has watches or is a favourite.
      if (prev?.watch_count > 0 || prev?.favorite) this.byKey.set(libKey(kind, id), { ...prev, status: '', watchlist_position: null })
      else this.byKey.delete(libKey(kind, id))
    } else {
      this.#apply(kind, id, res)
    }
    return !on
  }

  /** Set status and/or favourite. Returns the stored state. */
  async setState(kind, id, patch) {
    const res = await api.setTitle(kind, id, patch)
    this.#apply(kind, id, res)
    return res?.state ?? null
  }

  /** Clear status, favourite and note. Titles with watches stay (as "Watched"); others leave the library. */
  async clear(kind, id) {
    await api.clearTitle(kind, id)
    const prev = this.get(kind, id)
    if (prev?.watch_count > 0) this.byKey.set(libKey(kind, id), { ...prev, status: '', favorite: false, watchlist_note: null, watchlist_position: null })
    else this.byKey.delete(libKey(kind, id))
  }

  /** Re-read one title after a write that changes derived fields (rating, watch count). */
  async refresh(kind, id) {
    this.#apply(kind, id, await api.myTitle(kind, id))
  }
}

export const library = new Library()
