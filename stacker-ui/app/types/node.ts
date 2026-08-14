/**
 * `unknown` — never checked
 * `ok`      — the key authenticated against the host
 * `failed`  — the host refused the key (or was unreachable)
 */
export type NodeKeyStatus = 'unknown' | 'ok' | 'failed'

export interface Node {
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
  /**
   * True for the machine stacker itself is installed on. It is seeded by the
   * server on every start, has no ssh details, and cannot be deleted.
   */
  local: boolean
  keyStatus: NodeKeyStatus
  /** When keyStatus was last determined */
  keyCheckedAt?: string
  createdAt: string
  updatedAt: string
}

export type NodePayload = Omit<Node, 'id' | 'local' | 'createdAt' | 'updatedAt'>

/** Outcome of an ssh-copy-id run or a plain key check */
export interface KeyCheckResult {
  ok: boolean
  message: string
}

export type NodeSortBy = 'name' | 'ssh' | 'createdAt' | 'updatedAt'
export type NodeSortDir = 'asc' | 'desc'
