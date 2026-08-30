/**
 * Projects, as the stacker server models them (`/api/projects`).
 *
 * The shapes mirror the Go module in `internal/modules/project` field for field,
 * so a payload sent here needs no translation on either side.
 */

import type { Deployment } from '~/types/deployment'

/** Where the compose file comes from */
export type SourceKind = 'git' | 'compose'

export type GitProvider = 'github' | 'gitlab' | 'bitbucket' | 'gitea'

export interface GitSource {
  provider: GitProvider
  /** `owner/repo` or a full clone URL — the server normalises it */
  repo: string
  branch: string
  /** Path to the compose file inside the repo */
  composePath: string
}

/** A key/value pair — used for both plain variables and secrets */
export interface EnvVar {
  key: string
  value: string
}

/**
 * `manual`   — only ever deploys when someone presses the button
 * `push`     — every push to the environment's branch
 * `tag`      — pushes of a tag matching the pattern
 * `schedule` — a cron expression
 *
 * Only `manual` runs today; the rest are stored settings the server does not act
 * on yet.
 */
export type TriggerKind = 'manual' | 'push' | 'tag' | 'schedule'

export interface DeployTrigger {
  kind: TriggerKind
  /** Tag glob for `tag`, cron expression for `schedule`; unused otherwise */
  pattern: string
}

/** How traffic reaches a service in an environment */
export interface Domain {
  id: string
  /** Hostname Traefik routes on, e.g. `shop.acme.dev` */
  host: string
  /** Compose service the host points at */
  service: string
  /** Container port Traefik forwards to */
  port: number
  /**
   * `auto` — certificate issued by Let's Encrypt
   * `none` — plain http, for internal hosts
   */
  tls: 'auto' | 'none'
  /** Send `www.host` to `host` */
  redirectWww: boolean
}

/** `rolling` starts the new task first; `recreate` stops the old one first */
export type DeployStrategy = 'rolling' | 'recreate'

/** How an environment is rolled out */
export interface DeploySettings {
  strategy: DeployStrategy
  /** Replicas per service, unless the compose file pins its own */
  replicas: number
  /** Placement constraint, e.g. `node.labels.tier==edge`; blank = anywhere */
  placement: string
  /** Seconds swarm watches a new task before accepting it */
  healthGraceSec: number
  /** Roll back automatically when the new tasks fail their health check */
  autoRollback: boolean
  /** Pull base images again even when the tag is unchanged */
  alwaysPull: boolean
}

/**
 * One deployable target of a project — `staging`, `production`, and so on. Each
 * one is a swarm stack of its own; only the source is shared.
 *
 * Secret **values** are never sent back by the server. A secret whose value
 * comes back blank keeps whatever is stored when the project is saved, so the
 * round trip does not wipe it; removing the row is how a secret is cleared.
 */
export interface Environment {
  id: string
  name: string
  /** Overrides the project branch when the source is git; blank = inherit */
  branch: string
  variables: EnvVar[]
  secrets: EnvVar[]
  domains: Domain[]
  trigger: DeployTrigger
  deploy: DeploySettings
}

export interface Project {
  id: string
  name: string
  description: string
  sourceKind: SourceKind
  git: GitSource
  /** Raw compose YAML, when `sourceKind` is `compose` */
  compose: string
  environments: Environment[]
  createdAt: string
  updatedAt: string
}

export type ProjectPayload = Omit<Project, 'id' | 'createdAt' | 'updatedAt'>

/* ---- live status ---- */

/**
 * What docker reports for an environment's stack right now. This is read from
 * the daemon on every poll, not stored: a stack can be scaled by hand, lose a
 * node or crash-loop long after a deploy that succeeded.
 *
 * `deploying` outranks the reading while a run is in flight — mid-rollout a
 * stack is short of tasks by design.
 */
export type RuntimeState = 'running' | 'degraded' | 'stopped' | 'deploying' | 'unknown'

export interface ServiceState {
  /** Compose service name, with the stack prefix removed */
  name: string
  stack: string
  image: string
  mode: string
  running: number
  desired: number
}

export interface EnvironmentStatus {
  environmentId: string
  name: string
  /** The swarm stack this environment deploys into */
  stack: string
  state: RuntimeState
  services: ServiceState[]
  /** Task totals across every service of the stack */
  running: number
  desired: number
  /** Why the state is not `running` */
  message?: string
  domains: string[]
  lastDeployment?: Deployment
}

export interface ProjectStatus {
  projectId: string
  /** The worst of its environments — a card must not claim green over a red */
  state: RuntimeState
  environments: EnvironmentStatus[]
  /** Newest run across every environment */
  lastDeployment?: Deployment
  checkedAt: string
}

/**
 * The current tail of one service's container output.
 *
 * Unlike a deployment's log this has no cursor: docker's own log command
 * always answers with the tail it has right now, so each poll replaces what
 * is shown rather than appending to it.
 */
export interface ServiceLogChunk {
  service: string
  lines: string[]
  fetchedAt: string
}
