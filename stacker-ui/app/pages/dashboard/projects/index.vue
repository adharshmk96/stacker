<script setup lang="ts">
import type { Project } from '~/types/project'

useHead({ title: 'Projects · Stacker' })

const toast = useToast()

const { items, pending, error, load, remove, statusOf, refreshStatus } = useProjects()

const search = ref('')

onMounted(load)

// The cards show what docker is running, which changes on its own — so it is
// polled rather than read once on mount.
useLivePoll(refreshStatus, 5000)

const filtered = computed(() => {
  const term = search.value.trim().toLowerCase()
  if (!term) return items.value

  return items.value.filter(project =>
    [project.name, project.description, project.git.repo]
      .some(field => field.toLowerCase().includes(term)))
})

const sourceLabel = (project: Project) =>
  project.sourceKind === 'git' ? project.git.repo : 'compose file'

/** Pre-computed card data so the template does not re-scan status per binding. */
const cardData = computed(() => {
  const map = new Map<string, {
    status: ReturnType<typeof statusOf>
    tasks: { running: number, desired: number } | null
  }>()

  for (const project of filtered.value) {
    const status = statusOf(project.id)
    const tasks = status
      ? status.environments.reduce(
          (total, env) => ({ running: total.running + env.running, desired: total.desired + env.desired }),
          { running: 0, desired: 0 }
        )
      : null
    map.set(project.id, {
      status,
      tasks: tasks?.desired ? tasks : null
    })
  }
  return map
})

const deleteTarget = ref<Project | null>(null)
const deleting = ref(false)

async function confirmDelete() {
  const project = deleteTarget.value
  if (!project) return

  deleting.value = true
  try {
    await remove(project.id)
    toast.add({
      title: 'Project deleted',
      description: `${project.name} and everything it was running`,
      icon: 'i-lucide-trash-2',
      color: 'success'
    })
    deleteTarget.value = null
  } catch (err: any) {
    toast.add({ title: 'Could not delete', description: err.message, color: 'error' })
  } finally {
    deleting.value = false
  }
}

const formatDate = (value: string) =>
  new Date(value).toLocaleDateString(undefined, { day: 'numeric', month: 'short', year: 'numeric' })
</script>

<template>
  <UDashboardPanel id="projects" :ui="{ body: 'relative' }">
    <template #header>
      <UDashboardNavbar title="Projects">
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
            label="New project"
            icon="i-lucide-plus"
            to="/dashboard/projects/new"
            class="shadow-lg shadow-primary/20"
          />
        </template>
      </UDashboardNavbar>

      <UDashboardToolbar>
        <template #left>
          <UInput
            v-model="search"
            icon="i-lucide-search"
            placeholder="Search name or repository…"
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
        </template>
      </UDashboardToolbar>
    </template>

    <template #body>
      <div class="stacker-grid pointer-events-none absolute inset-0" />

      <UAlert
        v-if="error"
        :description="error"
        title="Could not load projects"
        icon="i-lucide-triangle-alert"
        color="error"
        variant="subtle"
        class="mb-4 shrink-0"
      />

      <div v-if="pending && !items.length" class="flex shrink-0 justify-center py-12">
        <UIcon name="i-lucide-loader-circle" class="size-6 animate-spin text-dimmed" />
      </div>

      <div v-else-if="filtered.length" class="grid shrink-0 gap-3 md:grid-cols-2 xl:grid-cols-3">
        <NuxtLink
          v-for="project in filtered"
          :key="project.id"
          :to="`/dashboard/projects/${project.id}/overview`"
          class="group flex flex-col gap-3 rounded-lg border border-default bg-default/60 p-4 backdrop-blur transition-colors hover:border-accented hover:bg-elevated/40"
        >
          <div class="flex items-start gap-3">
            <div class="flex size-9 shrink-0 items-center justify-center rounded-md bg-elevated ring-1 ring-default">
              <UIcon
                :name="project.sourceKind === 'git' ? 'i-lucide-git-branch' : 'i-lucide-file-code'"
                class="size-4.5 text-primary"
              />
            </div>
            <div class="min-w-0 flex-1 leading-tight">
              <p class="truncate font-medium text-highlighted">{{ project.name }}</p>
              <p class="truncate font-mono text-xs text-dimmed">{{ sourceLabel(project) }}</p>
            </div>

            <UBadge
              v-if="cardData.get(project.id)?.status"
              :label="runtimeStateLabel[cardData.get(project.id)!.status!.state]"
              :color="runtimeStateColor[cardData.get(project.id)!.status!.state]"
              variant="subtle"
              size="sm"
            />

            <UButton
              icon="i-lucide-trash-2"
              color="error"
              variant="ghost"
              size="xs"
              aria-label="Delete project"
              class="opacity-0 transition-opacity group-hover:opacity-100"
              @click.prevent="deleteTarget = project"
            />
          </div>

          <p class="line-clamp-2 min-h-[2.5rem] text-sm text-muted">
            {{ project.description || 'No description.' }}
          </p>

          <div class="flex flex-wrap gap-1.5">
            <UBadge
              v-for="env in project.environments"
              :key="env.id"
              :label="env.name"
              color="neutral"
              variant="subtle"
              class="font-mono text-[11px]"
            />
            <UBadge
              v-if="cardData.get(project.id)?.tasks?.desired"
              :label="`${cardData.get(project.id)!.tasks!.running}/${cardData.get(project.id)!.tasks!.desired} tasks`"
              color="neutral"
              variant="outline"
              class="font-mono text-[11px]"
            />
          </div>

          <div class="flex items-center justify-between gap-2 border-t border-default pt-3 text-xs">
            <span v-if="cardData.get(project.id)?.status?.lastDeployment" class="flex items-center gap-1.5">
              <UBadge
                :label="cardData.get(project.id)!.status!.lastDeployment!.status"
                :color="deploymentStatusColor[cardData.get(project.id)!.status!.lastDeployment!.status]"
                variant="subtle"
                size="sm"
              />
              <span class="text-dimmed">#{{ cardData.get(project.id)!.status!.lastDeployment!.number }}</span>
            </span>
            <span v-else class="text-dimmed">Never deployed</span>

            <span class="text-dimmed">{{ formatDate(project.updatedAt) }}</span>
          </div>
        </NuxtLink>
      </div>

      <div
        v-else
        class="flex shrink-0 flex-col items-center gap-3 rounded-lg border border-dashed border-default py-12"
      >
        <UIcon name="i-lucide-package" class="size-8 text-dimmed" />
        <p class="text-sm text-muted">
          {{ search ? 'No project matches this search.' : 'No projects yet.' }}
        </p>
        <UButton
          v-if="search"
          label="Clear search"
          color="neutral"
          variant="subtle"
          @click="search = ''"
        />
        <UButton
          v-else
          label="Create your first project"
          icon="i-lucide-plus"
          to="/dashboard/projects/new"
        />
      </div>
    </template>
  </UDashboardPanel>

  <UModal
    :open="!!deleteTarget"
    title="Delete project"
    :description="`This removes ${deleteTarget?.name} from stacker and stops everything it is running. It cannot be undone.`"
    @update:open="deleteTarget = null"
  >
    <template #footer>
      <div class="flex w-full justify-end gap-2">
        <UButton label="Cancel" color="neutral" variant="ghost" @click="deleteTarget = null" />
        <UButton
          label="Delete"
          color="error"
          icon="i-lucide-trash-2"
          :loading="deleting"
          @click="confirmDelete"
        />
      </div>
    </template>
  </UModal>
</template>
