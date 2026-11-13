// Small, dependency-free chart and format helpers shared by the three directions.

export const COUNTRY = { US: 'United States', GB: 'United Kingdom', IN: 'India' };
/** Country name as it reads mid-sentence: "in the United Kingdom", "in India". */
export const inCountry = (cc) => (cc === 'US' || cc === 'GB' ? 'the ' : '') + (COUNTRY[cc] ?? cc);
export const PROVIDER = { netflix: 'Netflix', 'amazon-prime': 'Prime Video', 'apple-tv': 'Apple TV+' };

/** Rank series (1 = top, null = off chart) → SVG path; rank 1 at the top. Gaps break the line. */
export function rankPath(ranks, w, h, maxRank = 10, pad = 2) {
  const n = ranks.length;
  const x = (i) => pad + (i * (w - pad * 2)) / Math.max(1, n - 1);
  const y = (r) => pad + ((r - 1) * (h - pad * 2)) / (maxRank - 1);
  let d = '';
  let pen = false;
  ranks.forEach((r, i) => {
    if (r == null) { pen = false; return; }
    d += `${pen ? 'L' : 'M'}${x(i).toFixed(1)} ${y(r).toFixed(1)}`;
    pen = true;
  });
  return { d, x, y };
}

/** Value series → area + line paths scaled to the box (min at bottom). */
export function seriesPath(values, w, h, pad = 2) {
  const max = Math.max(...values), min = Math.min(0, ...values);
  const x = (i) => pad + (i * (w - pad * 2)) / Math.max(1, values.length - 1);
  const y = (v) => h - pad - ((v - min) * (h - pad * 2)) / Math.max(1, max - min);
  const line = values.map((v, i) => `${i ? 'L' : 'M'}${x(i).toFixed(1)} ${y(v).toFixed(1)}`).join('');
  return { line, area: `${line}L${x(values.length - 1).toFixed(1)} ${h - pad}L${x(0).toFixed(1)} ${h - pad}Z`, x, y, max };
}

/** New entries per day in a chart history: titles on day i that were not on day i-1. */
export function debutsPerDay(entriesHist, nDays) {
  const out = Array(nDays).fill(0);
  for (const ranks of entriesHist) for (let i = 1; i < nDays; i++) if (ranks?.[i] != null && ranks[i - 1] == null) out[i]++;
  return out.slice(1);
}

export const compact = (n) =>
  n >= 1e9 ? (n / 1e9).toFixed(1) + 'B' : n >= 1e6 ? (n / 1e6).toFixed(1) + 'M' : n >= 1e3 ? Math.round(n / 1e3) + 'K' : String(n);

const MON = ['Jan', 'Feb', 'Mar', 'Apr', 'May', 'Jun', 'Jul', 'Aug', 'Sep', 'Oct', 'Nov', 'Dec'];
export const dayMonth = (d) => `${+d.slice(8, 10)} ${MON[+d.slice(5, 7) - 1]}`;
export const longDate = (d) => {
  const dt = new Date(d + 'T12:00:00Z');
  return dt.toLocaleDateString('en-GB', { weekday: 'long', day: 'numeric', month: 'long', year: 'numeric', timeZone: 'UTC' });
};

/** Average colour of an image (same-origin or data: URL) for artwork-tinted surfaces. */
export function sampleColor(src) {
  return new Promise((resolve) => {
    const img = new Image();
    img.onload = () => {
      try {
        const c = document.createElement('canvas');
        c.width = c.height = 8;
        const g = c.getContext('2d', { willReadFrequently: true });
        g.drawImage(img, 0, 0, 8, 8);
        const px = g.getImageData(0, 0, 8, 8).data;
        let r = 0, gg = 0, b = 0;
        for (let i = 0; i < px.length; i += 4) { r += px[i]; gg += px[i + 1]; b += px[i + 2]; }
        const n = px.length / 4;
        resolve([r / n, gg / n, b / n].map(Math.round));
      } catch { resolve(null); }
    };
    img.onerror = () => resolve(null);
    // Cross-origin artwork (image.tmdb.org) needs a CORS request or the canvas is tainted;
    // if CORS is refused we resolve null and the hero stays untinted.
    img.crossOrigin = 'anonymous';
    img.src = src;
  });
}
