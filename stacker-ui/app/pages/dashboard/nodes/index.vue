<script setup lang="ts">
import type { DropdownMenuItem, TableColumn } from '@nuxt/ui'
import type { Node, NodeKeyStatus, NodeSortBy, NodeSortDir } from '~/types/node'

useHead({ title: 'Nodes · Stacker' })

const { items, sshKeys, pending, error, load, testKey } = useNodes()

// Client-side only: the stacker server is a local daemon, so there is nothing
// for the SSR pass to talk to.
onMounted(() => load())

const search = ref('')
const authFilter = ref<NodeKeyStatus | 'all'>('all')
const keyFilter = ref<string | 'all'>('all')
const sortBy = ref<NodeSortBy>('name')
const sortDir = ref<NodeSortDir>('asc')
const page = ref(1)
const pageSize = 10

const authItems = [
  { label: 'Any status', value: 'all' },
  { label: 'Connected', value: 'ok' },
  { label: 'Failing', value: 'failed' },
  { label: 'Not installed', value: 'unknown' }
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
    if (authFilter.value !== 'all' && node.keyStatus !== authFilter.value) return false
    if (keyFilter.value !== 'all' && node.sshKeyId !== keyFilter.value) return false

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
watch([search, authFilter, keyFilter, sorted], () => {
  if (page.value > pageCount.value) page.value = pageCount.value
})

const hasFilters = computed(() =>
  !!search.value || authFilter.value !== 'all' || keyFilter.value !== 'all')

function resetFilters() {
  search.value = ''
  authFilter.value = 'all'
  keyFilter.value = 'all'
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

/** Ids currently being re-checked, so each row can show its own spinner */
const testing = ref(new Set<string>())

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

// The local node has no ssh connection to test or copy, and the server refuses
// to delete it — so it only gets the rename.
function rowActions(node: Node): DropdownMenuItem[][] {
  if (node.local) {
    return [
      [{ label: 'Rename', icon: 'i-lucide-pencil', onSelect: () => onEdit(node) }]
    ]
  }

  return [
    [
      { label: 'Edit', icon: 'i-lucide-pencil', onSelect: () => onEdit(node) },
      {
        label: 'Test connection',
        icon: 'i-lucide-plug-zap',
        onSelect: () => onTest(node)
      },
      {
        label: 'Copy SSH command',
        icon: 'i-lucide-copy',
        onSelect: () => copySshCommand(node)
      }
    ],
    [
      { label: 'Delete', icon: 'i-lucide-trash-2', color: 'error', onSelect: () => onDelete(node) }
    ]
  ]
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

          <USelect v-model="authFilter" :items="authItems" value-key="value" class="w-36" />
          <USelect
            v-model="keyFilter"
            :items="keyItems"
            value-key="value"
            icon="i-lucide-key-round"
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

      <UTable
        :data="paginated"
        :columns="columns"
        :loading="pending"
        class="stacker-table shrink-0 rounded-lg border border-default bg-default/60 backdrop-blur"
        :ui="{ tr: 'transition-colors hover:bg-elevated/40' }"
      >
        <template #name-cell="{ row }">
          <div class="flex items-center gap-3">
            <div class="flex size-8 items-center justify-center rounded-md bg-elevated ring-1 ring-default">
              <UIcon
                :name="row.original.local ? 'i-lucide-monitor' : 'i-lucide-server'"
                class="size-4 text-primary"
              />
            </div>
            <div class="leading-tight">
              <p class="flex items-center gap-2 font-medium text-highlighted">
                {{ row.original.name }}
                <UBadge v-if="row.original.local" label="This machine" color="neutral" variant="subtle" size="sm" />
              </p>
              <p class="font-mono text-xs text-dimmed">{{ row.original.id }}</p>
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

        <template #updatedAt-cell="{ row }">
          {{ formatDate(row.original.updatedAt) }}
        </template>

        <template #actions-cell="{ row }">
          <div class="flex justify-end">
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
</template>
