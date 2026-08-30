<script setup lang="ts">
import type { TableColumn } from '@nuxt/ui'
import type { Deployment, DeploymentStatus } from '~/types/deployment'
import { isLive } from '~/types/deployment'

useHead({ title: 'Deployments · Stacker' })

const toast = useToast()

const { items, pending, error, load, refresh, hasLive, cancel } = useDeployments()
const { items: projects, load: loadProjects, deploy } = useProjects()

onMounted(() => Promise.all([load(true), loadProjects(true)]))

/**
 * The table follows live runs. While nothing is moving there is nothing to poll
 * for, so an idle installation stops asking — a finished run's row never changes
 * again.
 */
useLivePoll(() => {
  if (hasLive.value) return refresh()
}, 3000)

const search = ref('')
const projectFilter = ref<string | 'all'>('all')
const envFilter = ref<string | 'all'>('all')
const statusFilter = ref<DeploymentStatus | 'all'>('all')

const projectItems = computed(() => [
  { label: 'All projects', value: 'all' },
  ...projects.value.map(project => ({ label: project.name, value: project.id }))
])

const envItems = computed(() => [
  { label: 'All environments', value: 'all' },
  // Environment names are per project, so the filter lists the union of them.
  ...[...new Set(projects.value.flatMap(project => project.environments.map(env => env.name)))]
    .map(name => ({ label: name, value: name }))
])

const statusItems = [
  { label: 'Any status', value: 'all' },
  { label: 'Queued', value: 'queued' },
  { label: 'Running', value: 'running' },
  { label: 'Succeeded', value: 'succeeded' },
  { label: 'Failed', value: 'failed' },
  { label: 'Cancelled', value: 'cancelled' }
]

const filtered = computed(() => {
  const term = search.value.trim().toLowerCase()

  return items.value
    .filter((deployment) => {
      if (projectFilter.value !== 'all' && deployment.projectId !== projectFilter.value) return false
      if (envFilter.value !== 'all' && deployment.environment !== envFilter.value) return false
      if (statusFilter.value !== 'all' && deployment.status !== statusFilter.value) return false
      if (!term) return true

      return [deployment.projectName, deployment.message, deployment.revision, deployment.actor]
        .some(field => (field ?? '').toLowerCase().includes(term))
    })
    .sort((a, b) => b.startedAt.localeCompare(a.startedAt))
})

const columns: TableColumn<Deployment>[] = [
  { accessorKey: 'number', header: 'Run' },
  { accessorKey: 'projectName', header: 'Project' },
  { accessorKey: 'environment', header: 'Environment' },
  { accessorKey: 'status', header: 'Status' },
  { id: 'trigger', header: 'Trigger' },
  { accessorKey: 'startedAt', header: 'Started' },
  { id: 'duration', header: 'Duration' },
  { id: 'actions' }
]

const triggerIcon: Record<Deployment['triggeredBy'], string> = {
  manual: 'i-lucide-mouse-pointer-click',
  push: 'i-lucide-git-commit-horizontal',
  tag: 'i-lucide-tag',
  schedule: 'i-lucide-clock'
}

/* ---- logs ---- */

const logTarget = ref<Deployment | null>(null)

/** Kept in step with the poll, so the modal's badge is as live as its log. */
const logDeployment = computed(() => {
  if (!logTarget.value) return null
  return items.value.find(item => item.id === logTarget.value!.id) ?? logTarget.value
})

/* ---- actions ---- */

/**
 * Redeploy runs the same environment again with whatever configuration it has
 * now — it does not replay the old revision, which for a git source would mean
 * resurrecting a commit the branch has moved past.
 */
async function redeploy(deployment: Deployment) {
  try {
    const started = await deploy(deployment.projectId, deployment.environmentId,
      `Redeploy of #${deployment.number}`)
    await refresh()
    logTarget.value = started
    toast.add({
      title: 'Deploying',
      description: `${deployment.projectName} · ${deployment.environment}`,
      icon: 'i-lucide-rocket',
      color: 'success'
    })
  } catch (err: any) {
    toast.add({ title: 'Could not start the deploy', description: err.message, color: 'error' })
  }
}

async function cancelRun(deployment: Deployment) {
  try {
    await cancel(deployment.id)
    toast.add({ title: 'Cancelling', description: `#${deployment.number}`, color: 'warning' })
  } catch (err: any) {
    toast.add({ title: 'Could not cancel', description: err.message, color: 'error' })
  }
}

const formatTime = (value: string) =>
  new Date(value).toLocaleString(undefined, { day: 'numeric', month: 'short', hour: '2-digit', minute: '2-digit' })

const formatDuration = (seconds?: number) => {
  if (seconds === undefined) return '—'
  if (seconds < 60) return `${seconds}s`
  return `${Math.floor(seconds / 60)}m ${seconds % 60}s`
}
</script>

<template>
  <UDashboardPanel id="deployments" :ui="{ body: 'relative' }">
    <template #header>
      <UDashboardNavbar title="Deployments">
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
          <UBadge
            v-if="hasLive"
            label="live"
            color="primary"
            variant="subtle"
            class="font-mono"
          />
        </template>

        <template #right>
          <UButton
            icon="i-lucide-refresh-cw"
            color="neutral"
            variant="ghost"
            aria-label="Refresh"
            :loading="pending"
            @click="load(true)"
          />
        </template>
      </UDashboardNavbar>

      <UDashboardToolbar>
        <template #left>
          <UInput
            v-model="search"
            icon="i-lucide-search"
            placeholder="Search project, commit or message…"
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

          <USelectMenu
            v-model="projectFilter"
            :items="projectItems"
            value-key="value"
            class="w-44"
          />
          <USelectMenu
            v-model="envFilter"
            :items="envItems"
            value-key="value"
            class="w-44"
          />
          <USelectMenu
            v-model="statusFilter"
            :items="statusItems"
            value-key="value"
            class="w-40"
          />
        </template>
      </UDashboardToolbar>
    </template>

    <template #body>
      <div class="stacker-grid pointer-events-none absolute inset-0" />

      <UAlert
        v-if="error"
        :description="error"
        title="Could not load deployments"
        icon="i-lucide-triangle-alert"
        color="error"
        variant="subtle"
        class="mb-4 shrink-0"
        :actions="[{ label: 'Retry', color: 'neutral', variant: 'subtle', onClick: () => load(true) }]"
      />

      <UTable
        :data="filtered"
        :columns="columns"
        :loading="pending && !items.length"
        class="stacker-table shrink-0 rounded-lg border border-default bg-default/60 backdrop-blur"
        :ui="{ tr: 'transition-colors hover:bg-elevated/40' }"
      >
        <template #number-cell="{ row }">
          <span class="font-mono text-xs text-dimmed">#{{ row.original.number }}</span>
        </template>

        <template #projectName-cell="{ row }">
          <div class="leading-tight">
            <NuxtLink
              :to="`/dashboard/projects/${row.original.projectId}/overview`"
              class="font-medium text-highlighted hover:text-primary"
            >
              {{ row.original.projectName }}
            </NuxtLink>
            <p class="truncate font-mono text-xs text-dimmed">{{ row.original.revision }}</p>
          </div>
        </template>

        <template #environment-cell="{ row }">
          <UBadge
            :label="row.original.environment"
            color="neutral"
            variant="subtle"
            class="font-mono text-[11px]"
          />
        </template>

        <template #status-cell="{ row }">
          <div class="flex items-center gap-2">
            <UBadge
              :label="row.original.status"
              :color="deploymentStatusColor[row.original.status]"
              variant="subtle"
            />
            <UIcon
              v-if="isLive(row.original.status)"
              name="i-lucide-loader-circle"
              class="size-3.5 animate-spin text-primary"
            />
          </div>
          <p v-if="row.original.error" class="mt-1 max-w-64 truncate text-xs text-error">
            {{ row.original.error }}
          </p>
        </template>

        <template #trigger-cell="{ row }">
          <span class="flex items-center gap-1.5 text-sm text-muted">
            <UIcon :name="triggerIcon[row.original.triggeredBy]" class="size-4 text-dimmed" />
            {{ row.original.actor }}
          </span>
        </template>

        <template #startedAt-cell="{ row }">
          <span class="text-sm text-muted">{{ formatTime(row.original.startedAt) }}</span>
        </template>

        <template #duration-cell="{ row }">
          <span class="font-mono text-xs text-toned">{{ formatDuration(row.original.durationSec) }}</span>
        </template>

        <template #actions-cell="{ row }">
          <div class="flex justify-end gap-1">
            <UButton
              icon="i-lucide-scroll-text"
              color="neutral"
              variant="ghost"
              aria-label="View log"
              @click="logTarget = row.original"
            />
            <UButton
              v-if="isLive(row.original.status)"
              icon="i-lucide-x"
              color="warning"
              variant="ghost"
              aria-label="Cancel"
              @click="cancelRun(row.original)"
            />
            <UButton
              v-else
              icon="i-lucide-rotate-ccw"
              color="neutral"
              variant="ghost"
              aria-label="Redeploy"
              title="Deploy this environment again with its current configuration"
              @click="redeploy(row.original)"
            />
          </div>
        </template>

        <template #empty>
          <div class="flex flex-col items-center gap-3 py-6">
            <UIcon name="i-lucide-rocket" class="size-8 text-dimmed" />
            <p class="text-sm text-muted">
              {{ items.length ? 'No deployment matches these filters.' : 'Nothing has been deployed yet.' }}
            </p>
            <UButton
              v-if="!items.length"
              label="Go to projects"
              color="neutral"
              variant="subtle"
              to="/dashboard/projects"
            />
          </div>
        </template>
      </UTable>

      <div class="mt-auto flex items-center justify-between gap-3 border-t border-default pt-4">
        <p class="text-sm text-muted">
          {{ filtered.length }} of {{ items.length }}
          {{ items.length === 1 ? 'deployment' : 'deployments' }}
        </p>
      </div>
    </template>
  </UDashboardPanel>

  <UModal
    :open="!!logTarget"
    :title="`Deployment #${logDeployment?.number ?? ''} · ${logDeployment?.projectName ?? ''}`"
    :ui="{ content: 'max-w-4xl' }"
    @update:open="logTarget = null"
  >
    <template #body>
      <DeploymentLogViewer v-if="logDeployment" :key="logDeployment.id" :deployment="logDeployment" />
    </template>

    <template #footer>
      <div class="flex w-full justify-end gap-2">
        <UButton
          v-if="logDeployment && isLive(logDeployment.status)"
          label="Cancel run"
          color="warning"
          variant="subtle"
          icon="i-lucide-x"
          @click="cancelRun(logDeployment)"
        />
        <UButton label="Close" color="neutral" variant="ghost" @click="logTarget = null" />
      </div>
    </template>
  </UModal>
</template>
