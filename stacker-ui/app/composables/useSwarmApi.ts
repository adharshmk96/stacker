import type {
  SwarmActionPayload,
  SwarmActionResult,
  SwarmCreatePayload,
  SwarmListResult,
  SwarmNodeError,
  SwarmNodeRef,
  SwarmResourceKey,
  SwarmRow
} from '~/types/swarm'

/**
 * Live docker resource lists, backed by the stacker server (`/api/swarm`).
 *
 * Unlike `useNodes`, the state is per caller rather than module-scoped: there
 * is one Swarm page, it holds one resource at a time, and nothing is stored on
 * the server to keep in sync. Switching tabs simply loads the next list.
 */
export function useSwarmApi() {
  const api = useApi()

  const rows = ref<SwarmRow[]>([])
  const nodes = ref<SwarmNodeRef[]>([])
  /** Nodes that could not be read — shown beside the rows, not instead of them. */
  const nodeErrors = ref<SwarmNodeError[]>([])
  /** The whole list failing: no manager yet, or the server is down. */
  const error = ref<string | null>(null)
  const pending = ref(false)
  const loaded = ref(false)

  // Tab switches and refreshes can overlap, and a slow earlier list must not
  // land on top of the one the user is looking at now.
  let latest = 0

  async function load(resource: SwarmResourceKey, node?: string) {
    const request = ++latest
    pending.value = true
    error.value = null

    const query = node && node !== 'all' ? `?node=${encodeURIComponent(node)}` : ''

    try {
      const result = await api.get<SwarmListResult>(`/swarm/${resource}${query}`)
      if (request !== latest) return

      rows.value = result.rows ?? []
      nodes.value = result.nodes ?? []
      nodeErrors.value = result.errors ?? []
      loaded.value = true
    } catch (err: any) {
      if (request !== latest) return

      rows.value = []
      nodeErrors.value = []
      error.value = err.message
    } finally {
      if (request === latest) pending.value = false
    }
  }

  /** Runs one row action. The server decides which docker command that is. */
  const action = (resource: SwarmResourceKey, payload: SwarmActionPayload) =>
    api.post<SwarmActionResult>(`/swarm/${resource}/action`, payload)

  /** Runs the resource's create action. */
  const create = (resource: SwarmResourceKey, payload: SwarmCreatePayload) =>
    api.post<SwarmActionResult>(`/swarm/${resource}`, payload)

  return { rows, nodes, nodeErrors, error, pending, loaded, load, action, create }
}
