// Dark is the default; light is a per-browser choice.
const KEY = 'metareel.theme'

function initial() {
  try {
    return localStorage.getItem(KEY) === 'light' ? 'light' : 'dark'
  } catch {
    return 'dark'
  }
}

export const theme = $state({ mode: initial() })

function apply() {
  if (theme.mode === 'light') document.documentElement.dataset.theme = 'light'
  else delete document.documentElement.dataset.theme
}
apply()

export function toggleTheme() {
  theme.mode = theme.mode === 'light' ? 'dark' : 'light'
  apply()
  try {
    localStorage.setItem(KEY, theme.mode)
  } catch {
    // storage blocked: the choice lasts for this visit only
  }
}
