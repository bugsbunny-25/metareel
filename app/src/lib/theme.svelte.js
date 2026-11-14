// The theme is a per-device choice: dark (the default), light, or match the system. The top-bar
// button flips between light and dark; Settings also offers "match system".
const KEY = 'metareel.theme'

function initial() {
  try {
    const v = localStorage.getItem(KEY)
    return v === 'light' || v === 'system' ? v : 'dark'
  } catch {
    return 'dark'
  }
}

const media = typeof matchMedia === 'function' ? matchMedia('(prefers-color-scheme: light)') : null

/** mode: what was chosen; effective: what is showing ('light' | 'dark'). */
export const theme = $state({ mode: initial(), effective: 'dark' })

function apply() {
  theme.effective = theme.mode === 'system' ? (media?.matches ? 'light' : 'dark') : theme.mode
  if (theme.effective === 'light') document.documentElement.dataset.theme = 'light'
  else delete document.documentElement.dataset.theme
}
apply()
media?.addEventListener('change', () => theme.mode === 'system' && apply())

export function setTheme(mode) {
  theme.mode = mode
  apply()
  try {
    localStorage.setItem(KEY, mode)
  } catch {
    // storage blocked: the choice lasts for this visit only
  }
}

export const toggleTheme = () => setTheme(theme.effective === 'light' ? 'dark' : 'light')
