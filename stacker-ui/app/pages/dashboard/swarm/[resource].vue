<script setup lang="ts">
import type { DropdownMenuItem, NavigationMenuItem, TableColumn } from '@nuxt/ui'
import type { SwarmAction, SwarmRow } from '~/types/swarm'

/**
 * One page for every docker resource: the tab is the route, so each list is
 * linkable and the back button moves between them.
 */

const route = useRoute()
const { resources, find, createAction } = useSwarmResources()

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

const create = computed(() => resource.value && createAction(resource.value.key))

const search = ref('')

const router = useRouter()

/**
 * Node selection lives in the URL (`?node=edge-01`) so a filtered list can be
 * linked to — "the containers on edge-01" is the useful thing to send someone.
 */
const nodeFilter = computed({
  get: () => String(route.query.node ?? 'all'),
  set: (value) => {
    router.replace({
      query: value === 'all' ? {} : { node: value }
    })
  }
})

// Filters are per-resource; carrying a node selection onto a list that has no
// node column would silently hide rows.
watch(resource, () => {
  search.value = ''
  if (nodeFilter.value !== 'all') nodeFilter.value = 'all'
})

/** Which node each row belongs to, for the resources that pin to one. */
const nodeColumn = computed(() => resource.value?.nodeColumn)

const nodeItems = computed(() => {
  const column = nodeColumn.value
  if (!column) return []

  const names = [...new Set((resource.value?.rows ?? []).map(row => String(row[column])))].sort()
  return [{ label: 'All nodes', value: 'all' }, ...names.map(name => ({ label: name, value: name }))]
})

// Each resource has its own columns, so the search box just looks at every cell.
const rows = computed(() => {
  const term = search.value.trim().toLowerCase()
  const column = nodeColumn.value

  return (resource.value?.rows ?? []).filter((row) => {
    if (column && nodeFilter.value !== 'all' && String(row[column]) !== nodeFilter.value) return false
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
  if (['pending', 'starting', 'preparing'].includes(value)) return 'warning'
  if (['failed', 'exited', 'rejected', 'shutdown'].includes(value)) return 'error'
  return 'neutral'
}

const toast = useToast()

/**
 * Turns the resource's action config into a menu for one row.
 *
 * Everything but the links into Nodes is inert for now — the toast names the
 * call the swarm API will make, which is also the checklist for wiring it up.
 */
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
        onSelect: action.to ? undefined : () => run(action, row)
      }
    }))
}

function run(action: SwarmAction, row: SwarmRow) {
  const name = row.name ?? row.repository ?? row.id
  toast.add({
    title: `${action.label} — not wired yet`,
    description: `${name} · this runs against the swarm API once it lands.`,
    icon: action.icon,
    color: 'neutral'
  })
}

const formatDate = (value: string) =>
  new Date(value).toLocaleDateString(undefined, { day: 'numeric', month: 'short', year: 'numeric' })
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
            disabled
            title="Live data lands with the swarm API"
          />
          <UButton
            v-if="create"
            :label="create.label"
            :icon="create.icon"
            class="shadow-lg shadow-primary/20"
            @click="toast.add({
              title: `${create.label} — not wired yet`,
              description: 'The form lands with the swarm API.',
              icon: create.icon,
              color: 'neutral'
            })"
          />
        </template>
      </UDashboardNavbar>
    </template>

    <template #body>
      <div class="stacker-grid pointer-events-none absolute inset-0" />

      <SwarmTopology class="shrink-0" />

      <UAlert
        icon="i-lucide-flask-conical"
        color="neutral"
        variant="subtle"
        title="Placeholder data"
        description="These lists are static samples and the actions are inert. Both switch to live docker data once the swarm API is in."
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
            v-if="nodeItems.length"
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
        {{ rows.length }} of {{ resource.rows.length }}
        {{ resource.rows.length === 1 ? resource.singular : resource.label.toLowerCase() }}
        <template v-if="nodeColumn && nodeFilter !== 'all'"> on {{ nodeFilter }}</template>
      </p>
    </template>
  </UDashboardPanel>
</template>
