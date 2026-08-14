<script setup lang="ts">
import type { DropdownMenuItem, TableColumn } from '@nuxt/ui'
import type { Vps, VpsKeyStatus, VpsSortBy, VpsSortDir } from '~/types/vps'

useHead({ title: 'VPS · Stacker' })

const { items, sshKeys, pending, error, load, testKey } = useVps()

// Client-side only: the stacker server is a local daemon, so there is nothing
// for the SSR pass to talk to.
onMounted(() => load())

const search = ref('')
const authFilter = ref<VpsKeyStatus | 'all'>('all')
const keyFilter = ref<string | 'all'>('all')
const sortBy = ref<VpsSortBy>('name')
const sortDir = ref<VpsSortDir>('asc')
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

  return items.value.filter((vps) => {
    if (authFilter.value !== 'all' && vps.keyStatus !== authFilter.value) return false
    if (keyFilter.value !== 'all' && vps.sshKeyId !== keyFilter.value) return false

    if (!term) return true

    return [vps.name, vps.ssh, String(vps.port), keyName(vps.sshKeyId) ?? '']
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
const selected = ref<Vps | null>(null)

function onCreate() {
  selected.value = null
  formOpen.value = true
}

function onEdit(vps: Vps) {
  selected.value = vps
  formOpen.value = true
}

function onDelete(vps: Vps) {
  selected.value = vps
  deleteOpen.value = true
}

const toast = useToast()

const statusMeta: Record<VpsKeyStatus, { icon: string, class: string, label: string }> = {
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

async function onTest(vps: Vps) {
  testing.value = new Set(testing.value).add(vps.id)

  try {
    const result = await testKey(vps)
    toast.add({
      title: result.ok ? 'Connection OK' : 'Connection failed',
      description: `${vps.name} — ${result.message}`,
      icon: result.ok ? 'i-lucide-circle-check' : 'i-lucide-circle-x',
      color: result.ok ? 'success' : 'error'
    })
  } finally {
    const next = new Set(testing.value)
    next.delete(vps.id)
    testing.value = next
  }
}

function rowActions(vps: Vps): DropdownMenuItem[][] {
  return [
    [
      { label: 'Edit', icon: 'i-lucide-pencil', onSelect: () => onEdit(vps) },
      {
        label: 'Test connection',
        icon: 'i-lucide-plug-zap',
        onSelect: () => onTest(vps)
      },
      {
        label: 'Copy SSH command',
        icon: 'i-lucide-copy',
        onSelect: () => copySshCommand(vps)
      }
    ],
    [
      { label: 'Delete', icon: 'i-lucide-trash-2', color: 'error', onSelect: () => onDelete(vps) }
    ]
  ]
}

async function copySshCommand(vps: Vps) {
  const command = `ssh -p ${vps.port} ${vps.ssh}`
  await navigator.clipboard.writeText(command)
  toast.add({ title: 'Copied', description: command, icon: 'i-lucide-clipboard-check' })
}

const columns: TableColumn<Vps>[] = [
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
  <UDashboardPanel id="vps" :ui="{ body: 'relative' }">
    <template #header>
      <UDashboardNavbar title="VPS">
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
            label="Add VPS"
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
              <UIcon name="i-lucide-server" class="size-4 text-primary" />
            </div>
            <div class="leading-tight">
              <p class="font-medium text-highlighted">{{ row.original.name }}</p>
              <p class="font-mono text-xs text-dimmed">{{ row.original.id }}</p>
            </div>
          </div>
        </template>

        <template #ssh-cell="{ row }">
          <span class="font-mono text-toned">{{ row.original.ssh }}</span>
          <span class="font-mono text-dimmed">:{{ row.original.port }}</span>
        </template>

        <template #auth-cell="{ row }">
          <div class="flex items-center gap-2">
            <UBadge
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
                aria-label="VPS actions"
              />
            </UDropdownMenu>
          </div>
        </template>

        <template #empty>
          <div class="flex flex-col items-center gap-3 py-6">
            <UIcon name="i-lucide-server-off" class="size-8 text-dimmed" />
            <p class="text-sm text-muted">
              {{ hasFilters ? 'No VPS matches these filters.' : 'No VPS registered yet.' }}
            </p>
            <UButton
              v-if="hasFilters"
              label="Reset filters"
              color="neutral"
              variant="subtle"
              @click="resetFilters"
            />
            <UButton v-else label="Add your first VPS" icon="i-lucide-plus" @click="onCreate" />
          </div>
        </template>
      </UTable>

      <div class="flex items-center justify-between gap-3 border-t border-default pt-4 mt-auto">
        <p class="text-sm text-muted">
          {{ sorted.length }} of {{ items.length }} {{ items.length === 1 ? 'server' : 'servers' }}
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

  <VpsFormModal v-model:open="formOpen" :vps="selected" />
  <VpsDeleteModal v-model:open="deleteOpen" :vps="selected" />
</template>
