<script>
  let { name, size = 16 } = $props();
  const PATHS = {
    bookmark: 'M6 3.5h8v13l-4-3-4 3z',
    'bookmark-fill': 'M6 3.5h8v13l-4-3-4 3z',
    check: 'M4.5 10.5l3.5 3.5 7.5-8',
    plus: 'M10 4.5v11M4.5 10h11',
    pin: 'M10 17s-5-4.6-5-8.5a5 5 0 0110 0C15 12.4 10 17 10 17z',
    flag: 'M5.5 17V3.5M5.5 4h8.5l-2 3 2 3H5.5',
    heart: 'M10 16.5s-6-3.6-6-8a3.3 3.3 0 016-1.9 3.3 3.3 0 016 1.9c0 4.4-6 8-6 8z',
    'heart-fill': 'M10 16.5s-6-3.6-6-8a3.3 3.3 0 016-1.9 3.3 3.3 0 016 1.9c0 4.4-6 8-6 8z',
    pencil: 'M4 16l1-4 8.5-8.5a1.4 1.4 0 012 2L7 14l-3 2zM12 5l3 3',
    trash: 'M4.5 6h11M8 6V4.5h4V6M6 6l.8 10h6.4L14 6',
    repeat: 'M5 9V7.5A2.5 2.5 0 017.5 5H15l-2-2M15 11v1.5A2.5 2.5 0 0112.5 15H5l2 2',
    search: 'M8.5 14a5.5 5.5 0 100-11 5.5 5.5 0 000 11zM12.5 12.5l4 4',
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
