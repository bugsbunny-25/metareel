// lib/rows.js — the one view model every direction renders.
// Pure functions: components call chartRow() inside $derived, so reactivity
// comes from the $state entry passed in, not from anything in here.

const TMDB_IMG = 'https://image.tmdb.org/t/p/';
const MON = ['Jan', 'Feb', 'Mar', 'Apr', 'May', 'Jun', 'Jul', 'Aug', 'Sep', 'Oct', 'Nov', 'Dec'];

/** TMDB returns paths; JustWatch poster_url (and test fixtures) are already full URLs. */
export const posterSrc = (path, size = 'w342') =>
  !path ? null : /^(https?:|data:)/.test(path) ? path : `${TMDB_IMG}${size}${path}`;

/** FlixPatrol `movie | tv_show` → TMDB namespace `movie | tv`. Label by this, never by chart. */
export const tmdbKind = (k) => (k === 'tv_show' || k === 'tv' ? 'tv' : 'movie');

/** A stand-up special (TMDB movie) on a TV chart, or the reverse. */
export const offChart = (kind, category) =>
  (kind === 'movie' && category === 'tv_shows') || (kind === 'tv' && category === 'movies');

/**
 * previous_rank is null for debuts AND re-entries.
 * days_in_top10 counts every date on this chart, so > 1 means it was here before.
 */
export function movement({ rank, previous_rank, days_in_top10 }) {
  if (previous_rank == null) return { type: days_in_top10 <= 1 ? 'debut' : 'reentry', by: 0 };
  const by = previous_rank - rank;
  return { type: by > 0 ? 'up' : by < 0 ? 'down' : 'same', by: Math.abs(by) };
}

export function moveLabel(m, prev) {
  switch (m.type) {
    case 'up': return `Up ${m.by} from number ${prev}`;
    case 'down': return `Down ${m.by} from number ${prev}`;
    case 'same': return `Unchanged from number ${prev}`;
    case 'debut': return 'New on this chart';
    default: return 'Back on this chart';
  }
}

const ORIGIN = { original: 'Original', in_house: 'In-house', licensed: 'Licensed' };
export const originOf = (o) =>
  o && ORIGIN[o.origin] ? { key: o.origin, label: ORIGIN[o.origin], owner: o.owner, reason: o.reason } : null;

const fromSources = (r, source) => r.sources?.find((s) => s.source === source)?.value;

/** RatingsResponse → ordered list on native scales. Letterboxd and Metacritic live in sources[]. */
export function ratingList(r) {
  if (!r) return [];
  const out = [];
  if (r.imdb?.rating != null)
    out.push({ key: 'imdb', short: 'IMDb', label: 'IMDb', value: r.imdb.rating.toFixed(1), max: 10, n: r.imdb.rating });
  const rt = r.rotten_tomatoes;
  if (rt?.critics_score != null)
    out.push({ key: 'rt', short: 'RT', label: 'Rotten Tomatoes critics', value: String(rt.critics_score), unit: '%',
      max: 100, n: rt.critics_score, certified: !!rt.certified_fresh, rotten: rt.critics_score < 60 });
  const tmdb = fromSources(r, 'tmdb');
  if (tmdb != null) out.push({ key: 'tmdb', short: 'TMDB', label: 'TMDB', value: tmdb.toFixed(1), max: 10, n: tmdb });
  const lb = fromSources(r, 'letterboxd');
  if (lb != null) out.push({ key: 'lb', short: 'LB', label: 'Letterboxd, out of 5', value: lb.toFixed(1), max: 5, n: lb });
  const mc = fromSources(r, 'metacritic');
  if (mc != null) out.push({ key: 'mc', short: 'MC', label: 'Metacritic', value: String(Math.round(mc)), max: 100, n: mc });
  return out;
}

/** /me/titles state → the one mark a chart row shows. Watched wins over watchlist. */
export function userMark(state) {
  if (state?.watch_count > 0) return { kind: 'watched', count: state.watch_count, rating: state.rating ?? null };
  if (state?.status === 'watchlist') return { kind: 'watchlist', count: 0, rating: null };
  return { kind: 'none', count: 0, rating: null };
}

/** 1–10 half-stars → "★★★★½". There is no zero. */
export const stars = (r) => (r ? '★'.repeat(Math.floor(r / 2)) + (r % 2 ? '½' : '') : '');
export const starsLabel = (r) => (r ? `rated ${r / 2} of 5` : 'not rated');

export const shortDate = (d) => {
  const [, m, day] = d.slice(0, 10).split('-');
  return `${+day} ${MON[+m - 1]}`;
};

/**
 * §7 joins. `offers` is this title's OfferEvents from /availability/changes, already
 * intersected with /me/watchlist by the page. The feed reports a removal once it is
 * observed, so the copy says "Left", not "Leaving".
 */
export const offerFlags = (offers = []) =>
  offers.map((e) => ({ type: e.event === 'added' ? 'arrived' : 'left', provider: e.provider_name, date: e.observed_at.slice(0, 10) }));

/**
 * entry = { item: Top10Result, category, poster_path?, ratings?, state?, offers? }
 * poster_path is joined in: Top10Result does not carry one (see the gap note).
 */
export function chartRow({ item, category, poster_path = null, ratings = null, state = null, offers = [] }) {
  const kind = tmdbKind(item.kind);
  return {
    rank: item.rank,
    prev: item.previous_rank,
    days: item.days_in_top10,
    move: movement(item),
    name: item.name,
    season: item.season_number ?? null,
    kind,
    category,
    offChart: offChart(kind, category),
    origin: originOf(item.origin),
    poster: poster_path,
    seed: item.tmdb_id ?? item.slug,
    href: item.tmdb_id ? `#/title/${kind}/${item.tmdb_id}` : null,
    scores: ratingList(ratings),
    stale: !!ratings?.stale,
    mark: userMark(state),
    flags: offerFlags(offers),
  };
}
