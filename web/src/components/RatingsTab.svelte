<script>
  // A TMDB title's ratings (IMDb, Rotten Tomatoes), cached, with a refresh.
  import { getRatings } from '../lib/api.js'
  import { relativeTime, dateTime, number } from '../lib/format.js'
  import { toast } from '../lib/toast.svelte.js'
  import Icon from './Icon.svelte'

  let { tmdbKind = '', tmdbId = '' } = $props()

  let ratings = $state(null)
  let error = $state('')
  let loading = $state(false)
  let reload = $state(0)

  async function load(refresh) {
    loading = true
    error = ''
    try {
      ratings = await getRatings(tmdbKind, tmdbId, refresh)
      if (refresh) toast.success('Ratings refreshed')
    } catch (e) {
      error = e.message
    } finally {
      loading = false
    }
  }

  $effect(() => {
    void reload
    ratings = null
    if (tmdbId) load(false)
  })
</script>

{#if !tmdbId}
  <div class="empty"><div class="empty-title">Not mapped to TMDB</div>Ratings are looked up by TMDB ID. Map this title first.</div>
{:else if error}
  <div class="alert alert-error"><Icon name="alert" />{error}</div>
  <button class="btn" onclick={() => reload++}>Retry</button>
{:else if !ratings}
  <div class="skeleton" style="height:90px"></div>
{:else}
  <div class="row" style="justify-content:space-between;margin-bottom:12px">
    <span class="muted small">
      {ratings.cached ? 'From cache' : 'Fetched'} · refreshed <span title={dateTime(ratings.refreshed_at)}>{relativeTime(ratings.refreshed_at)}</span>
      {#if ratings.stale}<span class="badge badge-warning">stale</span>{/if}
    </span>
    <button class="btn btn-sm" onclick={() => load(true)} disabled={loading}><Icon name="refresh" size={12} />Refresh now</button>
  </div>
  <div class="grid" style="grid-template-columns:repeat(3,1fr)">
    <div class="card stat">
      <div class="stat-label">IMDb</div>
      <div class="score">{ratings.imdb ? ratings.imdb.rating.toFixed(1) : '—'}</div>
      <div class="stat-foot">{ratings.imdb ? `${number(ratings.imdb.votes)} votes · via ${ratings.imdb.source}` : 'No rating'}</div>
    </div>
    <div class="card stat">
      <div class="stat-label">Tomatometer</div>
      <div class="score">{ratings.rotten_tomatoes?.critics_score ?? '—'}{ratings.rotten_tomatoes?.critics_score !== null && ratings.rotten_tomatoes ? '%' : ''}</div>
      <div class="stat-foot">{ratings.rotten_tomatoes?.critics_rating ?? 'No score'}{ratings.rotten_tomatoes?.critics_source ? ` · via ${ratings.rotten_tomatoes.critics_source}` : ''}</div>
    </div>
    <div class="card stat">
      <div class="stat-label">Popcornmeter</div>
      <div class="score">{ratings.rotten_tomatoes?.audience_score ?? '—'}{ratings.rotten_tomatoes?.audience_score !== null && ratings.rotten_tomatoes ? '%' : ''}</div>
      <div class="stat-foot">{ratings.rotten_tomatoes?.audience_rating ?? 'No score'}{ratings.rotten_tomatoes?.audience_source ? ` · via ${ratings.rotten_tomatoes.audience_source}` : ''}</div>
    </div>
  </div>
  <div class="section-title">All sources</div>
  <div class="card table-wrap">
    <table class="table">
      <thead><tr><th>Provider</th><th>Source</th><th class="right">Value</th><th class="right">Votes</th><th>Fetched</th></tr></thead>
      <tbody>
        {#each ratings.sources as s (s.provider + s.source)}
          <tr>
            <td>{s.provider}</td>
            <td>{#if s.url}<a href={s.url} target="_blank" rel="noopener">{s.source}</a>{:else}{s.source}{/if}</td>
            <td class="right mono">{s.value}</td>
            <td class="right">{number(s.votes)}</td>
            <td class="muted small">{relativeTime(s.fetched_at)}</td>
          </tr>
        {:else}
          <tr><td colspan="5"><div class="empty small">No rating sources yet.</div></td></tr>
        {/each}
      </tbody>
    </table>
  </div>
{/if}
