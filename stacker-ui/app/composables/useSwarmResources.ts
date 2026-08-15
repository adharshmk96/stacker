import type { SwarmResource, SwarmResourceKey, SwarmRow } from '~/types/swarm'

/**
 * Docker resource lists for the Swarm menu.
 *
 * Placeholder data only — the shape mirrors what `docker <resource> ls` returns,
 * so swapping each `rows` array for a `/api/swarm/<key>` call is the whole of
 * the follow-up work. Columns and actions stay here either way.
 */

/** Node names used across the placeholder rows, so the lists agree with each other. */
const NODES = ['local', 'edge-01', 'edge-02'] as const

/** Docker refuses to remove its own built-in networks. */
const builtinNetwork = (row: SwarmRow) =>
  ['ingress', 'bridge', 'host', 'none'].includes(String(row.name))
    ? 'Built-in networks cannot be removed'
    : undefined

const resources: SwarmResource[] = [
  {
    key: 'stacks',
    label: 'Stacks',
    icon: 'i-lucide-layers',
    singular: 'stack',
    scope: 'swarm',
    description: 'Compose files deployed to the swarm, each owning a group of services.',
    columns: [
      { key: 'name', label: 'Name' },
      { key: 'services', label: 'Services', kind: 'mono' },
      { key: 'nodes', label: 'Nodes', kind: 'mono' },
      { key: 'orchestrator', label: 'Orchestrator' },
      { key: 'updatedAt', label: 'Updated', kind: 'date' }
    ],
    actions: [
      [
        { key: 'services', label: 'View services', icon: 'i-lucide-workflow' },
        { key: 'redeploy', label: 'Redeploy', icon: 'i-lucide-rocket' },
        { key: 'compose', label: 'Copy compose file', icon: 'i-lucide-copy' }
      ],
      [{ key: 'remove', label: 'Remove stack', icon: 'i-lucide-trash-2', danger: true }]
    ],
    rows: [
      { name: 'monitoring', services: 4, nodes: 'all 3', orchestrator: 'Swarm', updatedAt: '2026-08-11T09:12:00Z' },
      { name: 'storefront', services: 6, nodes: 'local, edge-01, edge-02', orchestrator: 'Swarm', updatedAt: '2026-08-14T17:40:00Z' },
      { name: 'internal-tools', services: 2, nodes: 'local', orchestrator: 'Swarm', updatedAt: '2026-07-29T08:05:00Z' }
    ]
  },
  {
    key: 'services',
    label: 'Services',
    icon: 'i-lucide-workflow',
    singular: 'service',
    scope: 'swarm',
    description: 'The declared workloads — the manager keeps the requested replicas running.',
    columns: [
      { key: 'name', label: 'Name' },
      { key: 'mode', label: 'Mode', kind: 'badge' },
      { key: 'replicas', label: 'Replicas', kind: 'mono' },
      { key: 'nodes', label: 'Running on', kind: 'mono' },
      { key: 'image', label: 'Image', kind: 'mono' },
      { key: 'ports', label: 'Ports', kind: 'mono' }
    ],
    actions: [
      [
        { key: 'tasks', label: 'View tasks', icon: 'i-lucide-list-checks' },
        { key: 'logs', label: 'View logs', icon: 'i-lucide-scroll-text' },
        { key: 'inspect', label: 'Inspect', icon: 'i-lucide-file-search' }
      ],
      [
        {
          key: 'scale',
          label: 'Scale…',
          icon: 'i-lucide-scaling',
          unavailable: row => row.mode === 'global'
            ? 'A global service runs one task per node and cannot be scaled'
            : undefined
        },
        { key: 'update', label: 'Force update', icon: 'i-lucide-refresh-cw' },
        { key: 'rollback', label: 'Rollback', icon: 'i-lucide-undo-2' }
      ],
      [{ key: 'remove', label: 'Remove service', icon: 'i-lucide-trash-2', danger: true }]
    ],
    rows: [
      { name: 'storefront_web', mode: 'replicated', replicas: '3/3', nodes: 'local, edge-01, edge-02', image: 'ghcr.io/acme/web:1.8.2', ports: '*:80->8080/tcp' },
      { name: 'storefront_api', mode: 'replicated', replicas: '2/3', nodes: 'edge-01, edge-02', image: 'ghcr.io/acme/api:2.4.0', ports: '*:8081->8081/tcp' },
      { name: 'storefront_redis', mode: 'replicated', replicas: '1/1', nodes: 'local', image: 'redis:7-alpine', ports: '—' },
      { name: 'monitoring_node-exporter', mode: 'global', replicas: '3/3', nodes: 'all 3', image: 'prom/node-exporter:v1.8.1', ports: '—' },
      { name: 'monitoring_grafana', mode: 'replicated', replicas: '1/1', nodes: 'local', image: 'grafana/grafana:11.1.0', ports: '*:3000->3000/tcp' }
    ]
  },
  {
    key: 'tasks',
    label: 'Tasks',
    icon: 'i-lucide-list-checks',
    singular: 'task',
    scope: 'swarm',
    nodeColumn: 'node',
    description: 'Individual service instances, each pinned to the node running it.',
    columns: [
      { key: 'id', label: 'ID', kind: 'mono' },
      { key: 'name', label: 'Name' },
      { key: 'node', label: 'Node', kind: 'node' },
      { key: 'state', label: 'State', kind: 'badge' },
      { key: 'error', label: 'Message' },
      { key: 'updatedAt', label: 'Updated', kind: 'date' }
    ],
    actions: [
      [
        { key: 'logs', label: 'View logs', icon: 'i-lucide-scroll-text' },
        { key: 'inspect', label: 'Inspect', icon: 'i-lucide-file-search' },
        { key: 'service', label: 'Go to service', icon: 'i-lucide-workflow' },
        { key: 'node', label: 'Go to node', icon: 'i-lucide-server', to: () => '/dashboard/nodes' }
      ],
      [
        {
          key: 'shutdown',
          label: 'Shut down task',
          icon: 'i-lucide-power',
          danger: true,
          // A finished task has nothing left to stop; the service replaces it.
          unavailable: row => ['failed', 'shutdown', 'complete'].includes(String(row.state))
            ? 'This task has already stopped'
            : undefined
        }
      ]
    ],
    rows: [
      { id: 'q7fk2m9x1a', name: 'storefront_web.1', node: 'local', state: 'running', error: '—', updatedAt: '2026-08-14T17:41:00Z' },
      { id: 'b3nd8p0v5c', name: 'storefront_web.2', node: 'edge-01', state: 'running', error: '—', updatedAt: '2026-08-14T17:41:00Z' },
      { id: 'z1cq4r7t2e', name: 'storefront_api.3', node: 'edge-02', state: 'pending', error: 'no suitable node (insufficient memory)', updatedAt: '2026-08-15T06:02:00Z' },
      { id: 'w9xs5h6y3d', name: 'storefront_api.2', node: 'edge-01', state: 'failed', error: 'task: non-zero exit (1)', updatedAt: '2026-08-15T05:58:00Z' }
    ]
  },
  {
    key: 'containers',
    label: 'Containers',
    icon: 'i-lucide-box',
    singular: 'container',
    scope: 'node',
    nodeColumn: 'node',
    description: 'Containers on each node, swarm-managed or started by hand.',
    columns: [
      { key: 'id', label: 'ID', kind: 'mono' },
      { key: 'name', label: 'Name' },
      { key: 'node', label: 'Node', kind: 'node' },
      { key: 'image', label: 'Image', kind: 'mono' },
      { key: 'state', label: 'State', kind: 'badge' },
      { key: 'status', label: 'Status' }
    ],
    actions: [
      [
        { key: 'logs', label: 'View logs', icon: 'i-lucide-scroll-text' },
        { key: 'exec', label: 'Open shell', icon: 'i-lucide-terminal',
          unavailable: row => row.state === 'running' ? undefined : 'The container is not running' },
        { key: 'stats', label: 'Stats', icon: 'i-lucide-activity' },
        { key: 'inspect', label: 'Inspect', icon: 'i-lucide-file-search' },
        { key: 'node', label: 'Go to node', icon: 'i-lucide-server', to: () => '/dashboard/nodes' }
      ],
      [
        { key: 'restart', label: 'Restart', icon: 'i-lucide-rotate-cw' },
        {
          key: 'stop',
          label: 'Stop',
          icon: 'i-lucide-square',
          unavailable: row => row.state === 'running' ? undefined : 'The container is already stopped'
        },
        {
          key: 'start',
          label: 'Start',
          icon: 'i-lucide-play',
          unavailable: row => row.state === 'running' ? 'The container is already running' : undefined
        }
      ],
      [
        {
          key: 'remove',
          label: 'Remove container',
          icon: 'i-lucide-trash-2',
          danger: true,
          unavailable: row => row.state === 'running' ? 'Stop the container first' : undefined
        }
      ]
    ],
    rows: [
      { id: '4f1a9c23bd7e', name: 'storefront_web.1.q7fk2m9x1a', node: 'local', image: 'ghcr.io/acme/web:1.8.2', state: 'running', status: 'Up 6 hours' },
      { id: 'ac02e7719b31', name: 'storefront_redis.1.k2ld9', node: 'local', image: 'redis:7-alpine', state: 'running', status: 'Up 2 days' },
      { id: '77bd41e0c9aa', name: 'monitoring_grafana.1.p8ma3', node: 'edge-01', image: 'grafana/grafana:11.1.0', state: 'running', status: 'Up 2 days' },
      { id: '1e5c8ab34f60', name: 'storefront_api.2.w9xs5h6y3d', node: 'edge-01', image: 'ghcr.io/acme/api:2.4.0', state: 'exited', status: 'Exited (1) 4 minutes ago' },
      { id: '90ffab2c1d43', name: 'monitoring_node-exporter.3.n4qw8', node: 'edge-02', image: 'prom/node-exporter:v1.8.1', state: 'running', status: 'Up 5 days' }
    ]
  },
  {
    key: 'images',
    label: 'Images',
    icon: 'i-lucide-hard-drive',
    singular: 'image',
    scope: 'node',
    nodeColumn: 'node',
    description: 'Images pulled onto each node, with what they cost on that node\'s disk.',
    columns: [
      { key: 'repository', label: 'Repository' },
      { key: 'tag', label: 'Tag', kind: 'mono' },
      { key: 'node', label: 'Node', kind: 'node' },
      { key: 'id', label: 'Image ID', kind: 'mono' },
      { key: 'size', label: 'Size', kind: 'mono' },
      { key: 'inUse', label: 'In use' },
      { key: 'createdAt', label: 'Created', kind: 'date' }
    ],
    actions: [
      [
        { key: 'pull', label: 'Pull again', icon: 'i-lucide-download' },
        { key: 'pullAll', label: 'Pull on every node', icon: 'i-lucide-cloud-download' },
        { key: 'digest', label: 'Copy digest', icon: 'i-lucide-copy' },
        { key: 'inspect', label: 'Inspect', icon: 'i-lucide-file-search' },
        { key: 'node', label: 'Go to node', icon: 'i-lucide-server', to: () => '/dashboard/nodes' }
      ],
      [
        {
          key: 'remove',
          label: 'Remove image',
          icon: 'i-lucide-trash-2',
          danger: true,
          unavailable: row => row.inUse === 'yes' ? 'A container on this node still uses it' : undefined
        }
      ]
    ],
    rows: [
      { repository: 'ghcr.io/acme/web', tag: '1.8.2', node: 'local', id: 'sha256:9b1c4f', size: '182 MB', inUse: 'yes', createdAt: '2026-08-14T12:00:00Z' },
      { repository: 'ghcr.io/acme/web', tag: '1.8.1', node: 'edge-01', id: 'sha256:7f22ab', size: '181 MB', inUse: 'no', createdAt: '2026-08-02T12:00:00Z' },
      { repository: 'ghcr.io/acme/api', tag: '2.4.0', node: 'edge-01', id: 'sha256:3ad07e', size: '246 MB', inUse: 'yes', createdAt: '2026-08-13T10:30:00Z' },
      { repository: 'redis', tag: '7-alpine', node: 'local', id: 'sha256:c0f2b8', size: '41 MB', inUse: 'yes', createdAt: '2026-06-02T00:00:00Z' },
      { repository: 'grafana/grafana', tag: '11.1.0', node: 'edge-01', id: 'sha256:71ea9d', size: '412 MB', inUse: 'yes', createdAt: '2026-05-19T00:00:00Z' },
      { repository: 'prom/node-exporter', tag: 'v1.8.1', node: 'edge-02', id: 'sha256:5cc31a', size: '23 MB', inUse: 'yes', createdAt: '2026-04-11T00:00:00Z' }
    ]
  },
  {
    key: 'volumes',
    label: 'Volumes',
    icon: 'i-lucide-database',
    singular: 'volume',
    scope: 'node',
    nodeColumn: 'node',
    description: 'Named volumes live on one node — a task moving elsewhere leaves its data behind.',
    columns: [
      { key: 'name', label: 'Name' },
      { key: 'node', label: 'Node', kind: 'node' },
      { key: 'driver', label: 'Driver', kind: 'badge' },
      { key: 'mountpoint', label: 'Mountpoint', kind: 'mono' },
      { key: 'size', label: 'Size', kind: 'mono' },
      { key: 'inUse', label: 'In use' },
      { key: 'createdAt', label: 'Created', kind: 'date' }
    ],
    actions: [
      [
        { key: 'inspect', label: 'Inspect', icon: 'i-lucide-file-search' },
        { key: 'mountpoint', label: 'Copy mountpoint', icon: 'i-lucide-copy' },
        { key: 'node', label: 'Go to node', icon: 'i-lucide-server', to: () => '/dashboard/nodes' }
      ],
      [
        {
          key: 'remove',
          label: 'Remove volume',
          icon: 'i-lucide-trash-2',
          danger: true,
          unavailable: row => row.inUse === 'yes' ? 'A container on this node still mounts it' : undefined
        }
      ]
    ],
    rows: [
      { name: 'storefront_redis-data', node: 'local', driver: 'local', mountpoint: '/var/lib/docker/volumes/storefront_redis-data/_data', size: '312 MB', inUse: 'yes', createdAt: '2026-06-20T08:00:00Z' },
      { name: 'monitoring_grafana-data', node: 'edge-01', driver: 'local', mountpoint: '/var/lib/docker/volumes/monitoring_grafana-data/_data', size: '1.2 GB', inUse: 'yes', createdAt: '2026-05-19T09:15:00Z' },
      { name: 'monitoring_prom-data', node: 'edge-01', driver: 'local', mountpoint: '/var/lib/docker/volumes/monitoring_prom-data/_data', size: '8.4 GB', inUse: 'yes', createdAt: '2026-05-19T09:15:00Z' },
      { name: 'storefront_api-cache', node: 'edge-02', driver: 'local', mountpoint: '/var/lib/docker/volumes/storefront_api-cache/_data', size: '96 MB', inUse: 'no', createdAt: '2026-07-04T10:00:00Z' }
    ]
  },
  {
    key: 'networks',
    label: 'Networks',
    icon: 'i-lucide-network',
    singular: 'network',
    scope: 'swarm',
    description: 'Overlay networks span the swarm; bridge and host stay on one node.',
    columns: [
      { key: 'name', label: 'Name' },
      { key: 'driver', label: 'Driver', kind: 'badge' },
      { key: 'scope', label: 'Scope' },
      { key: 'nodes', label: 'Present on', kind: 'mono' },
      { key: 'subnet', label: 'Subnet', kind: 'mono' },
      { key: 'attachable', label: 'Attachable' }
    ],
    actions: [
      [
        { key: 'inspect', label: 'Inspect', icon: 'i-lucide-file-search' },
        { key: 'subnet', label: 'Copy subnet', icon: 'i-lucide-copy' },
        { key: 'connected', label: 'View connected services', icon: 'i-lucide-workflow' }
      ],
      [
        {
          key: 'remove',
          label: 'Remove network',
          icon: 'i-lucide-trash-2',
          danger: true,
          unavailable: builtinNetwork
        }
      ]
    ],
    rows: [
      { name: 'ingress', driver: 'overlay', scope: 'swarm', nodes: 'all 3', subnet: '10.0.0.0/24', attachable: 'no' },
      { name: 'storefront_default', driver: 'overlay', scope: 'swarm', nodes: 'all 3', subnet: '10.0.1.0/24', attachable: 'yes' },
      { name: 'monitoring_default', driver: 'overlay', scope: 'swarm', nodes: 'local, edge-01', subnet: '10.0.2.0/24', attachable: 'no' },
      { name: 'bridge', driver: 'bridge', scope: 'local', nodes: 'local', subnet: '172.17.0.0/16', attachable: 'no' }
    ]
  },
  {
    key: 'secrets',
    label: 'Secrets',
    icon: 'i-lucide-lock',
    singular: 'secret',
    scope: 'swarm',
    description: 'Encrypted values the manager distributes to tasks. Contents are never readable back.',
    columns: [
      { key: 'name', label: 'Name' },
      { key: 'id', label: 'ID', kind: 'mono' },
      { key: 'usedBy', label: 'Used by', kind: 'mono' },
      { key: 'nodes', label: 'Mounted on', kind: 'mono' },
      { key: 'createdAt', label: 'Created', kind: 'date' },
      { key: 'updatedAt', label: 'Updated', kind: 'date' }
    ],
    actions: [
      [
        { key: 'inspect', label: 'Inspect metadata', icon: 'i-lucide-file-search' },
        { key: 'services', label: 'View using services', icon: 'i-lucide-workflow' },
        // Docker secrets are immutable: rotating means creating the next one and
        // updating the services onto it.
        { key: 'rotate', label: 'Rotate value…', icon: 'i-lucide-refresh-cw' }
      ],
      [
        {
          key: 'remove',
          label: 'Remove secret',
          icon: 'i-lucide-trash-2',
          danger: true,
          unavailable: row => row.usedBy === 'unused'
            ? undefined
            : 'A service still mounts this secret'
        }
      ]
    ],
    rows: [
      { name: 'storefront_db_password', id: 'x8kd02m4qn', usedBy: '2 services', nodes: 'local, edge-01', createdAt: '2026-06-20T08:00:00Z', updatedAt: '2026-08-01T11:20:00Z' },
      { name: 'storefront_stripe_key', id: 'p1la93b7vz', usedBy: '1 service', nodes: 'edge-01', createdAt: '2026-07-02T14:45:00Z', updatedAt: '2026-07-02T14:45:00Z' },
      { name: 'grafana_admin_password', id: 'r4no61c8ws', usedBy: '1 service', nodes: 'local', createdAt: '2026-05-19T09:15:00Z', updatedAt: '2026-05-19T09:15:00Z' },
      { name: 'legacy_api_token', id: 'm5tq18e3jb', usedBy: 'unused', nodes: '—', createdAt: '2026-02-10T07:30:00Z', updatedAt: '2026-02-10T07:30:00Z' }
    ]
  },
  {
    key: 'configs',
    label: 'Configs',
    icon: 'i-lucide-file-cog',
    singular: 'config',
    scope: 'swarm',
    description: 'Non-secret files distributed to services — config, templates, certificates.',
    columns: [
      { key: 'name', label: 'Name' },
      { key: 'id', label: 'ID', kind: 'mono' },
      { key: 'size', label: 'Size', kind: 'mono' },
      { key: 'usedBy', label: 'Used by', kind: 'mono' },
      { key: 'nodes', label: 'Mounted on', kind: 'mono' },
      { key: 'createdAt', label: 'Created', kind: 'date' }
    ],
    actions: [
      [
        { key: 'view', label: 'View content', icon: 'i-lucide-file-text' },
        { key: 'download', label: 'Copy content', icon: 'i-lucide-copy' },
        { key: 'services', label: 'View using services', icon: 'i-lucide-workflow' },
        // Like secrets, a config is immutable — updating creates the next one.
        { key: 'update', label: 'Update content…', icon: 'i-lucide-pencil' }
      ],
      [
        {
          key: 'remove',
          label: 'Remove config',
          icon: 'i-lucide-trash-2',
          danger: true,
          unavailable: row => row.usedBy === 'unused'
            ? undefined
            : 'A service still mounts this config'
        }
      ]
    ],
    rows: [
      { name: 'monitoring_prometheus_yml', id: 'j2vc70d9xt', size: '3.1 KB', usedBy: '1 service', nodes: 'edge-01', createdAt: '2026-05-19T09:15:00Z' },
      { name: 'storefront_nginx_conf', id: 'h6zb45f1kq', size: '1.8 KB', usedBy: '1 service', nodes: 'all 3', createdAt: '2026-06-20T08:00:00Z' },
      { name: 'storefront_nginx_conf_v1', id: 'c3wy82g5nr', size: '1.7 KB', usedBy: 'unused', nodes: '—', createdAt: '2026-05-02T13:10:00Z' }
    ]
  }
]

/**
 * The one create action each list offers, shown in the navbar.
 *
 * Tasks and containers have none: the swarm creates those itself as a
 * consequence of a service, and stacker has no reason to offer a hand-run one.
 */
const createActions: Partial<Record<SwarmResourceKey, { label: string, icon: string }>> = {
  stacks: { label: 'Deploy stack', icon: 'i-lucide-rocket' },
  services: { label: 'Create service', icon: 'i-lucide-plus' },
  images: { label: 'Pull image', icon: 'i-lucide-download' },
  volumes: { label: 'Create volume', icon: 'i-lucide-plus' },
  networks: { label: 'Create network', icon: 'i-lucide-plus' },
  secrets: { label: 'Create secret', icon: 'i-lucide-plus' },
  configs: { label: 'Create config', icon: 'i-lucide-plus' }
}

const byKey = new Map(resources.map(resource => [resource.key, resource]))

export function useSwarmResources() {
  const find = (key: string): SwarmResource | undefined =>
    byKey.get(key as SwarmResourceKey)

  const createAction = (key: SwarmResourceKey) => createActions[key]

  return { resources, find, createAction, nodeNames: NODES }
}
