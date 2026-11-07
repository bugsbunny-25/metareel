// Who is signed in to the admin console, and whether the server requires it.
// ready: the first /auth/session answer arrived. loginRequired: the server
// runs with AUTH_REQUIRE_ADMIN_LOGIN, so the console needs an admin session.
import { getAuthSession, login, logout, setUnauthorizedHandler } from './api.js'

export const auth = $state({ ready: false, loginRequired: false, user: null, error: '' })

export async function loadAuth() {
  try {
    const s = await getAuthSession()
    auth.loginRequired = s.admin_login_required
    auth.user = s.user
    auth.error = ''
  } catch (e) {
    auth.error = e.message
  } finally {
    auth.ready = true
  }
}

export async function signIn(username, password) {
  const res = await login(username, password)
  auth.user = res.user
  return res.user
}

export async function signOut() {
  try {
    await logout()
  } finally {
    auth.user = null
  }
}

// A 401 from the admin API means the session ended (expired, revoked, or
// the user was disabled): show the sign-in screen again.
setUnauthorizedHandler(() => {
  auth.user = null
})
