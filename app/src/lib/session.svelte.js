// Who is signed in. The app needs a user: tracking routes refuse anything else,
// and the chart routes accept the same session cookie.
import { api, setUnauthorizedHandler } from './api.js'

export const session = $state({ ready: false, user: null, error: '' })

export async function loadSession() {
  try {
    const s = await api.session()
    session.user = s.user ?? null
    session.error = ''
  } catch (err) {
    session.error = err.message
  } finally {
    session.ready = true
  }
}

export async function signIn(username, password) {
  const res = await api.login(username, password)
  session.user = res.user ?? null
  return session.user
}

export async function signOut() {
  try {
    await api.logout()
  } finally {
    session.user = null
  }
}

// A 401 from a data route means the session ended; show the sign-in screen.
setUnauthorizedHandler(() => {
  session.user = null
})
