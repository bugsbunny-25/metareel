<script>
  import { getMapping, remapMapping } from '../lib/api.js'
  import { href } from '../lib/router.svelte.js'
  import { links, parseImdbId, parseTmdbInput, parseWikidataId, dateOnly } from '../lib/format.js'
  import { toast } from '../lib/toast.svelte.js'
  import { confirm } from '../lib/confirm.svelte.js'
  import Drawer from '../components/Drawer.svelte'
  import Icon from '../components/Icon.svelte'
  import KindBadge from '../components/KindBadge.svelte'
  import IdCell from '../components/IdCell.svelte'

  // refKey is "movie:123" / "tv:456".
  let { refKey, onclose, onchanged } = $props()

  const MATCH_BADGE = { matched: 'badge-success', manual: 'badge-accent', unmatched: 'badge-warning', pending: '' }

  let entry = $state(null)
  let loadError = $state('')
  let remap = $state({ by: 'tmdb', kind: 'movie', value: '' })
  let saving = $state(false)

  $effect(() => {
    const key = refKey
    const [kind, id] = key.split(':')
    entry = null
    loadError = ''
    getMapping(kind, id)
      .then((e) => {
        if (key !== refKey) return
        entry = e
        remap = { by: 'tmdb', kind: e.tmdb_kind, value: '' }
      })
      .catch((e) => (loadError = e.message))
  })

  const total = $derived(entry ? entry.flixpatrol_titles.length + entry.netflix_titles.length : 0)

  function normalize() {
    if (remap.by === 'tmdb') {
      const p = parseTmdbInput(remap.value)
      remap.value = p.id
      if (p.kind) remap.kind = p.kind === 'tv_show' ? 'tv' : 'movie'
    } else if (remap.by === 'imdb') {
      remap.value = parseImdbId(remap.value)
    } else {
      remap.value = parseWikidataId(remap.value)
    }
  }

  async function doRemap() {
    normalize()
    if (!remap.value) return
    const target = remap.by === 'tmdb' ? `TMDB ${remap.kind} ${remap.value}` : remap.value
    if (!(await confirm({
      title: 'Move everything to another TMDB title?',
      message: `All ${total} title(s) mapped to "${entry.title ?? entry.tmdb_id}" move to ${target} and are marked as fixed by hand. Linked titles follow.`,
      confirmLabel: 'Move all',
    }))) return
    saving = true
    try {
      const body = { tmdb_kind: remap.kind }
      if (remap.by === 'tmdb') body.tmdb_id = remap.value
      else if (remap.by === 'imdb') body.imdb_id = remap.value
      else body.wikidata_id = remap.value
      const e = await remapMapping(entry.tmdb_kind, entry.tmdb_id, body)
      toast.success(`Moved ${total} title(s) to "${e.title ?? e.tmdb_id}"`)
      onchanged?.(`${e.tmdb_kind}:${e.tmdb_id}`)
    } catch (e) {
      toast.error(e.message)
    } finally {
      saving = false
    }
  }
</script>

<Drawer {onclose}>
  {#snippet header()}
    {#if entry}
      <div class="row" style="gap:10px">
        <div class="drawer-title">{entry.title ?? entry.flixpatrol_titles[0]?.name ?? entry.netflix_titles[0]?.show_title ?? `TMDB ${entry.tmdb_id}`}{entry.year ? ` (${entry.year})` : ''}</div>
        <KindBadge kind={entry.tmdb_kind === 'tv' ? 'tv_show' : 'movie'} />
        {#if entry.conflicts}<span class="badge badge-danger">conflict</span>{/if}
      </div>
      <div class="row small muted" style="margin-top:4px">
        <span>{entry.flixpatrol_titles.length} FlixPatrol · {entry.netflix_titles.length} Netflix Top 10 title(s)</span>
        {#if entry.last_seen}·<span>last seen {dateOnly(entry.last_seen)}</span>{/if}
      </div>
    {:else}
      <div class="drawer-title">{loadError ? 'Nothing is mapped to this TMDB title' : 'Loading…'}</div>
    {/if}
  {/snippet}

  {#if loadError}
    <div class="alert alert-error"><Icon name="alert" />{loadError}</div>
  {:else if entry}
    <div class="section-title" style="margin-top:0">IDs</div>
    <dl class="kv">
      <dt>TMDB</dt><dd><IdCell value={`${entry.tmdb_kind}/${entry.tmdb_id}`} url={links.tmdb(entry.tmdb_kind, entry.tmdb_id)} /></dd>
      <dt>IMDb</dt><dd><IdCell value={entry.ids.imdb_id} url={links.imdb(entry.ids.imdb_id)} /></dd>
      <dt>Wikidata</dt><dd><IdCell value={entry.ids.wikidata_id} url={links.wikidata(entry.ids.wikidata_id)} /></dd>
      <dt>Rotten Tomatoes</dt><dd><IdCell value={entry.ids.rotten_tomatoes_id} url={links.rt(entry.ids.rotten_tomatoes_id)} /></dd>
      <dt>Netflix</dt><dd><IdCell value={entry.ids.netflix_id} url={links.netflix(entry.ids.netflix_id)} label="Not on Wikidata" /></dd>
      <dt>JustWatch</dt><dd><IdCell value={entry.ids.justwatch_id} /></dd>
      {#if entry.ids.metacritic_id}<dt>Metacritic</dt><dd><IdCell value={entry.ids.metacritic_id} /></dd>{/if}
      {#if entry.ids.letterboxd_id}<dt>Letterboxd</dt><dd><IdCell value={entry.ids.letterboxd_id} /></dd>{/if}
    </dl>
    <p class="field-hint">TMDB's own IDs and Wikidata's (looked up by the metadata job), gaps filled from the FlixPatrol titles' matches.</p>

    <div class="section-title">FlixPatrol titles</div>
    {#each entry.flixpatrol_titles as t (t.id)}
      <div class="candidate">
        <div class="meta">
          <div class="row"><a href={href('/titles', { q: t.slug, open: t.id })}><strong>{t.name}</strong></a>
            <span class="badge {MATCH_BADGE[t.match_status] ?? ''}" title={t.match_source ?? ''}>{t.match_status}</span>
            {#if t.page_kind && t.page_kind !== (entry.tmdb_kind === 'tv' ? 'tv_show' : 'movie')}<span class="badge badge-info" title="FlixPatrol's page says {t.page_kind}">page: {t.page_kind === 'tv_show' ? 'TV' : 'movie'}</span>{/if}
          </div>
          <div class="small muted row"><span class="mono">{t.slug}</span>{#if t.last_ranked_on}<span>last charted {dateOnly(t.last_ranked_on)}</span>{/if}</div>
          {#each t.links as l (l.id)}
            <div class="small">↔ <a href={href('/netflix', { open: l.id })}>{l.name}</a> (Netflix)
              {#if l.conflict}<span class="badge badge-danger" title="Mapped to TMDB {l.tmdb_kind} {l.tmdb_id}">mapped elsewhere</span>{/if}</div>
          {/each}
        </div>
      </div>
    {:else}<p class="muted small">No FlixPatrol title.</p>{/each}

    <div class="section-title">Netflix Top 10 titles</div>
    {#each entry.netflix_titles as n (n.id)}
      <div class="candidate">
        <div class="meta">
          <div class="row"><a href={href('/netflix', { open: n.id })}><strong>{n.show_title}</strong></a>
            <span class="muted small">{n.list === 'tv_show' ? 'TV' : 'Films'} list</span>
            <span class="badge {MATCH_BADGE[n.match_status] ?? ''}" title={n.match_source ?? ''}>{n.match_status}</span>
          </div>
          {#if n.last_week}<div class="small muted">last charted week ending {dateOnly(n.last_week)}</div>{/if}
          {#each n.links as l (l.id)}
            <div class="small">↔ <a href={href('/titles', { open: l.id })}>{l.name}</a> (FlixPatrol)
              {#if l.conflict}<span class="badge badge-danger" title="Mapped to TMDB {l.tmdb_kind} {l.tmdb_id}">mapped elsewhere</span>{/if}</div>
          {/each}
        </div>
      </div>
    {:else}<p class="muted small">No Netflix Top 10 title.</p>{/each}

    <div class="section-title" style="margin-top:24px">Move everything to another TMDB title</div>
    <div class="row" style="flex-wrap:wrap">
      <div class="segmented" role="group" aria-label="Identify the target by">
        <button class:active={remap.by === 'tmdb'} onclick={() => (remap.by = 'tmdb')}>TMDB ID</button>
        <button class:active={remap.by === 'imdb'} onclick={() => (remap.by = 'imdb')}>IMDb ID</button>
        <button class:active={remap.by === 'wikidata'} onclick={() => (remap.by = 'wikidata')}>Wikidata ID</button>
      </div>
      <div class="segmented" role="group" aria-label="TMDB kind">
        <button class:active={remap.kind === 'movie'} onclick={() => (remap.kind = 'movie')}>Movie</button>
        <button class:active={remap.kind === 'tv'} onclick={() => (remap.kind = 'tv')}>TV show</button>
      </div>
    </div>
    <div class="row" style="margin-top:8px">
      <input class="input mono grow" bind:value={remap.value} onblur={normalize} aria-label="Target ID"
        placeholder={remap.by === 'tmdb' ? '119051 or TMDB URL' : remap.by === 'imdb' ? 'tt1234567 or IMDb URL' : 'Q141461750 or Wikidata URL'}
        onkeydown={(e) => e.key === 'Enter' && doRemap()} />
      <button class="btn btn-primary" onclick={doRemap} disabled={saving || !remap.value.trim()}>{saving ? 'Moving…' : 'Move all'}</button>
    </div>
    <p class="field-hint">IMDb and Wikidata IDs are resolved to TMDB (TMDB's /find, then Wikidata); the kind only breaks a tie when both a movie and a show match. Every title moved is marked as fixed by hand; to fix a single title, open it instead.</p>
  {/if}
</Drawer>
