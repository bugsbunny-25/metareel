// One-line confirmations ("Added to your watchlist"), announced to screen readers.
export const toasts = $state({ items: [] })

let next = 1
export function toast(text, kind = 'ok') {
  const id = next++
  toasts.items.push({ id, text, kind })
  setTimeout(() => {
    const i = toasts.items.findIndex((t) => t.id === id)
    if (i >= 0) toasts.items.splice(i, 1)
  }, kind === 'error' ? 6000 : 3200)
}
