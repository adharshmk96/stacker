import type { KeyCheckResult, Vps, VpsPayload } from '~/types/vps'

/**
 * VPS entries, backed by the stacker server (`/api/vps`).
 *
 * Same shape as `useSshKeys`: one module-scope list shared by the page and the
 * modals, written through after the server confirms each mutation.
 */

const items = ref<Vps[]>([])
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

export function useVps() {
  const api = useApi()

  // The SSH Keys menu owns the key list; a VPS only references a key by id.
  const { items: sshKeys, load: loadSshKeys } = useSshKeys()

  async function load(force = false) {
    if (loaded && !force) return
    if (inflight) return inflight

    pending.value = true
    error.value = null

    // The table renders key names, so the keys have to be in hand too.
    inflight = Promise.all([api.get<Vps[]>('/vps'), loadSshKeys(force)])
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

  async function create(payload: VpsPayload) {
    const vps = await api.post<Vps>('/vps', payload)
    items.value = [vps, ...items.value]
    await confirmKey(vps, payload)
    return vps
  }

  async function update(id: string, payload: VpsPayload) {
    const vps = await api.put<Vps>(`/vps/${id}`, payload)

    const index = items.value.findIndex(item => item.id === id)
    items.value = index === -1
      ? [vps, ...items.value]
      : items.value.toSpliced(index, 1, vps)

    await confirmKey(vps, payload)
    return vps
  }

  /**
   * A saved record always comes back with keyStatus `unknown` — the server only
   * ever records a status it verified itself. When the form says the key was
   * just installed, ask the server to confirm so the row shows the real state.
   * Failure here is not a save failure, so it never throws.
   */
  async function confirmKey(vps: Vps, payload: VpsPayload) {
    if (payload.keyStatus !== 'ok') return
    try {
      await testKey(vps)
    } catch {
      // Leave the row as `unknown`; the user can re-test from the row menu.
    }
  }

  async function remove(id: string) {
    await api.del(`/vps/${id}`)
    items.value = items.value.filter(item => item.id !== id)
  }

  /**
   * Runs `ssh-copy-id` with the one-time password, then verifies key auth works.
   * Takes raw connection details rather than an id — the modal installs the key
   * while the host is still being entered, before the VPS is saved.
   */
  function installKey(args: InstallKeyArgs): Promise<KeyCheckResult> {
    return api.post<KeyCheckResult>('/vps/install-key', args)
  }

  /** Re-checks an already-installed key, no password involved. */
  async function testKey(vps: Vps): Promise<KeyCheckResult> {
    const result = await api.post<KeyCheckResult>(`/vps/${vps.id}/check-key`, {})

    // The server stamps keyStatus/keyCheckedAt on the row; mirror it locally.
    const index = items.value.findIndex(item => item.id === vps.id)
    if (index !== -1) {
      items.value = items.value.toSpliced(index, 1, {
        ...items.value[index]!,
        keyStatus: result.ok ? 'ok' : 'failed',
        keyCheckedAt: new Date().toISOString()
      })
    }

    return result
  }

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
    testKey
  }
}
