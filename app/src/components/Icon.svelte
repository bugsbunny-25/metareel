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
    more: 'M5 10h.01M10 10h.01M15 10h.01',
    grid: 'M3.5 3.5h5v5h-5zM11.5 3.5h5v5h-5zM3.5 11.5h5v5h-5zM11.5 11.5h5v5h-5z',
    list: 'M7 5h10M7 10h10M7 15h10M3.5 5h.01M3.5 10h.01M3.5 15h.01',
    gear: 'M10 12.5a2.5 2.5 0 100-5 2.5 2.5 0 000 5zM10 2.5v2M10 15.5v2M2.5 10h2M15.5 10h2M4.7 4.7l1.4 1.4M13.9 13.9l1.4 1.4M4.7 15.3l1.4-1.4M13.9 6.1l1.4-1.4',
    chevron: 'M8 5l5 5-5 5',
    'chevron-left': 'M12 5l-5 5 5 5',
    flame: 'M10 17.5c3 0 5-2.2 5-5 0-3.5-3-5-3.5-9-2 1.5-3.5 4-3 6.5-1-.5-1.8-1.6-2-2.8C5.300 8.600 5 10.300 5 12.500c0 2.800 2 5 5 5z',
    play: 'M7 4.5l8 5.5-8 5.5z',
  };
</script>

<svg width={size} height={size} viewBox="0 0 20 20" aria-hidden="true" focusable="false">
  <path
    d={PATHS[name]}
    fill={name.endsWith('-fill') ? 'currentColor' : 'none'}
    stroke="currentColor"
    stroke-width={name === 'more' ? 3 : 1.6}
    stroke-linejoin="round"
    stroke-linecap="round"
  />
</svg>

<style>
  svg { display: block; flex: none; }
</style>
