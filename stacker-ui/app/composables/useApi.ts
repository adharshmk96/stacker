/**
 * Thin wrapper over `$fetch` for the stacker server.
 *
 * Every endpoint answers `{ data: … }` on success and `{ error: "…" }` on
 * failure, so unwrapping and error shaping happen once, here, instead of in
 * every composable. The session token is attached here for the same reason.
 */

interface Envelope<T> {
  data: T
}

export function useApi() {
  const config = useRuntimeConfig().public
  const baseURL = config.apiBase

  /**
   * A 401 means the token is gone or was revoked, whatever the caller was
   * doing. Dropping it here is what makes a revoked session bounce to /login on
   * its next request instead of showing empty panels.
   *
   * `/auth/login` is exempt: a wrong password answers 401 too, and that is the
   * form's error to show, not a sign-out.
   */
  function handle(error: any, path: string): never {
    if (error?.response?.status === 401 && !path.startsWith('/auth/login')) {
      clearAuth()
    }
    // FetchError carries the server's JSON body on `.data`; surface the
    // message the API wrote rather than a bare "500".
    throw new Error(error?.data?.error ?? error?.message ?? 'Could not reach the stacker server')
  }

  function headers() {
    const token = authToken()
    return token ? { Authorization: `Bearer ${token}` } : undefined
  }

  async function request<T>(path: string, options: Parameters<typeof $fetch>[1] = {}): Promise<T> {
    try {
      const response = await $fetch<Envelope<T>>(path, { baseURL, headers: headers(), ...options })
      return response.data
    } catch (error: any) {
      handle(error, path)
    }
  }

  /**
   * Absolute ws:// URL for an API path.
   *
   * The origin is the page's own — which is what makes it work behind Traefik
   * without knowing anything about it. Development is the exception: the nitro
   * dev proxy does not forward websocket upgrades, so `apiWsOrigin` points
   * straight at the stacker server there.
   *
   * The session token cannot be attached here: a browser sets no headers on a
   * websocket, so callers pass it as a subprotocol instead (see
   * `useNodeTerminal`).
   */
  function wsUrl(path: string, query: Record<string, string | number> = {}) {
    const url = new URL(`${baseURL}${path}`, config.apiWsOrigin || window.location.origin)
    url.protocol = url.protocol === 'https:' ? 'wss:' : 'ws:'
    for (const [key, value] of Object.entries(query)) url.searchParams.set(key, String(value))
    return url.toString()
  }

  return {
    get: <T>(path: string) => request<T>(path),
    post: <T>(path: string, body?: unknown) => request<T>(path, { method: 'POST', body }),
    put: <T>(path: string, body?: unknown) => request<T>(path, { method: 'PUT', body }),
    wsUrl,
    /** DELETE answers 204 with no body, so there is nothing to unwrap. */
    del: async (path: string) => {
      try {
        await $fetch(path, { baseURL, method: 'DELETE', headers: headers() })
      } catch (error: any) {
        handle(error, path)
      }
    }
  }
}
