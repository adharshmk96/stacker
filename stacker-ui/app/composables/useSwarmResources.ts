import type { SwarmResource, SwarmResourceKey, SwarmRow } from '~/types/swarm'

/**
 * What each docker resource list looks like and what can be done to a row.
 *
 * This is description only — the rows come from `/api/swarm/<key>` through
 * `useSwarmApi`. Column keys match the fields the server builds from docker's
 * own `{{json .}}` output, so adding a column means adding it on both sides.
 */

/** Docker refuses to remove the networks it created for itself. */
const builtinNetwork = (row: SwarmRow) =>
  ['ingress', 'bridge', 'host', 'none', 'docker_gwbridge'].includes(String(row.name))
    ? 'Docker\'s own networks cannot be removed'
    : undefined

/** Actions available on any node-scoped row, linking back to the node itself. */
const goToNode = { key: 'node', label: 'Go to node', icon: 'i-lucide-server', to: () => '/dashboard/nodes' }

const resources: SwarmResource[] = [
  {
    key: 'stacks',
    label: 'Stacks',
    icon: 'i-lucide-layers',
    singular: 'stack',
    scope: 'swarm',
    idField: 'name',
    description: 'Compose files deployed to the swarm, each owning a group of services.',
    columns: [
      { key: 'name', label: 'Name' },
      { key: 'services', label: 'Services', kind: 'mono' },
      { key: 'orchestrator', label: 'Orchestrator' }
    ],
    actions: [
      [
        { key: 'services', label: 'View services', icon: 'i-lucide-workflow', reads: true },
        { key: 'tasks', label: 'View tasks', icon: 'i-lucide-list-checks', reads: true }
      ],
      [{ key: 'remove', label: 'Remove stack', icon: 'i-lucide-trash-2', danger: true }]
    ],
    create: {
      label: 'Deploy stack',
      icon: 'i-lucide-rocket',
      title: 'Deploy a stack',
      description: 'The compose file is sent to the manager on stdin and deployed under this name.',
      fields: [
        { key: 'name', label: 'Stack name', placeholder: 'storefront', required: true },
        {
          key: 'content',
          label: 'Compose file',
          type: 'textarea',
          required: true,
          placeholder: 'services:\n  web:\n    image: nginx:alpine',
          help: 'The same YAML you would pass to `docker stack deploy -c`.'
        }
      ]
    }
  },
  {
    key: 'services',
    label: 'Services',
    icon: 'i-lucide-workflow',
    singular: 'service',
    scope: 'swarm',
    idField: 'name',
    description: 'The declared workloads — the manager keeps the requested replicas running.',
    columns: [
      { key: 'name', label: 'Name' },
      { key: 'mode', label: 'Mode', kind: 'badge' },
      { key: 'replicas', label: 'Replicas', kind: 'mono' },
      { key: 'image', label: 'Image', kind: 'mono' },
      { key: 'ports', label: 'Ports', kind: 'mono' }
    ],
    actions: [
      [
        { key: 'tasks', label: 'View tasks', icon: 'i-lucide-list-checks', reads: true },
        { key: 'logs', label: 'View logs', icon: 'i-lucide-scroll-text', reads: true },
        { key: 'inspect', label: 'Inspect', icon: 'i-lucide-file-search', reads: true }
      ],
      [
        {
          key: 'scale',
          label: 'Scale…',
          icon: 'i-lucide-scaling',
          prompt: 'scale',
          unavailable: row => row.mode === 'global'
            ? 'A global service runs one task per node and cannot be scaled'
            : undefined
        },
        { key: 'update', label: 'Force update', icon: 'i-lucide-refresh-cw' },
        { key: 'rollback', label: 'Rollback', icon: 'i-lucide-undo-2' }
      ],
      [{ key: 'remove', label: 'Remove service', icon: 'i-lucide-trash-2', danger: true }]
    ],
    create: {
      label: 'Create service',
      icon: 'i-lucide-plus',
      title: 'Create a service',
      description: 'A bare `docker service create` — anything more involved belongs in a stack.',
      fields: [
        { key: 'name', label: 'Name', placeholder: 'api', required: true },
        { key: 'image', label: 'Image', placeholder: 'nginx:alpine', required: true },
        { key: 'replicas', label: 'Replicas', type: 'number', placeholder: '1' }
      ]
    }
  },
  {
    key: 'tasks',
    label: 'Tasks',
    icon: 'i-lucide-list-checks',
    singular: 'task',
    scope: 'swarm',
    idField: 'id',
    description: 'Individual service instances, each pinned to the node running it.',
    columns: [
      { key: 'name', label: 'Name' },
      { key: 'node', label: 'Node', kind: 'node' },
      { key: 'state', label: 'State', kind: 'badge' },
      { key: 'desired', label: 'Desired', kind: 'badge' },
      { key: 'current', label: 'Current state' },
      { key: 'error', label: 'Message' }
    ],
    actions: [
      [
        { key: 'logs', label: 'View logs', icon: 'i-lucide-scroll-text', reads: true },
        { key: 'inspect', label: 'Inspect', icon: 'i-lucide-file-search', reads: true },
        goToNode
      ]
    ]
  },
  {
    key: 'containers',
    label: 'Containers',
    icon: 'i-lucide-box',
    singular: 'container',
    scope: 'node',
    idField: 'id',
    description: 'Containers on each node, swarm-managed or started by hand.',
    columns: [
      { key: 'name', label: 'Name' },
      { key: 'node', label: 'Node', kind: 'node' },
      { key: 'image', label: 'Image', kind: 'mono' },
      { key: 'state', label: 'State', kind: 'badge' },
      { key: 'status', label: 'Status' },
      { key: 'ports', label: 'Ports', kind: 'mono' }
    ],
    actions: [
      [
        { key: 'logs', label: 'View logs', icon: 'i-lucide-scroll-text', reads: true },
        { key: 'inspect', label: 'Inspect', icon: 'i-lucide-file-search', reads: true },
        goToNode
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
      [{ key: 'remove', label: 'Remove container', icon: 'i-lucide-trash-2', danger: true }]
    ]
  },
  {
    key: 'images',
    label: 'Images',
    icon: 'i-lucide-hard-drive',
    singular: 'image',
    scope: 'node',
    // Removing and inspecting go by id; pulling needs the reference, which the
    // page swaps in for that one action.
    idField: 'id',
    description: 'Images pulled onto each node, with what they cost on that node\'s disk.',
    columns: [
      { key: 'repository', label: 'Repository' },
      { key: 'tag', label: 'Tag', kind: 'mono' },
      { key: 'node', label: 'Node', kind: 'node' },
      { key: 'id', label: 'Image ID', kind: 'mono' },
      { key: 'size', label: 'Size', kind: 'mono' },
      { key: 'createdAt', label: 'Created', kind: 'date' }
    ],
    actions: [
      [
        { key: 'pull', label: 'Pull again', icon: 'i-lucide-download' },
        { key: 'inspect', label: 'Inspect', icon: 'i-lucide-file-search', reads: true },
        goToNode
      ],
      [{ key: 'remove', label: 'Remove image', icon: 'i-lucide-trash-2', danger: true }]
    ],
    create: {
      label: 'Pull image',
      icon: 'i-lucide-download',
      title: 'Pull an image',
      description: 'Pulls onto one node. Images are per node, so a service that can land anywhere wants it everywhere.',
      fields: [
        { key: 'image', label: 'Image', placeholder: 'nginx:alpine', required: true },
        { key: 'node', label: 'Node', type: 'node', required: true }
      ]
    }
  },
  {
    key: 'volumes',
    label: 'Volumes',
    icon: 'i-lucide-database',
    singular: 'volume',
    scope: 'node',
    idField: 'name',
    description: 'Named volumes live on one node — a task moving elsewhere leaves its data behind.',
    columns: [
      { key: 'name', label: 'Name' },
      { key: 'node', label: 'Node', kind: 'node' },
      { key: 'driver', label: 'Driver', kind: 'badge' },
      { key: 'mountpoint', label: 'Mountpoint', kind: 'mono' }
    ],
    actions: [
      [
        { key: 'inspect', label: 'Inspect', icon: 'i-lucide-file-search', reads: true },
        goToNode
      ],
      [{ key: 'remove', label: 'Remove volume', icon: 'i-lucide-trash-2', danger: true }]
    ],
    create: {
      label: 'Create volume',
      icon: 'i-lucide-plus',
      title: 'Create a volume',
      description: 'The volume is created on one node and only exists there.',
      fields: [
        { key: 'name', label: 'Name', placeholder: 'redis-data', required: true },
        { key: 'node', label: 'Node', type: 'node', required: true },
        { key: 'driver', label: 'Driver', placeholder: 'local', help: 'Leave empty for docker\'s default.' }
      ]
    }
  },
  {
    key: 'networks',
    label: 'Networks',
    icon: 'i-lucide-network',
    singular: 'network',
    scope: 'swarm',
    idField: 'name',
    description: 'Overlay networks span the swarm; bridge and host stay on one node.',
    columns: [
      { key: 'name', label: 'Name' },
      { key: 'driver', label: 'Driver', kind: 'badge' },
      { key: 'scope', label: 'Scope' },
      { key: 'id', label: 'ID', kind: 'mono' },
      { key: 'createdAt', label: 'Created', kind: 'date' }
    ],
    actions: [
      [{ key: 'inspect', label: 'Inspect', icon: 'i-lucide-file-search', reads: true }],
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
    create: {
      label: 'Create network',
      icon: 'i-lucide-plus',
      title: 'Create a network',
      description: 'Created attachable on the manager, so services on any node can join it.',
      fields: [
        { key: 'name', label: 'Name', placeholder: 'storefront-net', required: true },
        { key: 'driver', label: 'Driver', placeholder: 'overlay', help: 'Leave empty for overlay, which spans the swarm.' }
      ]
    }
  },
  {
    key: 'secrets',
    label: 'Secrets',
    icon: 'i-lucide-lock',
    singular: 'secret',
    scope: 'swarm',
    idField: 'name',
    description: 'Encrypted values the manager distributes to tasks. Contents are never readable back.',
    // Docker reports these two as relative times ("2 hours ago") rather than
    // timestamps, so they render as the words it chose.
    columns: [
      { key: 'name', label: 'Name' },
      { key: 'id', label: 'ID', kind: 'mono' },
      { key: 'createdAt', label: 'Created' },
      { key: 'updatedAt', label: 'Updated' }
    ],
    actions: [
      [{ key: 'inspect', label: 'Inspect metadata', icon: 'i-lucide-file-search', reads: true }],
      [{ key: 'remove', label: 'Remove secret', icon: 'i-lucide-trash-2', danger: true }]
    ],
    create: {
      label: 'Create secret',
      icon: 'i-lucide-plus',
      title: 'Create a secret',
      description: 'The value goes to the manager on stdin and can never be read back — rotating means creating the next one.',
      fields: [
        { key: 'name', label: 'Name', placeholder: 'db_password', required: true },
        { key: 'content', label: 'Value', type: 'textarea', required: true }
      ]
    }
  },
  {
    key: 'configs',
    label: 'Configs',
    icon: 'i-lucide-file-cog',
    singular: 'config',
    scope: 'swarm',
    idField: 'name',
    description: 'Non-secret files distributed to services — config, templates, certificates.',
    // Docker reports these two as relative times ("2 hours ago") rather than
    // timestamps, so they render as the words it chose.
    columns: [
      { key: 'name', label: 'Name' },
      { key: 'id', label: 'ID', kind: 'mono' },
      { key: 'createdAt', label: 'Created' },
      { key: 'updatedAt', label: 'Updated' }
    ],
    actions: [
      [
        { key: 'view', label: 'View content', icon: 'i-lucide-file-text', reads: true },
        { key: 'inspect', label: 'Inspect metadata', icon: 'i-lucide-file-search', reads: true }
      ],
      [{ key: 'remove', label: 'Remove config', icon: 'i-lucide-trash-2', danger: true }]
    ],
    create: {
      label: 'Create config',
      icon: 'i-lucide-plus',
      title: 'Create a config',
      description: 'A config is immutable, like a secret — updating one means creating the next and pointing services at it.',
      fields: [
        { key: 'name', label: 'Name', placeholder: 'nginx_conf', required: true },
        { key: 'content', label: 'Content', type: 'textarea', required: true }
      ]
    }
  }
]

const byKey = new Map(resources.map(resource => [resource.key, resource]))

export function useSwarmResources() {
  const find = (key: string): SwarmResource | undefined =>
    byKey.get(key as SwarmResourceKey)

  return { resources, find }
}
