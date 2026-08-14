/**
 * A cluster is a named group of nodes. Only the built-in `default` cluster
 * exists today — the list is static and lives entirely in the client. When
 * clusters become real, this type is what the `/api/clusters` payload should
 * match, so only the composable has to change.
 */
export interface Cluster {
  id: string
  name: string
  description?: string
}

/** The cluster every node belongs to until clusters are user-managed. */
export const DEFAULT_CLUSTER: Cluster = {
  id: 'default',
  name: 'Default cluster',
  description: 'Every node stacker knows about'
}
