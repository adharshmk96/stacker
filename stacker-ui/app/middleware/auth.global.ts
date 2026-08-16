/**
 * The one gate in front of the app.
 *
 * Three states decide where a navigation lands:
 *   - no account on the install yet  → /register (and nowhere else)
 *   - an account, but not signed in  → /login
 *   - signed in                      → the dashboard, never the auth pages
 */

const PUBLIC = ['/login', '/register', '/forgot-password', '/reset-password']

export default defineNuxtRouteMiddleware(async (to) => {
  const { user, loadStatus, restore } = useAuth()

  const isPublic = PUBLIC.includes(to.path)

  // A stored token is only a claim until /auth/me confirms it, so resolve it
  // before deciding anything.
  await restore()

  let registered: boolean
  try {
    registered = await loadStatus()
  } catch {
    // The server is unreachable. Bouncing to /login would just hide the error,
    // so let the page render and report it itself.
    return
  }

  // First run: registration is the only thing that can happen.
  if (!registered) {
    return to.path === '/register' ? undefined : navigateTo('/register')
  }

  // Registration is closed once the account exists — the API refuses it too.
  if (to.path === '/register') {
    return navigateTo(user.value ? '/dashboard/nodes' : '/login')
  }

  if (!user.value) {
    if (isPublic) return
    // Remember where they were headed so login can send them back.
    return navigateTo({ path: '/login', query: to.fullPath === '/' ? undefined : { redirect: to.fullPath } })
  }

  if (isPublic) return navigateTo('/dashboard/nodes')
})
