<script>
  // Headline number with an explanation, an optional trend line or meter,
  // and an optional link to the detail page (the whole tile is clickable).
  import InfoTip from './InfoTip.svelte'
  import Icon from './Icon.svelte'
  import Sparkline from './Sparkline.svelte'

  let {
    label,
    value,
    unit = '',
    icon = '',
    info = '',
    href = '',
    spark = null, // number[]
    sparkColor = 'var(--c1)',
    meter = null, // 0..100
    meterColor = 'var(--success)',
    tone = '', // '' | 'danger' | 'warning'
    foot,
  } = $props()
</script>

<div class="card kpi {tone ? `tone-${tone}` : ''}" style="position:relative">
  <div class="kpi-label">
    {#if icon}<Icon name={icon} size={14} />{/if}
    <span>{label}</span>
    {#if info}<span style="position:relative;z-index:2"><InfoTip text={info} label={`About ${label}`} /></span>{/if}
  </div>
  <div class="kpi-value">{value}{#if unit}<span class="unit">{unit}</span>{/if}</div>
  {#if foot}<div class="kpi-foot">{@render foot()}</div>{/if}
  {#if spark && spark.length > 1}
    <div class="kpi-spark"><Sparkline values={spark} color={sparkColor} /></div>
  {:else if meter !== null}
    <div class="meter" role="img" aria-label={`${Math.round(meter)}%`}><span style="width:{Math.max(0, Math.min(100, meter))}%;background:{meterColor}"></span></div>
  {/if}
  {#if href}<a {href} class="kpi-link" aria-label={`Open ${label}`} style="position:absolute;inset:0;border-radius:inherit;z-index:1"></a>{/if}
</div>

<style>
  .kpi:has(.kpi-link:hover) { border-color: var(--border-strong); box-shadow: var(--shadow); }
</style>
