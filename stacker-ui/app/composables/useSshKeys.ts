import type { SshKey, SshKeyPayload } from '~/types/sshKey'

/**
 * SSH keys, backed by the stacker server (`/api/ssh-keys`).
 *
 * The list lives in module scope so the page, the modals and the Node menu
 * (which resolves a key by id) all read the same array — mutations write
 * straight into it after the server confirms, so no caller has to refetch.
 */

const items = ref<SshKey[]>([])
const pending = ref(false)
const error = ref<string | null>(null)

/** In-flight load, so two pages mounting at once make one request. */
let inflight: Promise<void> | null = null
let loaded = false

export function useSshKeys() {
  const api = useApi()

  async function load(force = false) {
    if (loaded && !force) return
    if (inflight) return inflight

    pending.value = true
    error.value = null

    inflight = api.get<SshKey[]>('/ssh-keys')
      .then((keys) => {
        items.value = keys ?? []
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

  /** The server runs the keygen; only the public half ever comes back. */
  async function create(payload: SshKeyPayload) {
    const key = await api.post<SshKey>('/ssh-keys', payload)
    items.value = [key, ...items.value]
    return key
  }

  async function remove(id: string) {
    // The server refuses keys still referenced by a Node — let that error through.
    await api.del(`/ssh-keys/${id}`)
    items.value = items.value.filter(item => item.id !== id)
  }

  async function rotate(id: string) {
    const key = await api.post<SshKey>(`/ssh-keys/${id}/rotate`)
    items.value = items.value.map(item => item.id === id ? key : item)
    return key
  }

  return { items, pending, error, load, create, remove, rotate }
}
