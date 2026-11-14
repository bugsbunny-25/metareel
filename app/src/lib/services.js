// Streaming offers come per plan and channel ("HBO Max Amazon Channel", "Netflix Standard with
// Ads"). Discover groups them under the service you'd actually pay: by the provider ids the
// rotation endpoint lists per service, else by tidying the name.

const SUFFIX = / (Amazon Channel|Apple TV Channel|Roku Premium Channel|Standard with Ads|Basic with Ads|with Ads|Premium Plus|Premium|Essential)$/i
const RENAME = { 'Amazon Prime Video': 'Prime Video', 'Disney Plus': 'Disney+', 'Paramount Plus': 'Paramount+', 'Apple TV Plus': 'Apple TV+' }

/** families: Map(provider id → service name), from rotation services' provider_ids. */
export function familyMap(services = []) {
  const m = new Map()
  for (const s of services) for (const id of s.provider_ids ?? [s.provider_id]) m.set(id, s.provider_name)
  return m
}

/** The service an offer belongs to. */
export function serviceOf(offer, families) {
  const known = families?.get(offer.provider_id)
  if (known) return known
  let n = offer.provider_name ?? ''
  for (let i = 0; i < 2 && SUFFIX.test(n); i++) n = n.replace(SUFFIX, '')
  return RENAME[n] ?? n
}

/** Distinct services for a list of offers, in order. */
export const servicesOf = (offers = [], families) => [...new Set(offers.map((o) => serviceOf(o, families)))]

/** "1.2M", "612k", "5,400". */
export function votes(n) {
  if (n == null) return ''
  if (n >= 1e6) return `${(n / 1e6).toFixed(n >= 1e7 ? 0 : 1)}M`
  if (n >= 1e4) return `${Math.round(n / 1e3)}k`
  return n.toLocaleString()
}

const region = new Intl.DisplayNames(undefined, { type: 'region' })
/** "United Kingdom" for GB. */
export const regionName = (cc) => { try { return region.of(cc) ?? cc } catch { return cc } }
