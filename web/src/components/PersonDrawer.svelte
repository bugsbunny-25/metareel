<script>
  // A TMDB person and their titles metareel knows, with chart records.
  import { getPerson } from '../lib/api.js'
  import { href } from '../lib/router.svelte.js'
  import { number, dateOnly } from '../lib/format.js'
  import Drawer from './Drawer.svelte'
  import Icon from './Icon.svelte'
  import KindBadge from './KindBadge.svelte'

  let { personId, onclose } = $props()

  let person = $state(null)
  let error = $state('')

  $effect(() => {
    const id = personId
    person = null
    error = ''
    let cancelled = false
    getPerson(id)
      .then((p) => { if (!cancelled) person = p })
      .catch((e) => { if (!cancelled) error = e.message })
    return () => { cancelled = true }
  })
</script>

<Drawer {onclose}>
  {#snippet header()}
    <div class="drawer-title">{person?.name ?? 'Person'}</div>
  {/snippet}
  {#if error}
    <div class="alert alert-error"><Icon name="alert" />{error}</div>
  {:else if !person}
    <div class="skeleton" style="height:200px"></div>
  {:else}
    <div class="row" style="gap:14px;margin-bottom:12px">
      {#if person.profile_path}<img src="https://image.tmdb.org/t/p/w185{person.profile_path}" alt="" width="64" height="96" style="border-radius:6px;object-fit:cover" />{/if}
      <div>
        <div class="small muted">{person.known_for ?? ''} · <a href="https://www.themoviedb.org/person/{person.person_id}" target="_blank" rel="noopener">TMDB {person.person_id}</a></div>
        <div class="small">{number(person.points)} chart points · {person.charting} of {person.titles.length} titles charted</div>
      </div>
    </div>
    <div class="table-wrap">
      <table class="table">
        <thead><tr><th>Title</th><th>Role</th><th class="right">Points</th><th class="right">Days</th><th class="right">Best</th><th>Charted</th></tr></thead>
        <tbody>
          {#each person.titles as t (t.kind + t.tmdb_id)}
            <tr>
              <td><a class="cell-title" style="color:inherit" href={href('/mapping', { open: `${t.kind}:${t.tmdb_id}` })}>{t.title}</a><div class="cell-sub"><KindBadge kind={t.kind === 'tv' ? 'tv_show' : 'movie'} /> {t.year ?? ''}</div></td>
              <td class="small">{t.roles.join(', ')}</td>
              <td class="right">{number(t.points)}</td>
              <td class="right">{t.days || '—'}</td>
              <td class="right">{t.best_rank ? `#${t.best_rank}` : '—'}</td>
              <td class="small nowrap">{t.first_charted ? `${dateOnly(t.first_charted)} – ${dateOnly(t.last_charted)}` : '—'}</td>
            </tr>
          {/each}
        </tbody>
      </table>
    </div>
    <p class="field-hint">Credits from TMDB; chart records over every stored FlixPatrol chart.</p>
  {/if}
</Drawer>
