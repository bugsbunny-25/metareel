// Money is stored in minor units (cents, paise) with an ISO 4217 code, so it is never a float.
// These convert what a person types ("12.50") to and from minor units for a currency.

const digitsCache = new Map()
/** Fraction digits a currency uses: 2 for USD, 0 for JPY, 3 for KWD. */
export function fractionDigits(currency) {
  if (!digitsCache.has(currency)) {
    let d = 2
    try { d = new Intl.NumberFormat('en', { style: 'currency', currency }).resolvedOptions().maximumFractionDigits } catch { /* unknown code */ }
    digitsCache.set(currency, d)
  }
  return digitsCache.get(currency)
}

/** "12.50" → 1250 for USD; '' → null; NaN or negative → undefined (invalid). */
export function toMinor(text, currency) {
  const s = String(text ?? '').trim().replace(/,/g, '')
  if (s === '') return null
  const n = Number(s)
  if (!Number.isFinite(n) || n < 0) return undefined
  return Math.round(n * 10 ** fractionDigits(currency))
}

/** 1250 → "12.50" for USD; null → ''. */
export function fromMinor(minor, currency) {
  if (minor == null) return ''
  const d = fractionDigits(currency)
  return (minor / 10 ** d).toFixed(d)
}

const BY_COUNTRY = {
  US: 'USD', GB: 'GBP', IN: 'INR', CA: 'CAD', AU: 'AUD', NZ: 'NZD', JP: 'JPY', KR: 'KRW', BR: 'BRL', MX: 'MXN',
  CH: 'CHF', SE: 'SEK', NO: 'NOK', DK: 'DKK', PL: 'PLN', SG: 'SGD', AE: 'AED', ZA: 'ZAR', HK: 'HKD',
}
const EURO = new Set(['DE', 'FR', 'ES', 'IT', 'NL', 'IE', 'BE', 'AT', 'PT', 'FI', 'GR', 'LU', 'SK', 'SI', 'EE', 'LV', 'LT', 'HR', 'CY', 'MT'])

/** A sensible currency for a country, for prefilling. */
export const currencyFor = (country) => BY_COUNTRY[country] ?? (EURO.has(country) ? 'EUR' : 'USD')

/** Remembered choices from the last log (per browser; convenience only). */
export function remember(key, value) {
  try { localStorage.setItem(`metareel.log.${key}`, JSON.stringify(value)) } catch { /* storage off */ }
}
export function recall(key, fallback = null) {
  try { return JSON.parse(localStorage.getItem(`metareel.log.${key}`)) ?? fallback } catch { return fallback }
}
