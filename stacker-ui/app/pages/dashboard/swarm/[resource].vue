<script setup lang="ts">
import type { DropdownMenuItem, NavigationMenuItem, TableColumn } from '@nuxt/ui'
import type { SwarmAction, SwarmCreatePayload, SwarmRow } from '~/types/swarm'

/**
 * One page for every docker resource: the tab is the route, so each list is
 * linkable and the back button moves between them.
 *
 * Rows come from `/api/swarm/<resource>`, which runs the matching `docker … ls`
 * on the manager (swarm-wide resources) or on every node at once (per-node
 * ones). Row actions post back to the same module.
 */

const route = useRoute()
const router = useRouter()
const toast = useToast()

const { resources, find } = useSwarmResources()
const swarm = useSwarmApi()

const resource = computed(() => find(String(route.params.resource)))

// An unknown segment is a 404 rather than a silent empty table.
watchEffect(() => {
  if (!resource.value) {
    throw createError({ statusCode: 404, statusMessage: 'Unknown swarm resource', fatal: true })
  }
})

useHead(() => ({ title: `${resource.value?.label ?? 'Swarm'} · Swarm · Stacker` }))

const tabs = computed<NavigationMenuItem[]>(() =>
  resources.map(item => ({
    label: item.label,
    icon: item.icon,
    to: `/dashboard/swarm/${item.key}`
  })))

const search = ref('')

/**
 * Node selection lives in the URL (`?node=<id>`) so a filtered list can be
 * linked to — "the containers on edge-01" is the useful thing to send someone.
 * It is seeded from the query and written back to it, rather than read through
 * it, so opening such a link starts filtered.
 */
const nodeFilter = ref(String(route.query.node ?? 'all'))

watch(nodeFilter, (value) => {
  router.replace({ query: value === 'all' ? {} : { node: value } })
})

/**
 * Per-node resources are filtered on the server, because filtering there also
 * means not reaching out to the other nodes at all. Swarm-wide lists come from
 * the manager whole and are filtered here.
 */
const serverFiltered = computed(() => resource.value?.scope === 'node')

function reload() {
  if (!resource.value) return
  return swarm.load(resource.value.key, serverFiltered.value ? nodeFilter.value : undefined)
}

// Filters are per-resource; carrying a node selection onto a list that has no
// node column would silently hide rows. Only a move between resources clears
// them — on the first load the filter is whatever the link asked for.
watch(resource, (now, before) => {
  if (!before) return
  search.value = ''
  nodeFilter.value = 'all'
})

// Both changes mean a different list. They are watched together so switching
// tabs with a filter set — which clears the filter above — still loads once.
watch([resource, nodeFilter], () => {
  reload()
}, { immediate: true })

/** Only the resources whose rows say which node they came from can be filtered. */
const nodeColumn = computed(() =>
  resource.value?.columns.some(column => column.key === 'node') ? 'node' : undefined)

const nodeItems = computed(() => [
  { label: 'All nodes', value: 'all' },
  ...swarm.nodes.value.map(node => ({ label: node.name, value: node.id }))
])

/** Names by id, so a row filtered by id still matches the name it displays. */
const nodeName = (id: string) => swarm.nodes.value.find(node => node.id === id)?.name ?? id

// Each resource has its own columns, so the search box just looks at every cell.
const rows = computed(() => {
  const term = search.value.trim().toLowerCase()
  const column = nodeColumn.value

  return swarm.rows.value.filter((row) => {
    if (column && !serverFiltered.value && nodeFilter.value !== 'all'
      && row[column] !== nodeName(nodeFilter.value)) return false
    if (!term) return true
    return Object.values(row).some(value => String(value).toLowerCase().includes(term))
  })
})

const columns = computed<TableColumn<SwarmRow>[]>(() => [
  ...(resource.value?.columns ?? []).map(column => ({
    accessorKey: column.key,
    header: column.label
  })),
  { id: 'actions' }
])

/** Cell rendering is driven by the column's `kind`. */
const kindOf = (key: string) =>
  resource.value?.columns.find(column => column.key === key)?.kind ?? 'text'

/** States docker reports that are worth colouring rather than reading. */
const badgeColor = (value: string): 'success' | 'warning' | 'error' | 'neutral' => {
  if (['running', 'active', 'ready', 'overlay'].includes(value)) return 'success'
  if (['pending', 'starting', 'preparing', 'new', 'assigned', 'accepted'].includes(value)) return 'warning'
  if (['failed', 'exited', 'rejected', 'shutdown', 'orphaned', 'dead'].includes(value)) return 'error'
  return 'neutral'
}

/* ---- actions ---- */

/** The row's docker identifier, which is what the action endpoint takes. */
const idOf = (row: SwarmRow) => String(row[resource.value?.idField ?? 'name'] ?? '')

/** How a row is named in confirmations and toasts. */
const labelOf = (row: SwarmRow) =>
  row.name || (row.repository ? `${row.repository}:${row.tag}` : '') || row.id || ''

const running = ref(false)

/** The row and action a modal is currently asking about. */
const pendingAction = ref<{ action: SwarmAction, row: SwarmRow } | null>(null)
const confirmOpen = ref(false)
const scaleOpen = ref(false)
const createOpen = ref(false)

const output = ref({ title: '', text: '' })
const outputOpen = ref(false)

function rowActions(row: SwarmRow): DropdownMenuItem[][] {
  return (resource.value?.actions ?? []).map(group =>
    group.map((action) => {
      const reason = action.unavailable?.(row)

      return {
        label: action.label,
        icon: action.icon,
        color: action.danger ? 'error' as const : undefined,
        disabled: !!reason,
        // A disabled item still renders its title, so the reason is readable.
        ui: { item: reason ? 'cursor-not-allowed' : undefined },
        title: reason,
        to: action.to?.(row),
        onSelect: action.to ? undefined : () => start(action, row)
      }
    }))
}

/**
 * Anything that cannot be undone, and anything that needs a value, stops at a
 * modal first. Everything else runs on the click.
 */
function start(action: SwarmAction, row: SwarmRow) {
  pendingAction.value = { action, row }

  if (action.prompt === 'scale') {
    scaleOpen.value = true
    return
  }
  if (action.danger) {
    confirmOpen.value = true
    return
  }
  run(action, row)
}

async function run(action: SwarmAction, row: SwarmRow, replicas?: number) {
  if (!resource.value) return

  running.value = true

  try {
    const result = await swarm.action(resource.value.key, {
      action: action.key,
      // Pulling an image again needs the reference, not the image id — the id
      // is what is already on disk.
      id: action.key === 'pull' ? `${row.repository}:${row.tag}` : idOf(row),
      node: row.nodeId,
      replicas
    })

    confirmOpen.value = false
    scaleOpen.value = false

    if (action.reads && result.output) {
      output.value = { title: `${action.label} · ${labelOf(row)}`, text: result.output }
      outputOpen.value = true
      return
    }

    toast.add({ title: result.message, icon: action.icon, color: 'success' })
    // A mutation changes what the list should say, so it is re-read rather
    // than patched: docker is the only thing that knows the new state.
    await reload()
  } catch (error) {
    toast.add({
      title: `Could not ${action.label.replace('…', '').toLowerCase()}`,
      description: error instanceof Error ? error.message : undefined,
      icon: 'i-lucide-circle-alert',
      color: 'error'
    })
  } finally {
    running.value = false
  }
}

function onConfirm() {
  const pending = pendingAction.value
  if (pending) run(pending.action, pending.row)
}

function onScale(replicas: number) {
  const pending = pendingAction.value
  if (pending) run(pending.action, pending.row, replicas)
}

async function onCreate(payload: SwarmCreatePayload) {
  if (!resource.value) return

  running.value = true

  try {
    const result = await swarm.create(resource.value.key, payload)
    toast.add({ title: result.message, icon: resource.value.create?.icon, color: 'success' })
    createOpen.value = false
    await reload()
  } catch (error) {
    toast.add({
      title: `Could not create the ${resource.value.singular}`,
      description: error instanceof Error ? error.message : undefined,
      icon: 'i-lucide-circle-alert',
      color: 'error'
    })
  } finally {
    running.value = false
  }
}

const formatDate = (value: string) => {
  const parsed = new Date(value)
  return Number.isNaN(parsed.valueOf())
    ? value
    : parsed.toLocaleDateString(undefined, { day: 'numeric', month: 'short', year: 'numeric' })
}
</script>

<template>
  <UDashboardPanel id="swarm" :ui="{ body: 'relative' }">
    <template #header>
      <UDashboardNavbar title="Swarm">
        <template #leading>
          <UDashboardSidebarCollapse />
        </template>

        <template #right>
          <UButton
            label="Refresh"
            icon="i-lucide-refresh-cw"
            color="neutral"
            variant="subtle"
            :loading="swarm.pending.value"
            @click="reload()"
          />
          <UButton
            v-if="resource?.create"
            :label="resource.create.label"
            :icon="resource.create.icon"
            class="shadow-lg shadow-primary/20"
            @click="createOpen = true"
          />
        </template>
      </UDashboardNavbar>
    </template>

    <template #body>
      <div class="stacker-grid pointer-events-none absolute inset-0" />

      <SwarmTopology class="shrink-0" />

      <UAlert
        v-if="swarm.error.value"
        icon="i-lucide-circle-alert"
        color="error"
        variant="subtle"
        title="Could not read the swarm"
        :description="swarm.error.value"
        class="mt-4 shrink-0"
      />

      <UAlert
        v-for="failure in swarm.nodeErrors.value"
        :key="failure.node"
        icon="i-lucide-server-off"
        color="warning"
        variant="subtle"
        :title="`${failure.node} did not answer`"
        :description="failure.message"
        class="mt-4 shrink-0"
      />

      <div class="mt-4 shrink-0 border-b border-default">
        <UNavigationMenu :items="tabs" highlight class="w-full overflow-x-auto" />
      </div>

      <div v-if="resource" class="mt-4 flex shrink-0 flex-wrap items-center justify-between gap-3">
        <div class="flex items-center gap-2">
          <UBadge
            :label="resource.scope === 'node' ? 'Per node' : 'Swarm-wide'"
            :icon="resource.scope === 'node' ? 'i-lucide-server' : 'i-lucide-boxes'"
            :color="resource.scope === 'node' ? 'neutral' : 'primary'"
            variant="subtle"
            :title="resource.scope === 'node'
              ? 'This resource exists separately on every node'
              : 'The manager holds this resource for the whole swarm'"
          />
          <p class="text-sm text-muted">{{ resource.description }}</p>
        </div>

        <div class="flex shrink-0 items-center gap-2">
          <USelect
            v-if="nodeColumn && swarm.nodes.value.length"
            v-model="nodeFilter"
            :items="nodeItems"
            value-key="value"
            icon="i-lucide-server"
            class="w-40"
          />

          <UInput
            v-model="search"
            icon="i-lucide-search"
            :placeholder="`Search ${resource.label.toLowerCase()}…`"
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
        </div>
      </div>

      <UTable
        :data="rows"
        :columns="columns"
        :loading="swarm.pending.value"
        class="stacker-table mt-4 shrink-0 rounded-lg border border-default bg-default/60 backdrop-blur"
        :ui="{ tr: 'transition-colors hover:bg-elevated/40' }"
      >
        <template
          v-for="column in resource?.columns ?? []"
          :key="column.key"
          #[`${column.key}-cell`]="{ row }"
        >
          <UBadge
            v-if="kindOf(column.key) === 'node'"
            :label="String(row.original[column.key])"
            icon="i-lucide-server"
            color="neutral"
            variant="subtle"
          />
          <UBadge
            v-else-if="kindOf(column.key) === 'badge'"
            :label="String(row.original[column.key])"
            :color="badgeColor(String(row.original[column.key]))"
            variant="subtle"
          />
          <span v-else-if="kindOf(column.key) === 'mono'" class="font-mono text-toned">
            {{ row.original[column.key] }}
          </span>
          <span v-else-if="kindOf(column.key) === 'date'">
            {{ formatDate(String(row.original[column.key])) }}
          </span>
          <span v-else>{{ row.original[column.key] }}</span>
        </template>

        <template #actions-cell="{ row }">
          <div class="flex justify-end">
            <UDropdownMenu :items="rowActions(row.original)">
              <UButton
                icon="i-lucide-ellipsis-vertical"
                color="neutral"
                variant="ghost"
                :aria-label="`${resource?.singular} actions`"
              />
            </UDropdownMenu>
          </div>
        </template>

        <template #empty>
          <div class="flex flex-col items-center gap-3 py-6">
            <UIcon :name="resource?.icon ?? 'i-lucide-box'" class="size-8 text-dimmed" />
            <p class="text-sm text-muted">
              {{ search || nodeFilter !== 'all'
                ? `No ${resource?.singular} matches these filters.`
                : `No ${resource?.singular} yet.` }}
            </p>
            <UButton
              v-if="search || nodeFilter !== 'all'"
              label="Reset filters"
              color="neutral"
              variant="subtle"
              @click="search = ''; nodeFilter = 'all'"
            />
          </div>
        </template>
      </UTable>

      <p v-if="resource" class="mt-4 border-t border-default pt-4 text-sm text-muted">
        {{ rows.length }} of {{ swarm.rows.value.length }}
        {{ swarm.rows.value.length === 1 ? resource.singular : resource.label.toLowerCase() }}
      </p>

      <SwarmOutputModal v-model:open="outputOpen" :title="output.title" :output="output.text" />

      <SwarmConfirmModal
        v-if="pendingAction"
        v-model:open="confirmOpen"
        :title="pendingAction.action.label"
        :target="labelOf(pendingAction.row)"
        :description="`will be removed from ${pendingAction.row.node ?? 'the swarm'}. This cannot be undone.`"
        :loading="running"
        @confirm="onConfirm"
      />

      <SwarmScaleModal
        v-if="pendingAction"
        v-model:open="scaleOpen"
        :service="labelOf(pendingAction.row)"
        :replicas="String(pendingAction.row.replicas ?? '')"
        :loading="running"
        @confirm="onScale"
      />

      <SwarmCreateModal
        v-if="resource?.create"
        v-model:open="createOpen"
        :form="resource.create"
        :nodes="swarm.nodes.value"
        :loading="running"
        @confirm="onCreate"
      />
    </template>
  </UDashboardPanel>
</template>
