/**
 * Docker resources listed under the Swarm menu.
 *
 * The key is also the URL segment (`/dashboard/swarm/services`) and the API
 * segment (`/api/swarm/services`), so it is the one identifier shared by the
 * route, the tab and the server.
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

/**
 * One row of a resource list.
 *
 * Every value is a string: the server hands through what docker's own
 * `{{json .}}` prints, which is all strings. Node-scoped rows also carry
 * `node`/`nodeId` saying which host they were read from.
 */
export type SwarmRow = Record<string, string>

/**
 * A row action.
 *
 * `to` makes it a link (used to cross over into Nodes); everything else posts
 * its `key` to `/api/swarm/<resource>/action`, where the server decides which
 * docker command that is.
 */
export interface SwarmAction {
  key: string
  label: string
  icon: string
  /** Destructive actions are red, sit in their own group and ask first */
  danger?: boolean
  /** Reads something back — the result opens in the output modal */
  reads?: boolean
  /** Opens the scale prompt instead of running straight away */
  prompt?: 'scale'
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

/** The field of a row that names it in confirmations, toasts and modals. */
export type SwarmRowLabel = (row: SwarmRow) => string

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
  /** The row field docker identifies this resource by, sent as the action's id */
  idField: string
  columns: SwarmColumn[]
  /** Grouped so destructive actions stay separated in the menu */
  actions: SwarmAction[][]
  /** The one create action, if the resource has one */
  create?: SwarmCreateForm
}

/** A field of a create form. */
export interface SwarmCreateField {
  key: 'name' | 'image' | 'replicas' | 'driver' | 'content' | 'node'
  label: string
  placeholder?: string
  help?: string
  type?: 'text' | 'number' | 'textarea' | 'node'
  required?: boolean
}

export interface SwarmCreateForm {
  label: string
  icon: string
  title: string
  description: string
  fields: SwarmCreateField[]
}

/** A node as the resource lists refer to it. */
export interface SwarmNodeRef {
  id: string
  name: string
  role: 'manager' | 'worker' | 'none'
  online: boolean
}

/** One node that could not be read — shown beside the rows, not instead of them. */
export interface SwarmNodeError {
  node: string
  message: string
}

/** The answer to `GET /api/swarm/<resource>`. */
export interface SwarmListResult {
  resource: SwarmResourceKey
  scope: SwarmScope
  rows: SwarmRow[]
  nodes: SwarmNodeRef[]
  errors: SwarmNodeError[]
}

/** The answer to an action or a create. */
export interface SwarmActionResult {
  message: string
  /** Filled only by the actions whose point is what they read back */
  output?: string
}

/** The payload of an action, minus the resource which is in the path. */
export interface SwarmActionPayload {
  action: string
  id: string
  node?: string
  replicas?: number
}

/** The payload of a create. Only the fields that resource's form has are sent. */
export interface SwarmCreatePayload {
  name?: string
  node?: string
  image?: string
  replicas?: number
  driver?: string
  content?: string
}
