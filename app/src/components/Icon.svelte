<script>
  let { name, size = 16 } = $props();
  const PATHS = {
    bookmark: 'M6 3.5h8v13l-4-3-4 3z',
    'bookmark-fill': 'M6 3.5h8v13l-4-3-4 3z',
    check: 'M4.5 10.5l3.5 3.5 7.5-8',
    plus: 'M10 4.5v11M4.5 10h11',
    pin: 'M10 17s-5-4.6-5-8.5a5 5 0 0110 0C15 12.4 10 17 10 17z',
    flag: 'M5.5 17V3.5M5.5 4h8.5l-2 3 2 3H5.5',
  };
</script>

<svg width={size} height={size} viewBox="0 0 20 20" aria-hidden="true" focusable="false">
  <path
    d={PATHS[name]}
    fill={name.endsWith('-fill') ? 'currentColor' : 'none'}
    stroke="currentColor"
    stroke-width="1.6"
    stroke-linejoin="round"
    stroke-linecap="round"
  />
</svg>

<style>
  svg { display: block; flex: none; }
</style>
