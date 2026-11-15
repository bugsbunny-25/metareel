/**
 * What a share or a pasted search names. Apps share a sentence plus a link
 * ("Check out Severance on Apple TV+ https://tv.apple.com/…", Netflix's
 * "Watch … https://www.netflix.com/title/81231974?s=a"), so the first link
 * is found in the text; IDs metareel can look up become a lookup, anything
 * else a title search with the words around the link.
 */
export type Shared =
  | { type: 'tmdb'; kind: 'movie' | 'tv'; id: string }
  | { type: 'imdb'; id: string }
  | { type: 'netflix'; id: string }
  | { type: 'letterboxd'; url: string }
  | { type: 'search'; query: string };

const URL_RE = /https?:\/\/[^\s<>"']+/i;

export function parseShared(input: string): Shared | null {
  const text = input.trim();
  if (!text) return null;
  const url = text.match(URL_RE)?.[0]?.replace(/[).,;!?]+$/, '');
  const hay = url ?? text;

  let m = hay.match(/themoviedb\.org\/(movie|tv)\/(\d+)/i);
  if (m) return { type: 'tmdb', kind: m[1].toLowerCase() as 'movie' | 'tv', id: m[2] };
  m = hay.match(/\b(tt\d{6,10})\b/i);
  if (m) return { type: 'imdb', id: m[1].toLowerCase() };
  m = hay.match(/netflix\.com\/(?:[a-z]{2}(?:-[a-z]{2})?\/)?title\/(\d+)/i);
  if (m) return { type: 'netflix', id: m[1] };
  if (url && /^https?:\/\/(?:www\.)?(?:letterboxd\.com|boxd\.it)\//i.test(url)) return { type: 'letterboxd', url };

  // Not a link metareel resolves: search with the words, or the link's slug.
  const words = text
    .replace(URL_RE, ' ')
    .replace(/\b(check out|watch|watching|on netflix|on prime video|on apple tv\+?|on disney\+?|on max|on hulu)\b/gi, ' ')
    .replace(/[“”"|·•–—:!+]+/g, ' ')
    .replace(/\s+/g, ' ')
    .trim();
  if (words.length >= 2) return { type: 'search', query: words };
  if (url) {
    const slug = url.split('?')[0].split('/').filter(Boolean).pop() ?? '';
    const q = slug.replace(/[-_]+/g, ' ').replace(/\b\d{4,}\b/g, '').trim();
    if (q.length >= 2) return { type: 'search', query: q };
  }
  return null;
}

/** The /titles/lookup query for a shared reference (not for a search). */
export function lookupQuery(r: Exclude<Shared, { type: 'search' }>): { imdb?: string; tmdb?: string; netflix?: string; letterboxd?: string } {
  switch (r.type) {
    case 'tmdb':
      return { tmdb: `${r.kind}:${r.id}` };
    case 'imdb':
      return { imdb: r.id };
    case 'netflix':
      return { netflix: r.id };
    default:
      return { letterboxd: r.url };
  }
}
