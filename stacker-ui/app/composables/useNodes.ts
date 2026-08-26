import type { KeyCheckResult, Node, NodePayload, ProvisionJob, SwarmResult } from '~/types/node'

/**
 * Node entries, backed by the stacker server (`/api/nodes`).
 *
 * Same shape as `useSshKeys`: one module-scope list shared by the page and the
 * modals, written through after the server confirms each mutation.
 */

const items = ref<Node[]>([])
const pending = ref(false)
const error = ref<string | null>(null)

let inflight: Promise<void> | null = null
let loaded = false

export interface InstallKeyArgs {
  ssh: string
  port: number
  sshKeyId: string
  /** Used once, for the ssh-copy-id login. Never stored. */
  password: string
}

export function useNodes() {
  const api = useApi()

  // The SSH Keys menu owns the key list; a Node only references a key by id.
  const { items: sshKeys, load: loadSshKeys } = useSshKeys()

  async function load(force = false) {
    if (loaded && !force) return
    if (inflight) return inflight

    pending.value = true
    error.value = null

    // The table renders key names, so the keys have to be in hand too.
    inflight = Promise.all([api.get<Node[]>('/nodes'), loadSshKeys(force)])
      .then(([list]) => {
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

  async function create(payload: NodePayload) {
    const node = await api.post<Node>('/nodes', payload)
    items.value = [node, ...items.value]
    await confirmKey(node, payload)
    return node
  }

  async function update(id: string, payload: NodePayload) {
    const node = await api.put<Node>(`/nodes/${id}`, payload)

    const index = items.value.findIndex(item => item.id === id)
    items.value = index === -1
      ? [node, ...items.value]
      : items.value.toSpliced(index, 1, node)

    await confirmKey(node, payload)
    return node
  }

  /**
   * A saved record always comes back with keyStatus `unknown` — the server only
   * ever records a status it verified itself. When the form says the key was
   * just installed, ask the server to confirm so the row shows the real state.
   * Failure here is not a save failure, so it never throws.
   */
  async function confirmKey(node: Node, payload: NodePayload) {
    if (payload.keyStatus !== 'ok') return
    try {
      await testKey(node)
    } catch {
      // Leave the row as `unknown`; the user can re-test from the row menu.
    }
  }

  async function remove(id: string) {
    await api.del(`/nodes/${id}`)
    items.value = items.value.filter(item => item.id !== id)
  }

  /**
   * Runs `ssh-copy-id` with the one-time password, then verifies key auth works.
   * Takes raw connection details rather than an id — the modal installs the key
   * while the host is still being entered, before the Node is saved.
   */
  function installKey(args: InstallKeyArgs): Promise<KeyCheckResult> {
    return api.post<KeyCheckResult>('/nodes/install-key', args)
  }

  /** Re-checks an already-installed key, no password involved. */
  async function testKey(node: Node): Promise<KeyCheckResult> {
    const result = await api.post<KeyCheckResult>(`/nodes/${node.id}/check-key`, {})

    // The server stamps keyStatus/keyCheckedAt on the row; mirror it locally.
    const index = items.value.findIndex(item => item.id === node.id)
    if (index !== -1) {
      items.value = items.value.toSpliced(index, 1, {
        ...items.value[index]!,
        keyStatus: result.ok ? 'ok' : 'failed',
        keyCheckedAt: new Date().toISOString()
      })
    }

    return result
  }

  /* ---- reachability ---- */

  /**
   * Probes every node at once and writes the readings back. Reachability is not
   * stored anywhere, so the list arrives `unknown` and only this call fills it
   * in; the page runs it on mount and on a timer.
   *
   * Failure is never fatal — an unreachable server already shows as the page's
   * error banner, and a half-refreshed status column is worse than a stale one.
   */
  async function pingAll() {
    try {
      const list = await api.post<Node[]>('/nodes/ping', {})
      items.value = list ?? []
    } catch {
      // Leave the rows as they stand.
    }
  }

  /** Re-probes one node, for the row's own refresh action. */
  async function ping(node: Node) {
    const updated = await api.post<Node>(`/nodes/${node.id}/ping`, {})

    const index = items.value.findIndex(item => item.id === updated.id)
    if (index !== -1) items.value = items.value.toSpliced(index, 1, updated)

    return updated
  }

  /* ---- swarm ---- */

  /**
   * Every swarm endpoint answers with the node as the server now sees it, so
   * they all funnel through here: run the call, write the row back, hand the
   * message to the caller for the toast.
   */
  async function swarmAction(node: Node, action: string, body?: unknown): Promise<SwarmResult> {
    const result = await api.post<SwarmResult>(`/nodes/${node.id}/swarm/${action}`, body ?? {})

    const index = items.value.findIndex(item => item.id === result.node.id)
    if (index !== -1) items.value = items.value.toSpliced(index, 1, result.node)

    return result
  }

  /**
   * Starts the configure run for a remote node: checks it, installs docker if
   * missing, then joins it to the installer-created swarm.
   *
   * It returns as soon as the run has started rather than when it finishes —
   * installing docker takes minutes — so the caller polls `provisionStatus`
   * for the checklist.
   */
  const configureSwarm = (node: Node) =>
    api.post<ProvisionJob>(`/nodes/${node.id}/swarm/configure`, {})

  /** The latest configure run for a node, checklist and all. */
  const provisionStatus = (node: Node) =>
    api.get<ProvisionJob>(`/nodes/${node.id}/swarm/configure`)

  const promoteSwarm = (node: Node) => swarmAction(node, 'promote')
  const demoteSwarm = (node: Node) => swarmAction(node, 'demote')
  const leaveSwarm = (node: Node) => swarmAction(node, 'leave')

  /** Re-reads the node's real state from docker, catching changes made outside stacker. */
  const refreshSwarm = (node: Node) => swarmAction(node, 'refresh')

  /**
   * Re-reads every configured node's swarm state and writes the rows back.
   *
   * A node's role is only what stacker last saw, so the page runs this on load:
   * a host that lost docker, or left the swarm by hand, then explains itself in
   * its own row instead of looking healthy until someone acts on it.
   *
   * Failure is never fatal — the server already records each node's own reason
   * on its row, and a sweep that could not run at all is covered by the page's
   * error banner.
   */
  async function refreshSwarmAll() {
    try {
      const list = await api.post<Node[]>('/nodes/refresh-swarm', {})
      items.value = list ?? []
    } catch {
      // Leave the rows as they stand.
    }
  }

  /** Nodes whose last swarm reading failed, for the page's summary banner. */
  const swarmProblems = computed(() => items.value.filter(item => !!item.swarmError))

  /** True once some node is a manager — nothing can join before that. */
  const hasManager = computed(() => items.value.some(item => item.swarmRole === 'manager'))

  return {
    items,
    sshKeys,
    pending,
    error,
    load,
    create,
    update,
    remove,
    installKey,
    testKey,
    pingAll,
    ping,
    hasManager,
    configureSwarm,
    provisionStatus,
    promoteSwarm,
    demoteSwarm,
    leaveSwarm,
    refreshSwarm,
    refreshSwarmAll,
    swarmProblems
  }
}
