// The premium-format taxonomy (/me/formats), loaded once per page load and shared by the
// cinema format picker and the physical-media pickers (categories disc and label).
import { api } from './api.js'

let pending = null
/** All active formats; a failed load is retried on the next call. */
export function loadFormats() {
  pending ??= api.formats().then((r) => r.formats ?? []).catch((err) => { pending = null; throw err })
  return pending
}

/** The media enum a disc format stands for, so watches keep recording `media` too. */
export const DISC_MEDIA = { 'uhd-blu-ray': 'bluray_4k', 'blu-ray': 'bluray', dvd: 'dvd' }
export const MEDIA_DISC = Object.fromEntries(Object.entries(DISC_MEDIA).map(([k, v]) => [v, k]))
