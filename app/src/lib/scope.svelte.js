// The country and service every chart screen reads, remembered per browser.
import { api } from './api.js'

const KEY = 'metareel.scope'

function saved() {
  try {
    return JSON.parse(localStorage.getItem(KEY) ?? 'null') ?? {}
  } catch {
    return {}
  }
}

const initial = saved()
/** Whether this browser has a country of its own; if not, the user's saved default applies. */
export const hasSavedScope = !!initial.country
export const scope = $state({ country: initial.country ?? 'US', provider: initial.provider ?? 'netflix' })

/** Reference data for the pickers: countries that have charts, and each service's countries. */
export const reference = $state({ loaded: false, countries: [], providers: [] })

export function setScope(patch) {
  Object.assign(scope, patch)
  // Keep the pair valid: a service must chart in the chosen country.
  const p = reference.providers.find((x) => x.slug === scope.provider)
  if (p && !p.countries.includes(scope.country)) {
    scope.provider = reference.providers.find((x) => x.countries.includes(scope.country))?.slug ?? scope.provider
  }
  try {
    localStorage.setItem(KEY, JSON.stringify({ country: scope.country, provider: scope.provider }))
  } catch {
    // storage blocked: the choice lasts for this visit only
  }
}

export async function loadReference() {
  if (reference.loaded) return
  const [countries, providers] = await Promise.all([api.countries(), api.providers()])
  const charted = new Set(providers.flatMap((p) => p.countries))
  reference.countries = countries.filter((c) => charted.has(c.code))
  reference.providers = providers
  reference.loaded = true
}

const PROVIDER_NAMES = {
  netflix: 'Netflix', 'amazon-prime': 'Prime Video', 'apple-tv': 'Apple TV+', disney: 'Disney+', 'hbo-max': 'HBO Max',
  hulu: 'Hulu', 'paramount-plus': 'Paramount+', peacock: 'Peacock',
}
export const providerName = (slug) => PROVIDER_NAMES[slug] ?? slug.replace(/-/g, ' ').replace(/\b\w/g, (c) => c.toUpperCase())
export const countryName = (code) => reference.countries.find((c) => c.code === code)?.name ?? code
/** "in the United States", "in India". */
export const inCountry = (code) => (['US', 'GB', 'NL', 'AE', 'PH'].includes(code) ? 'the ' : '') + countryName(code)
