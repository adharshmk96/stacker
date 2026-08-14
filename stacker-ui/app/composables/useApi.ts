/**
 * Thin wrapper over `$fetch` for the stacker server.
 *
 * Every endpoint answers `{ data: … }` on success and `{ error: "…" }` on
 * failure, so unwrapping and error shaping happen once, here, instead of in
 * every composable.
 */

interface Envelope<T> {
  data: T
}

export function useApi() {
  const baseURL = useRuntimeConfig().public.apiBase

  async function request<T>(path: string, options: Parameters<typeof $fetch>[1] = {}): Promise<T> {
    try {
      const response = await $fetch<Envelope<T>>(path, { baseURL, ...options })
      return response.data
    } catch (error: any) {
      // FetchError carries the server's JSON body on `.data`; surface the
      // message the API wrote rather than a bare "500".
      throw new Error(error?.data?.error ?? error?.message ?? 'Could not reach the stacker server')
    }
  }

  return {
    get: <T>(path: string) => request<T>(path),
    post: <T>(path: string, body?: unknown) => request<T>(path, { method: 'POST', body }),
    put: <T>(path: string, body?: unknown) => request<T>(path, { method: 'PUT', body }),
    /** DELETE answers 204 with no body, so there is nothing to unwrap. */
    del: async (path: string) => {
      try {
        await $fetch(path, { baseURL, method: 'DELETE' })
      } catch (error: any) {
        throw new Error(error?.data?.error ?? error?.message ?? 'Could not reach the stacker server')
      }
    }
  }
}
