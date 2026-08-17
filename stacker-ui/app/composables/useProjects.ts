import type { Deployment } from '~/types/deployment'
import type {
  Domain,
  Environment,
  Project,
  ProjectPayload,
  ProjectStatus,
  RuntimeState
} from '~/types/project'

/**
 * Projects, backed by the stacker server (`/api/projects`).
 *
 * Same shape as `useNodes`: one module-scope list shared by every page, written
 * through after the server confirms each mutation.
 *
 * Live status is kept in a second map rather than folded into the project rows.
 * A project is configuration and changes when someone saves it; its status is a
 * reading of docker that changes on its own every few seconds, and merging the
 * two would mean a poll could quietly overwrite an unsaved edit.
 */

export function blankDomain(): Domain {
  return {
    id: crypto.randomUUID(),
    host: '',
    service: '',
    port: 80,
    tls: 'auto',
    redirectWww: false
  }
}

export function blankEnvironment(name = 'production'): Environment {
  return {
    id: crypto.randomUUID(),
    name,
    branch: '',
    variables: [],
    secrets: [],
    domains: [],
    trigger: { kind: 'manual', pattern: '' },
    deploy: {
      strategy: 'rolling',
      replicas: 1,
      placement: '',
      healthGraceSec: 30,
      autoRollback: true,
      alwaysPull: false
    }
  }
}

export function blankProject(): ProjectPayload {
  return {
    name: '',
    description: '',
    sourceKind: 'git',
    git: { provider: 'github', repo: '', branch: 'main', composePath: 'docker-compose.yml' },
    compose: '',
    environments: [blankEnvironment()]
  }
}

/** Badge colour per runtime state, shared by the cards and the detail page. */
export const runtimeStateColor: Record<RuntimeState, 'primary' | 'success' | 'error' | 'neutral' | 'warning'> = {
  running: 'success',
  degraded: 'error',
  deploying: 'primary',
  stopped: 'neutral',
  unknown: 'warning'
}

export const runtimeStateLabel: Record<RuntimeState, string> = {
  running: 'running',
  degraded: 'degraded',
  deploying: 'deploying',
  stopped: 'stopped',
  unknown: 'unknown'
}

const items = ref<Project[]>([])
const statuses = ref<Record<string, ProjectStatus>>({})
const pending = ref(false)
const error = ref<string | null>(null)

let inflight: Promise<void> | null = null
let loaded = false

export function useProjects() {
  const api = useApi()

  async function load(force = false) {
    if (loaded && !force) return
    if (inflight) return inflight

    pending.value = true
    error.value = null

    inflight = api.get<Project[]>('/projects')
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

  const find = (id: string) => items.value.find(project => project.id === id) ?? null

  async function create(payload: ProjectPayload) {
    const project = await api.post<Project>('/projects', payload)
    items.value = [project, ...items.value]
    return project
  }

  async function update(id: string, payload: ProjectPayload) {
    const project = await api.put<Project>(`/projects/${id}`, payload)

    const index = items.value.findIndex(item => item.id === id)
    items.value = index === -1
      ? [project, ...items.value]
      : items.value.toSpliced(index, 1, project)

    return project
  }

  async function remove(id: string) {
    await api.del(`/projects/${id}`)
    items.value = items.value.filter(item => item.id !== id)

    const { [id]: _removed, ...rest } = statuses.value
    statuses.value = rest
  }

  /* ---- live status ---- */

  /**
   * Reads every project's live state in one call. Failure is deliberately
   * silent: a poll that could not run leaves the last reading on screen, which
   * is better than blanking every card because one tick was missed. A server
   * that is really gone shows up as the page's own error banner.
   */
  async function refreshStatus() {
    try {
      const list = await api.get<ProjectStatus[]>('/projects/status')
      statuses.value = Object.fromEntries((list ?? []).map(status => [status.projectId, status]))
    } catch {
      // Leave the last reading in place.
    }
  }

  /** The same for one project, which is what the detail page polls. */
  async function refreshProjectStatus(id: string) {
    try {
      const status = await api.get<ProjectStatus>(`/projects/${id}/status`)
      statuses.value = { ...statuses.value, [id]: status }
      return status
    } catch {
      return statuses.value[id] ?? null
    }
  }

  const statusOf = (id: string) => statuses.value[id] ?? null

  /* ---- deploying ---- */

  /**
   * Starts a deployment and hands back the queued run.
   *
   * It returns as soon as the run exists rather than when it finishes — a build
   * takes minutes — so the caller follows it through the run's logs and the
   * environment's status.
   */
  const deploy = (projectId: string, environmentId: string, message = '') =>
    api.post<Deployment>(`/projects/${projectId}/environments/${environmentId}/deploy`, { message })

  /** Removes an environment's stack, leaving its configuration in place. */
  const stop = (projectId: string, environmentId: string) =>
    api.post(`/projects/${projectId}/environments/${environmentId}/stop`, {})

  return {
    items,
    pending,
    error,
    load,
    find,
    create,
    update,
    remove,
    statuses,
    statusOf,
    refreshStatus,
    refreshProjectStatus,
    deploy,
    stop
  }
}
