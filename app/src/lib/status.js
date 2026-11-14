// Title statuses as the API stores them, with the words the UI shows.
export const STATUSES = [
  ['watching', 'Watching'], ['completed', 'Completed'], ['watchlist', 'Watchlist'], ['on_hold', 'On hold'], ['dropped', 'Dropped'], ['not_interested', 'Not interested'],
]
const LABEL = Object.fromEntries(STATUSES)

/** What to call a library entry's state; a title with watches but no status reads "Watched". */
export const statusLabel = (t) => LABEL[t?.status] ?? (t?.watch_count > 0 ? 'Watched' : 'In your library')
