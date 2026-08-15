/**
 * Docker resources listed under the Swarm menu.
 *
 * The key is also the URL segment (`/dashboard/swarm/services`), so it is the
 * one identifier used by the route, the tab and the placeholder dataset.
 */
export type SwarmResourceKey =
  | 'stacks'
  | 'services'
  | 'tasks'
  | 'containers'
  | 'images'
  | 'volumes'
  | 'networks'
  | 'secrets'
  | 'configs'

/** A cell is rendered as plain text unless the column asks for something else. */
export type SwarmColumnKind = 'text' | 'mono' | 'badge' | 'date' | 'node'

export interface SwarmColumn {
  key: string
  label: string
  kind?: SwarmColumnKind
}

/** One row of a resource list — shape depends on the resource's own columns. */
export type SwarmRow = Record<string, string | number>

/**
 * A row action.
 *
 * `to` makes it a link (used to cross over into Nodes); everything else is a
 * command that will call the swarm API once it exists.
 */
export interface SwarmAction {
  key: string
  label: string
  icon: string
  /** Destructive actions are red and always sit in their own group */
  danger?: boolean
  /** Why this action cannot run on this row, if it cannot — shown as a tooltip */
  unavailable?: (row: SwarmRow) => string | undefined
  to?: (row: SwarmRow) => string
}

/**
 * Where a resource lives.
 *
 * `swarm` resources are cluster-wide and the manager answers for them.
 * `node` resources exist separately on every node, so the list is only
 * meaningful next to the node it was read from.
 */
export type SwarmScope = 'swarm' | 'node'

export interface SwarmResource {
  key: SwarmResourceKey
  /** Tab label, plural */
  label: string
  icon: string
  /** Shown under the tabs, one line on what the resource is */
  description: string
  /** Singular noun, for empty states and counts */
  singular: string
  scope: SwarmScope
  /**
   * Column holding the node a row belongs to, for node-scoped resources and for
   * swarm-scoped ones that still resolve to a single node (a task).
   */
  nodeColumn?: string
  columns: SwarmColumn[]
  /** Grouped so destructive actions stay separated in the menu */
  actions: SwarmAction[][]
  /** Placeholder rows until the API lands */
  rows: SwarmRow[]
}
