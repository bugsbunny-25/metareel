// Plain-language definitions shown in the glossary drawer and in (i) tips.
// Keep each definition to one or two sentences.

export const GLOSSARY = [
  {
    group: 'Charts',
    terms: [
      ['Top 10 chart', 'A daily ranking of the 10 most-watched movies or TV shows on one streaming service in one country, scraped from FlixPatrol.'],
      ['Points', 'How a title scores across charts: #1 earns 10 points, #10 earns 1, for every chart and day it appears on. Leaderboards add them up.'],
      ['Days on chart', 'How many days a title has been in a chart’s Top 10. A gap means it dropped out and came back.'],
      ['Movers', 'Changes against the previous day’s chart: climbers and fallers moved rank, debuts are new, re-entries came back and exits dropped out.'],
      ['Netflix official Top 10', 'Netflix’s own weekly lists (Monday to Sunday) from netflix.com/tudum/top10, with views and hours viewed for the global lists.'],
    ],
  },
  {
    group: 'Mapping',
    terms: [
      ['Title', 'One entry as a chart names it: a FlixPatrol title or a Netflix Top 10 title. The same film can appear under different names.'],
      ['Mapping', 'Linking a title to its TMDB movie or show. Every ID (IMDb, Rotten Tomatoes, Wikidata, Netflix, JustWatch) hangs off that TMDB title.'],
      ['TMDB ID', 'The Movie Database identifier. It is the key everything else maps to; a title without one has no ratings, genres or release dates.'],
      ['Match status', 'Pending: not tried yet. Matched: found automatically. Unmatched: tried, no confident match (retried later). Manual: set by hand, never changed automatically.'],
      ['Coverage', 'The share of titles, or of chart entries, that have a TMDB ID. Higher is better; gaps show up as missing ratings and metadata downstream.'],
      ['Season mapping', 'Which TMDB season a Netflix season title (“Season 2”, “Part 3”) refers to, so rankings can be shown per season.'],
      ['Mapping history', 'A log of every change to a mapping and the evidence behind it, so a wrong match can be traced and undone.'],
    ],
  },
  {
    group: 'Operations',
    terms: [
      ['Schedule', 'A job that runs automatically at set UTC times each day: a FlixPatrol scrape, title matching, metadata refresh, an import or maintenance.'],
      ['Job run', 'One attempt of a job. A failed attempt is retried up to its retry limit; each retry is its own run.'],
      ['Partial run', 'A scrape where some charts succeeded and others failed. Only the failed charts are retried.'],
      ['Targets', 'The country and service charts a scrape covers. A run reports how many targets succeeded, failed or were skipped.'],
      ['Stale chart', 'A chart whose latest stored date is older than expected, usually because its scrape keeps failing.'],
      ['Chart gap', 'A missing day inside a chart’s history. A backfill can fetch it again.'],
      ['FlareSolverr', 'The proxy that loads FlixPatrol pages for the scraper. If it is down, every scrape fails.'],
    ],
  },
  {
    group: 'Analytics',
    terms: [
      ['Release lag', 'Days from a title’s release date (TMDB) to its first appearance on a chart.'],
      ['Decay', 'Of titles that debuted, the share still charting N days later. It shows how fast popularity fades.'],
      ['Country similarity', 'How much two countries’ charts overlap: shared titles divided by all titles either one charted (Jaccard index).'],
      ['Correlation (r)', 'How closely ratings track popularity, from −1 to 1. Near 0 means a high rating says little about how much a title is watched.'],
    ],
  },
]

/** Look up one definition by term (case-insensitive). */
export function define(term) {
  const t = term.toLowerCase()
  for (const g of GLOSSARY) for (const [k, v] of g.terms) if (k.toLowerCase() === t) return v
  return ''
}
