<script setup lang="ts">
import type { DropdownMenuItem, TableColumn } from '@nuxt/ui'
import type { Node, NodeKeyStatus, NodeSortBy, NodeSortDir, Reachability, SwarmRole } from '~/types/node'

useHead({ title: 'Nodes · Stacker' })

const {
  items,
  sshKeys,
  pending,
  error,
  load,
  testKey,
  pingAll,
  ping,
  hasManager,
  promoteSwarm,
  demoteSwarm,
  leaveSwarm,
  refreshSwarm,
  refreshSwarmAll,
  swarmProblems
} = useNodes()

/** How often the status column re-probes every host, in ms */
const pingInterval = 30_000

/** True while the load-time swarm sweep is still running. */
const syncing = ref(false)

// Client-side only: the UI is generated as a static SPA.
onMounted(async () => {
  await load()

  // Reachability is never stored, so the rows arrive `unknown` until a sweep
  // has run; from then on the timer keeps the column honest.
  pingAll()

  // Swarm state is stored, which is exactly why it needs re-reading: the row
  // says what stacker last saw, and the host may have changed since. Doing it
  // on every load means a node that lost docker says so before anyone tries to
  // deploy to it. It is not on the timer — each node costs an ssh round trip
  // and a `docker info`, which is too much to repeat every 30 seconds.
  syncing.value = true
  refreshSwarmAll().finally(() => {
    syncing.value = false
  })

  const timer = setInterval(pingAll, pingInterval)
  onBeforeUnmount(() => clearInterval(timer))
})

/** Re-runs both sweeps, for the toolbar's refresh. */
async function onRefresh() {
  syncing.value = true
  try {
    await Promise.all([load(true), pingAll()])
    await refreshSwarmAll()
  } finally {
    syncing.value = false
  }
}

const search = ref('')
const stateFilter = ref<Reachability | 'all'>('all')
const authFilter = ref<NodeKeyStatus | 'all'>('all')
const keyFilter = ref<string | 'all'>('all')
const swarmFilter = ref<SwarmRole | 'all'>('all')
const sortBy = ref<NodeSortBy>('name')
const sortDir = ref<NodeSortDir>('asc')
const page = ref(1)
const pageSize = 10

const stateItems = [
  { label: 'Any state', value: 'all' },
  { label: 'Online', value: 'online' },
  { label: 'Offline', value: 'offline' },
  { label: 'Unknown', value: 'unknown' }
]

const authItems = [
  { label: 'Any status', value: 'all' },
  { label: 'Connected', value: 'ok' },
  { label: 'Failing', value: 'failed' },
  { label: 'Not installed', value: 'unknown' }
]

const swarmItems = [
  { label: 'Any role', value: 'all' },
  { label: 'Managers', value: 'manager' },
  { label: 'Workers', value: 'worker' },
  { label: 'Not configured', value: 'none' }
]

const keyItems = computed(() => [
  { label: 'All keys', value: 'all' },
  ...sshKeys.value.map(key => ({ label: key.name, value: key.id }))
])

const sortItems = [
  { label: 'Name', value: 'name' },
  { label: 'SSH', value: 'ssh' },
  { label: 'Created', value: 'createdAt' },
  { label: 'Updated', value: 'updatedAt' }
]

const keyName = (id?: string) => sshKeys.value.find(key => key.id === id)?.name

const filtered = computed(() => {
  const term = search.value.trim().toLowerCase()

  return items.value.filter((node) => {
    if (stateFilter.value !== 'all' && (node.reachability ?? 'unknown') !== stateFilter.value) return false
    if (authFilter.value !== 'all' && node.keyStatus !== authFilter.value) return false
    if (keyFilter.value !== 'all' && node.sshKeyId !== keyFilter.value) return false
    if (swarmFilter.value !== 'all' && node.swarmRole !== swarmFilter.value) return false

    if (!term) return true

    return [node.name, node.ssh, String(node.port), keyName(node.sshKeyId) ?? '']
      .some(field => field.toLowerCase().includes(term))
  })
})

const sorted = computed(() => {
  const dir = sortDir.value === 'asc' ? 1 : -1
  const by = sortBy.value

  return [...filtered.value].sort((a, b) => {
    const left = a[by] ?? ''
    const right = b[by] ?? ''
    return String(left).localeCompare(String(right), undefined, { numeric: true }) * dir
  })
})

const pageCount = computed(() => Math.max(1, Math.ceil(sorted.value.length / pageSize)))

const paginated = computed(() => {
  const start = (page.value - 1) * pageSize
  return sorted.value.slice(start, start + pageSize)
})

// Any narrowing of the result set can strand the user on an empty page.
watch([search, stateFilter, authFilter, keyFilter, swarmFilter, sorted], () => {
  if (page.value > pageCount.value) page.value = pageCount.value
})

const hasFilters = computed(() =>
  !!search.value || stateFilter.value !== 'all' || authFilter.value !== 'all'
  || keyFilter.value !== 'all' || swarmFilter.value !== 'all')

function resetFilters() {
  search.value = ''
  stateFilter.value = 'all'
  authFilter.value = 'all'
  keyFilter.value = 'all'
  swarmFilter.value = 'all'
}

const formOpen = ref(false)
const deleteOpen = ref(false)
const selected = ref<Node | null>(null)

function onCreate() {
  selected.value = null
  formOpen.value = true
}

function onEdit(node: Node) {
  selected.value = node
  formOpen.value = true
}

function onDelete(node: Node) {
  selected.value = node
  deleteOpen.value = true
}

const toast = useToast()

const statusMeta: Record<NodeKeyStatus, { icon: string, class: string, label: string }> = {
  ok: {
    icon: 'i-lucide-circle-check',
    class: 'text-success',
    label: 'Key authentication works'
  },
  failed: {
    icon: 'i-lucide-circle-x',
    class: 'text-error',
    label: 'Key authentication failed'
  },
  unknown: {
    icon: 'i-lucide-circle-dashed',
    class: 'text-dimmed',
    label: 'Key not installed yet'
  }
}

const stateMeta: Record<Reachability, { label: string, dot: string, text: string, hint: string }> = {
  online: {
    label: 'Online',
    dot: 'bg-success',
    text: 'text-success',
    hint: 'The host answered the last check'
  },
  offline: {
    label: 'Offline',
    dot: 'bg-error',
    text: 'text-error',
    hint: 'The host did not answer the last check'
  },
  unknown: {
    label: 'Unknown',
    dot: 'bg-muted',
    text: 'text-dimmed',
    hint: 'Not checked yet'
  }
}

// API rows should always contain a reachability value, but an empty transient
// value must still render as unknown instead of crashing the whole table row.
const stateOf = (node: Node) => stateMeta[node.reachability || 'unknown']

/** Ids currently being re-checked, so each row can show its own spinner */
const testing = ref(new Set<string>())

/** Ids with a reachability probe in flight, so each row shows its own spinner */
const pinging = ref(new Set<string>())

async function onPing(node: Node) {
  pinging.value = new Set(pinging.value).add(node.id)

  try {
    const updated = await ping(node)
    toast.add({
      title: updated.reachability === 'online' ? 'Node is online' : 'Node is offline',
      description: `${node.name} — ${updated.reachabilityMessage ?? ''}`,
      icon: updated.reachability === 'online' ? 'i-lucide-wifi' : 'i-lucide-wifi-off',
      color: updated.reachability === 'online' ? 'success' : 'error'
    })
  } catch (err) {
    toast.add({
      title: 'Could not check the node',
      description: err instanceof Error ? err.message : undefined,
      icon: 'i-lucide-circle-alert',
      color: 'error'
    })
  } finally {
    const next = new Set(pinging.value)
    next.delete(node.id)
    pinging.value = next
  }
}

async function onTest(node: Node) {
  testing.value = new Set(testing.value).add(node.id)

  try {
    const result = await testKey(node)
    toast.add({
      title: result.ok ? 'Connection OK' : 'Connection failed',
      description: `${node.name} — ${result.message}`,
      icon: result.ok ? 'i-lucide-circle-check' : 'i-lucide-circle-x',
      color: result.ok ? 'success' : 'error'
    })
  } finally {
    const next = new Set(testing.value)
    next.delete(node.id)
    testing.value = next
  }
}

/* ---- swarm ---- */

const swarmMeta: Record<SwarmRole, { label: string, icon: string, color: 'primary' | 'success' | 'neutral' }> = {
  manager: { label: 'Manager', icon: 'i-lucide-crown', color: 'primary' },
  worker: { label: 'Worker', icon: 'i-lucide-boxes', color: 'success' },
  none: { label: 'Not configured', icon: 'i-lucide-circle-dashed', color: 'neutral' }
}

const configureOpen = ref(false)

/** Ids with a swarm call in flight, so each row shows its own spinner */
const swarmBusy = ref(new Set<string>())

/**
 * Demoting or removing the last manager would leave the swarm with no control
 * plane, so the option is hidden rather than offered and then refused.
 */
const managerCount = computed(() =>
  items.value.filter(node => node.swarmRole === 'manager').length)

const promoteOpen = ref(false)

/* ---- terminal ---- */

const terminalOpen = ref(false)

/**
 * A shell on the node, in the browser. Remote nodes go over the same key-only
 * ssh everything else uses, so a node whose key was never verified is not
 * offered one — the server would refuse it anyway.
 */
function onTerminal(node: Node) {
  selected.value = node
  terminalOpen.value = true
}

function onPromote(node: Node) {
  selected.value = node
  promoteOpen.value = true
}

function onConfigure(node: Node) {
  selected.value = node
  configureOpen.value = true
}

/**
 * Runs a swarm call for one row, showing the server's own message either way —
 * docker's errors ("this node is already part of a swarm", "docker not found")
 * are the useful part.
 */
async function runSwarm(node: Node, action: (node: Node) => Promise<{ message: string }>, title: string) {
  swarmBusy.value = new Set(swarmBusy.value).add(node.id)

  try {
    const result = await action(node)
    toast.add({ title, description: result.message, icon: 'i-lucide-boxes', color: 'success' })
  } catch (err) {
    toast.add({
      title: `Could not ${title.toLowerCase()}`,
      description: err instanceof Error ? err.message : undefined,
      icon: 'i-lucide-circle-alert',
      color: 'error'
    })
  } finally {
    const next = new Set(swarmBusy.value)
    next.delete(node.id)
    swarmBusy.value = next
  }
}

// The local node has no ssh connection to test or copy. install.sh owns its
// swarm setup, so the app only offers rename and state refresh.
function rowActions(node: Node): DropdownMenuItem[][] {
  const swarm = swarmActions(node)

  if (node.local) {
    return [
      [
        { label: 'Rename', icon: 'i-lucide-pencil', onSelect: () => onEdit(node) },
        { label: 'Open terminal', icon: 'i-lucide-terminal', onSelect: () => onTerminal(node) }
      ],
      swarm
    ]
  }

  return [
    [
      { label: 'Edit', icon: 'i-lucide-pencil', onSelect: () => onEdit(node) },
      {
        label: 'Check status',
        icon: 'i-lucide-activity',
        onSelect: () => onPing(node)
      },
      {
        label: 'Test connection',
        icon: 'i-lucide-plug-zap',
        onSelect: () => onTest(node)
      },
      {
        label: 'Open terminal',
        icon: 'i-lucide-terminal',
        // The shell rides on the node's key; without a verified one there is
        // nothing to connect with.
        disabled: node.keyStatus !== 'ok',
        onSelect: () => onTerminal(node)
      },
      {
        label: 'Copy SSH command',
        icon: 'i-lucide-copy',
        onSelect: () => copySshCommand(node)
      }
    ],
    swarm,
    [
      {
        label: 'Delete',
        icon: 'i-lucide-trash-2',
        color: 'error',
        // A node still in the swarm has to leave first, or it would go on
        // running tasks with nothing left to administer it.
        disabled: node.swarmRole !== 'none',
        onSelect: () => onDelete(node)
      }
    ]
  ]
}

/** The swarm section of the row menu, which depends entirely on the current role. */
function swarmActions(node: Node): DropdownMenuItem[] {
  if (node.swarmRole === 'none') {
    if (node.local) return []
    return [
      {
        label: 'Configure',
        icon: 'i-lucide-boxes',
        disabled: !hasManager.value,
        onSelect: () => onConfigure(node)
      }
    ]
  }

  const actions: DropdownMenuItem[] = [
    {
      label: 'Refresh swarm state',
      icon: 'i-lucide-refresh-cw',
      onSelect: () => runSwarm(node, refreshSwarm, 'Refresh swarm state')
    },
    // A node's role is what stacker last saw, not a guarantee it still holds:
    // a rebuilt host loses the docker stacker installed while the row goes on
    // claiming worker. Re-running the checklist puts it right, and every step
    // re-checks before it acts, so it is safe on a node that is simply fine.
  ]

  if (!node.local) {
    actions.push({
      label: 'Reconfigure',
      icon: 'i-lucide-wrench',
      onSelect: () => onConfigure(node)
    })
  }

  if (node.swarmRole === 'worker') {
    actions.unshift({
      label: 'Promote to manager',
      icon: 'i-lucide-crown',
      onSelect: () => onPromote(node)
    })
  } else if (managerCount.value > 1) {
    actions.unshift({
      label: 'Demote to worker',
      icon: 'i-lucide-arrow-down',
      onSelect: () => runSwarm(node, demoteSwarm, 'Demote node')
    })
  }

  // Leaving is only offered when something would still administer the swarm.
  if (node.swarmRole === 'worker' || managerCount.value > 1) {
    actions.push({
      label: 'Leave swarm',
      icon: 'i-lucide-log-out',
      color: 'error',
      onSelect: () => runSwarm(node, leaveSwarm, 'Leave swarm')
    })
  }

  return actions
}

async function copySshCommand(node: Node) {
  const command = `ssh -p ${node.port} ${node.ssh}`
  await navigator.clipboard.writeText(command)
  toast.add({ title: 'Copied', description: command, icon: 'i-lucide-clipboard-check' })
}

const columns: TableColumn<Node>[] = [
  { accessorKey: 'name', header: 'Name' },
  { accessorKey: 'ssh', header: 'Connection' },
  { id: 'auth', header: 'Key' },
  { id: 'swarm', header: 'Swarm' },
  { accessorKey: 'updatedAt', header: 'Updated' },
  { id: 'actions' }
]

const formatDate = (value: string) =>
  new Date(value).toLocaleDateString(undefined, { day: 'numeric', month: 'short', year: 'numeric' })
</script>

<template>
  <UDashboardPanel id="nodes" :ui="{ body: 'relative' }">
    <template #header>
      <UDashboardNavbar title="Nodes">
        <template #leading>
          <UDashboardSidebarCollapse />
        </template>

        <template #trailing>
          <UBadge
            :label="String(items.length)"
            color="neutral"
            variant="subtle"
            class="font-mono"
          />
        </template>

        <template #right>
          <UButton
            label="Refresh"
            icon="i-lucide-refresh-cw"
            color="neutral"
            variant="subtle"
            :loading="syncing"
            title="Re-check every host over SSH and re-read its swarm state"
            @click="onRefresh"
          />
          <UButton
            label="Add node"
            icon="i-lucide-plus"
            class="shadow-lg shadow-primary/20"
            @click="onCreate"
          />
        </template>
      </UDashboardNavbar>

      <UDashboardToolbar>
        <template #left>
          <UInput
            v-model="search"
            icon="i-lucide-search"
            placeholder="Search name, host or key…"
            class="w-64"
          >
            <template v-if="search" #trailing>
              <UButton
                icon="i-lucide-x"
                color="neutral"
                variant="link"
                size="xs"
                aria-label="Clear search"
                @click="search = ''"
              />
            </template>
          </UInput>

          <USelect
            v-model="stateFilter"
            :items="stateItems"
            value-key="value"
            icon="i-lucide-activity"
            class="w-36"
          />

          <USelect v-model="authFilter" :items="authItems" value-key="value" class="w-36" />
          <USelect
            v-model="keyFilter"
            :items="keyItems"
            value-key="value"
            icon="i-lucide-key-round"
            class="w-40"
          />

          <USelect
            v-model="swarmFilter"
            :items="swarmItems"
            value-key="value"
            icon="i-lucide-boxes"
            class="w-40"
          />

          <UButton
            v-if="hasFilters"
            label="Reset"
            color="neutral"
            variant="ghost"
            icon="i-lucide-filter-x"
            @click="resetFilters"
          />
        </template>

        <template #right>
          <USelect
            v-model="sortBy"
            :items="sortItems"
            value-key="value"
            icon="i-lucide-arrow-up-down"
            class="w-40"
          />
          <UButton
            :icon="sortDir === 'asc' ? 'i-lucide-arrow-up-narrow-wide' : 'i-lucide-arrow-down-wide-narrow'"
            color="neutral"
            variant="outline"
            :aria-label="sortDir === 'asc' ? 'Sort ascending' : 'Sort descending'"
            @click="sortDir = sortDir === 'asc' ? 'desc' : 'asc'"
          />
        </template>
      </UDashboardToolbar>
    </template>

    <template #body>
      <div class="stacker-grid pointer-events-none absolute inset-0" />

      <UAlert
        v-if="error"
        :description="error"
        title="Cannot reach the stacker server"
        icon="i-lucide-plug-zap"
        color="error"
        variant="subtle"
        class="mb-4 shrink-0"
        :actions="[{ label: 'Retry', color: 'neutral', variant: 'subtle', onClick: () => load(true) }]"
      />

      <!--
        A node whose stored role no longer matches its host. The row shows the
        reason too, but a node that has quietly stopped being usable is worth
        saying once at the top — it is why a deploy would fail later.
      -->
      <UAlert
        v-for="node in swarmProblems"
        :key="node.id"
        :title="`${node.name} is not answering as a ${swarmMeta[node.swarmRole].label.toLowerCase()}`"
        :description="node.swarmError"
        icon="i-lucide-triangle-alert"
        color="warning"
        variant="subtle"
        class="mb-4 shrink-0"
        :actions="[{
          label: 'Reconfigure',
          color: 'neutral',
          variant: 'subtle',
          onClick: () => onConfigure(node)
        }]"
      />

      <UTable
        :data="paginated"
        :columns="columns"
        :loading="pending"
        class="stacker-table shrink-0 rounded-lg border border-default bg-default/60 backdrop-blur"
        :ui="{ tr: 'transition-colors hover:bg-elevated/40' }"
      >
        <template #name-cell="{ row }">
          <div class="flex items-center gap-3">
            <div class="relative flex size-8 items-center justify-center rounded-md bg-elevated ring-1 ring-default">
              <UIcon
                :name="row.original.local ? 'i-lucide-monitor' : 'i-lucide-server'"
                class="size-4 text-primary"
              />
              <span
                class="absolute -right-1 -bottom-1 size-2.5 rounded-full ring-2 ring-default"
                :class="stateOf(row.original).dot"
                :title="stateOf(row.original).hint"
              />
            </div>
            <div class="leading-tight">
              <p class="flex items-center gap-2 font-medium text-highlighted">
                {{ row.original.name }}
                <UBadge v-if="row.original.local" label="This machine" color="neutral" variant="subtle" size="sm" />
              </p>
              <p class="flex items-center gap-2 text-xs">
                <span
                  class="inline-flex items-center gap-1"
                  :class="stateOf(row.original).text"
                  :title="row.original.reachabilityMessage || stateOf(row.original).hint"
                >
                  <UIcon
                    v-if="pinging.has(row.original.id)"
                    name="i-lucide-loader-circle"
                    class="size-3 animate-spin"
                  />
                  {{ pinging.has(row.original.id) ? 'Checking…' : stateOf(row.original).label }}
                </span>
                <span class="font-mono text-dimmed">{{ row.original.id }}</span>
              </p>
            </div>
          </div>
        </template>

        <template #ssh-cell="{ row }">
          <span v-if="row.original.local" class="font-mono text-dimmed">local — no ssh</span>
          <template v-else>
            <span class="font-mono text-toned">{{ row.original.ssh }}</span>
            <span class="font-mono text-dimmed">:{{ row.original.port }}</span>
          </template>
        </template>

        <template #auth-cell="{ row }">
          <div class="flex items-center gap-2">
            <UBadge
              v-if="row.original.local"
              color="neutral"
              variant="subtle"
              icon="i-lucide-shield-check"
              label="No key needed"
            />
            <UBadge
              v-else
              color="primary"
              variant="subtle"
              icon="i-lucide-key-round"
              :label="keyName(row.original.sshKeyId) ?? 'Unknown key'"
            />
            <UIcon
              v-if="testing.has(row.original.id)"
              name="i-lucide-loader-circle"
              class="size-4 shrink-0 animate-spin text-dimmed"
            />
            <UIcon
              v-else
              :name="statusMeta[row.original.keyStatus].icon"
              class="size-4 shrink-0"
              :class="statusMeta[row.original.keyStatus].class"
              :title="statusMeta[row.original.keyStatus].label"
            />
          </div>
        </template>

        <template #swarm-cell="{ row }">
          <div class="flex items-center gap-2">
            <UIcon
              v-if="swarmBusy.has(row.original.id)"
              name="i-lucide-loader-circle"
              class="size-4 shrink-0 animate-spin text-dimmed"
            />

            <UButton
              v-else-if="row.original.swarmRole === 'none' && !row.original.local"
              label="Configure"
              icon="i-lucide-boxes"
              size="xs"
              color="neutral"
              variant="subtle"
              :disabled="!hasManager"
              :title="!hasManager
                ? 'Rerun install.sh — a swarm manager is required before a node can join'
                : undefined"
              @click="onConfigure(row.original)"
            />

            <UBadge
              v-else-if="row.original.swarmRole === 'none'"
              label="Installer required"
              icon="i-lucide-triangle-alert"
              color="warning"
              variant="subtle"
              title="Rerun install.sh to restore the local swarm manager"
            />

            <UBadge
              v-else
              :label="swarmMeta[row.original.swarmRole].label"
              :icon="swarmMeta[row.original.swarmRole].icon"
              :color="swarmMeta[row.original.swarmRole].color"
              variant="subtle"
            />

            <UIcon
              v-if="row.original.swarmError"
              name="i-lucide-triangle-alert"
              class="size-4 shrink-0 text-warning"
            />
          </div>

          <!--
            The reason is spelled out rather than left in a tooltip: it is the
            difference between "worker" meaning ready and meaning stale, and a
            tooltip is not somewhere a user thinks to look.
          -->
          <!-- Narrow enough to wrap rather than widen the table past its
               actions column. -->
          <p v-if="row.original.swarmError" class="mt-1 max-w-44 text-xs text-warning">
            {{ row.original.swarmError }}
          </p>

          <p
            v-else-if="row.original.swarmRole === 'manager' && row.original.swarmAddr"
            class="mt-1 font-mono text-xs text-dimmed"
          >
            {{ row.original.swarmAddr }}:2377
          </p>
        </template>

        <template #updatedAt-cell="{ row }">
          {{ formatDate(row.original.updatedAt) }}
        </template>

        <template #actions-cell="{ row }">
          <div class="flex justify-end gap-1">
            <UButton
              icon="i-lucide-terminal"
              color="neutral"
              variant="ghost"
              aria-label="Open terminal"
              title="Open a shell on this node"
              :disabled="!row.original.local && row.original.keyStatus !== 'ok'"
              @click="onTerminal(row.original)"
            />
            <UDropdownMenu :items="rowActions(row.original)">
              <UButton
                icon="i-lucide-ellipsis-vertical"
                color="neutral"
                variant="ghost"
                aria-label="Node actions"
              />
            </UDropdownMenu>
          </div>
        </template>

        <template #empty>
          <div class="flex flex-col items-center gap-3 py-6">
            <UIcon name="i-lucide-server-off" class="size-8 text-dimmed" />
            <p class="text-sm text-muted">
              {{ hasFilters ? 'No node matches these filters.' : 'No node registered yet.' }}
            </p>
            <UButton
              v-if="hasFilters"
              label="Reset filters"
              color="neutral"
              variant="subtle"
              @click="resetFilters"
            />
            <UButton v-else label="Add your first node" icon="i-lucide-plus" @click="onCreate" />
          </div>
        </template>
      </UTable>

      <div class="flex items-center justify-between gap-3 border-t border-default pt-4 mt-auto">
        <p class="text-sm text-muted">
          {{ sorted.length }} of {{ items.length }} {{ items.length === 1 ? 'node' : 'nodes' }}
        </p>
        <UPagination
          v-if="pageCount > 1"
          v-model:page="page"
          :items-per-page="pageSize"
          :total="sorted.length"
        />
      </div>
    </template>
  </UDashboardPanel>

  <NodeFormModal v-model:open="formOpen" :node="selected" />
  <NodeDeleteModal v-model:open="deleteOpen" :node="selected" />
  <NodeSwarmConfigureModal v-model:open="configureOpen" :node="selected" />
  <NodeSwarmPromoteModal
    v-model:open="promoteOpen"
    :node="selected"
    :manager-count="managerCount"
  />
  <NodeTerminalModal v-model:open="terminalOpen" :node="selected" />
</template>
