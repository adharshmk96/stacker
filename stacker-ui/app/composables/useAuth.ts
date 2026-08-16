import type {
  AuthStatus,
  ChangePasswordPayload,
  LoginPayload,
  LoginResult,
  ProfilePayload,
  RegisterPayload,
  Session,
  User
} from '~/types/auth'

/**
 * The signed-in account.
 *
 * The session token lives in localStorage so a reload does not sign the
 * operator out; it names a session row on the server, which is what makes
 * "revoke" take effect at once rather than whenever the token expires.
 *
 * State is module scope, so the layout, the guard and the settings page all
 * read the same user.
 */

const TOKEN_KEY = 'stacker.token'

const token = ref<string | null>(null)
const user = ref<User | null>(null)

/** Whether an account exists at all; `null` until /auth/status has answered. */
const registered = ref<boolean | null>(null)

/** The token is read once, lazily, rather than on every composable call. */
let hydrated = false

/** In-flight restore, so several guards running at once make one request. */
let inflight: Promise<void> | null = null

function readToken() {
  if (hydrated || !import.meta.client) return
  token.value = localStorage.getItem(TOKEN_KEY)
  hydrated = true
}

function writeToken(value: string | null) {
  token.value = value
  hydrated = true
  if (!import.meta.client) return

  if (value) localStorage.setItem(TOKEN_KEY, value)
  else localStorage.removeItem(TOKEN_KEY)
}

/**
 * Read by `useApi` on every request, and by the 401 handler to drop a token the
 * server no longer accepts. Kept outside the composable so the API layer does
 * not have to be inside a component setup to reach it.
 */
export function authToken() {
  readToken()
  return token.value
}

export function clearAuth() {
  writeToken(null)
  user.value = null
}

export function useAuth() {
  const api = useApi()

  readToken()

  const isAuthenticated = computed(() => !!token.value && !!user.value)

  /** Has the install been set up yet? Answers without a token. */
  async function loadStatus(force = false) {
    if (registered.value !== null && !force) return registered.value
    const status = await api.get<AuthStatus>('/auth/status')
    registered.value = status.registered
    return registered.value
  }

  /**
   * Resolves the stored token to a user. A token the server refuses is dropped
   * here, so a stale one never leaves the guard hanging on a signed-in state.
   */
  async function restore() {
    if (user.value || !token.value) return
    if (inflight) return inflight

    inflight = api.get<User>('/auth/me')
      .then((me) => {
        user.value = me
        registered.value = true
      })
      .catch(() => {
        clearAuth()
      })
      .finally(() => {
        inflight = null
      })

    return inflight
  }

  function accept(result: LoginResult) {
    writeToken(result.token)
    user.value = result.user
    registered.value = true
    return result.user
  }

  /** Only open while the install has no account; the server enforces that too. */
  async function register(payload: RegisterPayload) {
    return accept(await api.post<LoginResult>('/auth/register', payload))
  }

  async function login(payload: LoginPayload) {
    return accept(await api.post<LoginResult>('/auth/login', payload))
  }

  async function logout() {
    try {
      await api.post('/auth/logout')
    } finally {
      // Local state is cleared either way — a failed revoke must not strand the
      // operator in a session they asked to leave.
      clearAuth()
    }
  }

  async function updateProfile(payload: ProfilePayload) {
    user.value = await api.put<User>('/auth/profile', payload)
    return user.value
  }

  /** Signs every other device out, server-side. This one stays valid. */
  async function changePassword(payload: ChangePasswordPayload) {
    await api.post('/auth/change-password', payload)
  }

  async function forgotPassword(email: string) {
    await api.post('/auth/forgot-password', { email })
  }

  async function resetPassword(resetToken: string, password: string) {
    await api.post('/auth/reset-password', { token: resetToken, password })
  }

  function sessions() {
    return api.get<Session[]>('/auth/sessions')
  }

  function revokeSession(id: string) {
    return api.del(`/auth/sessions/${id}`)
  }

  /**
   * Erases the whole database — nodes, keys and this account. The install goes
   * back to first-run, so the caller is signed out and sent to /register.
   */
  async function resetAllData() {
    await api.post('/auth/reset-data')
    clearAuth()
    registered.value = false
  }

  return {
    token,
    user,
    registered,
    isAuthenticated,
    loadStatus,
    restore,
    register,
    login,
    logout,
    updateProfile,
    changePassword,
    forgotPassword,
    resetPassword,
    sessions,
    revokeSession,
    resetAllData
  }
}
