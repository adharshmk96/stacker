import type { Deployment, DeploymentStatus, LogChunk } from '~/types/deployment'
import { isLive } from '~/types/deployment'

/**
 * Deployments, backed by the stacker server (`/api/deployments`).
 *
 * The list is polled rather than pushed. A run's own progress is in its log,
 * which is read with a cursor, so following one costs a short response per tick
 * instead of the whole log every time.
 */

/** Badge colour per status, shared by the list and the project pages. */
export const deploymentStatusColor: Record<DeploymentStatus, 'primary' | 'success' | 'error' | 'neutral' | 'warning'> = {
  queued: 'neutral',
  running: 'primary',
  succeeded: 'success',
  failed: 'error',
  cancelled: 'warning'
}

const items = ref<Deployment[]>([])
const pending = ref(false)
const error = ref<string | null>(null)

let inflight: Promise<void> | null = null
let loaded = false

export function useDeployments() {
  const api = useApi()

  async function load(force = false) {
    if (loaded && !force) return
    if (inflight) return inflight

    pending.value = true
    error.value = null

    inflight = api.get<Deployment[]>('/deployments')
      .then((list) => {
        items.value = list ?? []
        loaded = true
      })
      .catch((err: Error) => {
        error.value = err.message
      })
      .finally(() => {
        pending.value = false
        inflight = null
      })

    return inflight
  }

  /**
   * Re-reads the list for the poll. Unlike `load` it never sets `pending` and
   * never surfaces its failure: a refresh that fails must not replace a table
   * the user is reading with a spinner or an error.
   */
  async function refresh() {
    try {
      items.value = await api.get<Deployment[]>('/deployments') ?? []
      loaded = true
    } catch {
      // Leave the list as it stands.
    }
  }

  /** True while some run is still moving — what the pollers key off. */
  const hasLive = computed(() => items.value.some(item => isLive(item.status)))

  const forProject = (projectId: string) =>
    computed(() => items.value.filter(item => item.projectId === projectId))

  /** Reads the lines after a cursor. See `LogChunk`. */
  const logs = (id: string, after = 0) =>
    api.get<LogChunk>(`/deployments/${id}/logs?after=${after}`)

  async function cancel(id: string) {
    await api.post(`/deployments/${id}/cancel`, {})
    await refresh()
  }

  /** Records a run the caller just started, so the list moves without a poll. */
  function track(deployment: Deployment) {
    const index = items.value.findIndex(item => item.id === deployment.id)
    items.value = index === -1
      ? [deployment, ...items.value]
      : items.value.toSpliced(index, 1, deployment)
  }

  return { items, pending, error, load, refresh, hasLive, forProject, logs, cancel, track }
}
