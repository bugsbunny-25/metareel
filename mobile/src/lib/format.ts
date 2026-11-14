// Formatting shared by the screens: images, episodes, money, dates, stars.

const TMDB_IMG = 'https://image.tmdb.org/t/p/';

/** A TMDB poster/logo URL from a path ("/abc.jpg"); full URLs pass through. */
export function tmdbImage(path: string | null | undefined, size = 'w342'): string | null {
  if (!path) return null;
  return /^https?:/.test(path) ? path : `${TMDB_IMG}${size}${path}`;
}

export function episodeCode(season?: number | null, episode?: number | null): string {
  const pad = (n?: number | null) => String(n ?? 0).padStart(2, '0');
  return `S${pad(season)} E${pad(episode)}`;
}

/** FlixPatrol kind (movie | tv_show) or TMDB kind → TMDB namespace (movie | tv). */
export const tmdbKind = (k?: string | null): 'movie' | 'tv' => (k === 'tv_show' || k === 'tv' ? 'tv' : 'movie');

export const year = (date?: string | null) => (date && /^\d{4}/.test(date) ? date.slice(0, 4) : '');

// --- money: integer minor units with an ISO 4217 code, never floats.

const digits = new Map<string, number>();
export function fractionDigits(currency: string): number {
  if (!digits.has(currency)) {
    let d = 2;
    try {
      d = new Intl.NumberFormat('en', { style: 'currency', currency }).resolvedOptions().maximumFractionDigits ?? 2;
    } catch {
      // Unknown code: assume cents.
    }
    digits.set(currency, d);
  }
  return digits.get(currency)!;
}

/** "12.50" → 1250 (USD); "" → null; not a non-negative number → undefined. */
export function toMinor(text: string, currency: string): number | null | undefined {
  const s = text.trim().replace(/,/g, '');
  if (s === '') return null;
  const n = Number(s);
  if (!Number.isFinite(n) || n < 0) return undefined;
  return Math.round(n * 10 ** fractionDigits(currency));
}

export function formatMoney(minor: number | null | undefined, currency: string | null | undefined): string {
  if (minor == null || !currency) return '';
  try {
    return new Intl.NumberFormat(undefined, { style: 'currency', currency }).format(minor / 10 ** fractionDigits(currency));
  } catch {
    return `${(minor / 10 ** fractionDigits(currency)).toFixed(fractionDigits(currency))} ${currency}`;
  }
}

const BY_COUNTRY: Record<string, string> = {
  US: 'USD', GB: 'GBP', IN: 'INR', CA: 'CAD', AU: 'AUD', NZ: 'NZD', JP: 'JPY', KR: 'KRW', BR: 'BRL', MX: 'MXN',
  CH: 'CHF', SE: 'SEK', NO: 'NOK', DK: 'DKK', PL: 'PLN', SG: 'SGD', AE: 'AED', ZA: 'ZAR', HK: 'HKD',
};
const EURO = new Set(['DE', 'FR', 'ES', 'IT', 'NL', 'IE', 'BE', 'AT', 'PT', 'FI', 'GR', 'LU', 'SK', 'SI', 'EE', 'LV', 'LT', 'HR', 'CY', 'MT']);
export const currencyFor = (country?: string | null) => (country ? (BY_COUNTRY[country] ?? (EURO.has(country) ? 'EUR' : 'USD')) : 'USD');

// --- dates. The server takes UTC instants plus an IANA zone; the zone decides the day.

export function deviceTimeZone(): string {
  try {
    return Intl.DateTimeFormat().resolvedOptions().timeZone || 'UTC';
  } catch {
    return 'UTC';
  }
}

export type When = 'now' | 'hour' | 'today' | 'yesterday';

/**
 * When a quick log says the film ended. "Earlier today" and "Yesterday" are
 * day-precision watches, stored (like the web) as noon local so the instant
 * stays inside the right day whatever the offset.
 */
export function endedAt(when: When, now = new Date()): { ended_at: string; time_precision: 'minute' | 'day' } {
  switch (when) {
    case 'hour':
      return { ended_at: new Date(now.getTime() - 60 * 60_000).toISOString(), time_precision: 'minute' };
    case 'today':
      return { ended_at: new Date(now.getFullYear(), now.getMonth(), now.getDate(), 12).toISOString(), time_precision: 'day' };
    case 'yesterday':
      return { ended_at: new Date(now.getFullYear(), now.getMonth(), now.getDate() - 1, 12).toISOString(), time_precision: 'day' };
    default:
      return { ended_at: now.toISOString(), time_precision: 'minute' };
  }
}

/** A diary day heading: "Today", "Yesterday", or "Sat 14 Nov" (with the year if not this year). */
export function dayHeading(day: string, today = new Date()): string {
  const [y, m, d] = day.split('-').map(Number);
  const date = new Date(y, m - 1, d);
  const t = new Date(today.getFullYear(), today.getMonth(), today.getDate());
  const diff = Math.round((t.getTime() - date.getTime()) / 86_400_000);
  if (diff === 0) return 'Today';
  if (diff === 1) return 'Yesterday';
  return date.toLocaleDateString(undefined, {
    weekday: 'short',
    day: 'numeric',
    month: 'short',
    ...(y !== today.getFullYear() ? { year: 'numeric' } : {}),
  });
}

/** 1..10 half-stars → "★★★½". */
export function starText(rating?: number | null): string {
  if (!rating) return '';
  return '★'.repeat(Math.floor(rating / 2)) + (rating % 2 ? '½' : '');
}

/** Minutes → "2h 28m". */
export function runtimeText(min?: number | null): string {
  if (!min) return '';
  const h = Math.floor(min / 60);
  return h ? `${h}h ${min % 60}m` : `${min}m`;
}
