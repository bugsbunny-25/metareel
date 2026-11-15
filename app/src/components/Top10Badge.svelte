<!-- The Top 10 badge, Netflix style: a red rounded square with TOP over 10, drawn in SVG so it
     stays crisp at any size. Shown on a title that is in a Top 10 in your country today. -->
<script>
  let { size = 26, title = 'In a Top 10 today' } = $props()
</script>

<svg class="t10" width={size} height={size} viewBox="0 0 40 40" role="img" aria-label={title}>
  <title>{title}</title>
  <rect width="40" height="40" rx="6" fill="#e50914" />
  <text x="20" y="16.5" text-anchor="middle" font-family="Arial Black, Helvetica Neue, Arial, sans-serif" font-weight="900" font-size="11.5" letter-spacing="-0.2" fill="#fff">TOP</text>
  <text x="20" y="35" text-anchor="middle" font-family="Arial Black, Helvetica Neue, Arial, sans-serif" font-weight="900" font-size="19" letter-spacing="-1" fill="#fff">10</text>
</svg>

<style>
  .t10 { display: block; flex: none; filter: drop-shadow(0 1px 3px rgb(0 0 0 / 0.45)); }
</style>
