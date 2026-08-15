/**
 * `unknown` — never checked
 * `ok`      — the key authenticated against the host
 * `failed`  — the host refused the key (or was unreachable)
 */
export type NodeKeyStatus = 'unknown' | 'ok' | 'failed'

/**
 * `none`    — not configured into the swarm yet
 * `manager` — runs the swarm control plane
 * `worker`  — joined, runs tasks only
 */
export type SwarmRole = 'none' | 'manager' | 'worker'

/**
 * `unknown` — not probed since the server started
 * `online`  — the host answered the last probe
 * `offline` — it did not
 */
export type Reachability = 'unknown' | 'online' | 'offline'

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
  /** What the server last observed on the host — never a wish, always a reading */
  swarmRole: SwarmRole
  /** Docker's own id for the node, used to promote, demote or remove it */
  swarmNodeId: string
  /** Advertise address; only set on a manager, and the address workers join */
  swarmAddr: string
  /** Why the last swarm action or refresh failed, if it did */
  swarmError: string
  /** When the swarm fields were last read from docker */
  swarmSyncedAt?: string
  createdAt: string
  updatedAt: string
  /** Whether the host answered the last probe. Never stored — server memory only */
  reachability: Reachability
  /** Why the last probe failed, if it did */
  reachabilityMessage?: string
  /** When reachability was last determined */
  reachableCheckedAt?: string
}

export type NodePayload = Omit<
  Node,
  'id' | 'local' | 'createdAt' | 'updatedAt'
  | 'swarmRole' | 'swarmNodeId' | 'swarmAddr' | 'swarmError' | 'swarmSyncedAt'
  | 'reachability' | 'reachabilityMessage' | 'reachableCheckedAt'
>

/** Outcome of an ssh-copy-id run or a plain key check */
export interface KeyCheckResult {
  ok: boolean
  message: string
}

/** What every swarm mutation answers with: the updated node and a line about it */
export interface SwarmResult {
  node: Node
  message: string
}

/**
 * A step of the configure checklist.
 *
 * `skipped` is the normal outcome of re-running configure on a node that is
 * already set up — every step checks before it acts. `warned` did not pass but
 * was not fatal, so the run carried on.
 */
export type StepState = 'pending' | 'running' | 'done' | 'skipped' | 'failed' | 'warned'

export interface ProvisionStep {
  key: string
  title: string
  state: StepState
  /** What happened, in the step's own words — a version, a distro, or the error */
  detail: string
  startedAt?: string
  endedAt?: string
}

export type ProvisionState = 'running' | 'succeeded' | 'failed'

/** One configure run: the checklist plus its overall outcome. */
export interface ProvisionJob {
  nodeId: string
  state: ProvisionState
  steps: ProvisionStep[]
  message: string
  startedAt: string
  finishedAt?: string
}

export type NodeSortBy = 'name' | 'ssh' | 'createdAt' | 'updatedAt'
export type NodeSortDir = 'asc' | 'desc'
