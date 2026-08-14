/**
 * `unknown` — never checked
 * `ok`      — the key authenticated against the host
 * `failed`  — the host refused the key (or was unreachable)
 */
export type VpsKeyStatus = 'unknown' | 'ok' | 'failed'

export interface Vps {
  id: string
  name: string
  /** Connection string in `user@host` form */
  ssh: string
  port: number
  /**
   * Id of the SSH key stacker connects with. Required — the key is installed on
   * the host with `ssh-copy-id` and used for every connection afterwards.
   */
  sshKeyId: string
  keyStatus: VpsKeyStatus
  /** When keyStatus was last determined */
  keyCheckedAt?: string
  createdAt: string
  updatedAt: string
}

export type VpsPayload = Omit<Vps, 'id' | 'createdAt' | 'updatedAt'>

/** Outcome of an ssh-copy-id run or a plain key check */
export interface KeyCheckResult {
  ok: boolean
  message: string
}

export type VpsSortBy = 'name' | 'ssh' | 'createdAt' | 'updatedAt'
export type VpsSortDir = 'asc' | 'desc'
