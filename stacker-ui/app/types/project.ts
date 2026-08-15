/**
 * Placeholder types for the Application section. Nothing here is backed by the
 * stacker server yet — the pages run entirely on the in-memory mock in
 * `useProjects`, so the shapes are free to change once the API lands.
 */

/** Where the compose file comes from */
export type SourceKind = 'git' | 'compose'

export type GitProvider = 'github' | 'gitlab' | 'bitbucket' | 'gitea'

export interface GitSource {
  provider: GitProvider
  /** Clone URL or `owner/repo` — the server will normalise it */
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
  /** Hostname the proxy routes on, e.g. `shop.acme.dev` */
  host: string
  /** Compose service the host points at */
  service: string
  /** Container port the proxy forwards to */
  port: number
  /**
   * `auto`   — certificate issued by Let's Encrypt
   * `custom` — a certificate already loaded on the swarm
   * `none`   — plain http, for internal hosts
   */
  tls: 'auto' | 'custom' | 'none'
  /** Send `www.host` to `host` */
  redirectWww: boolean
}

/** `rolling` replaces tasks one at a time; `recreate` stops everything first */
export type DeployStrategy = 'rolling' | 'recreate'

/** How an environment is rolled out, once its trigger fires */
export interface DeploySettings {
  strategy: DeployStrategy
  /** Replicas per service, unless the compose file pins its own */
  replicas: number
  /** Placement constraint, e.g. `node.labels.tier==edge`; blank = anywhere */
  placement: string
  /** Seconds a task must stay healthy before the rollout continues */
  healthGraceSec: number
  /** Roll back automatically when the new tasks fail their health check */
  autoRollback: boolean
  /** Pull the image again even when the tag is unchanged */
  alwaysPull: boolean
}

/**
 * One deployable target of a project — `staging`, `production`, and so on.
 * Variables, secrets, domains and rollout settings are per environment; only
 * the source is shared.
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
