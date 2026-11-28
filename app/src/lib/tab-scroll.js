import { tick } from 'svelte'

// Tabs that swap a page's content (To watch / Upcoming) share the window's scroll, so a switch
// would land the new tab wherever the old one was scrolled to. tabScroll() remembers each tab's
// own position for the page's lifetime: switching saves the old one, renders the new tab, then
// returns it to where it was left (the top the first time).
export function tabScroll() {
  const pos = new Map()
  return async (from, to, show) => {
    if (from === to) return
    pos.set(from, window.scrollY)
    show()
    await tick()
    window.scrollTo({ top: pos.get(to) ?? 0, behavior: 'instant' })
  }
}
