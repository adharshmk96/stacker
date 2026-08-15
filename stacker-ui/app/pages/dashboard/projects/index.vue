<script setup lang="ts">
import type { Project } from '~/types/project'

useHead({ title: 'Projects · Stacker' })

const { items, remove } = useProjects()
const { items: deployments } = useDeployments()

const search = ref('')

const filtered = computed(() => {
  const term = search.value.trim().toLowerCase()
  if (!term) return items.value

  return items.value.filter(project =>
    [project.name, project.description, project.git.repo]
      .some(field => field.toLowerCase().includes(term)))
})

/** Newest deployment per project — the card's status line. */
const lastDeployment = computed(() => {
  const map = new Map<string, (typeof deployments.value)[number]>()
  for (const deployment of deployments.value) {
    const current = map.get(deployment.projectId)
    if (!current || deployment.startedAt > current.startedAt) {
      map.set(deployment.projectId, deployment)
    }
  }
  return map
})

const sourceLabel = (project: Project) =>
  project.sourceKind === 'git' ? project.git.repo : 'compose file'

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
        title="Placeholder"
        description="Projects are not wired to the stacker server yet — everything here lives in the browser and resets on reload."
        icon="i-lucide-flask-conical"
        color="neutral"
        variant="subtle"
        class="mb-4 shrink-0"
      />

      <div v-if="filtered.length" class="grid shrink-0 gap-3 md:grid-cols-2 xl:grid-cols-3">
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
            <UButton
              icon="i-lucide-trash-2"
              color="error"
              variant="ghost"
              size="xs"
              aria-label="Delete project"
              class="opacity-0 transition-opacity group-hover:opacity-100"
              @click.prevent="remove(project.id)"
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
          </div>

          <div class="flex items-center justify-between gap-2 border-t border-default pt-3 text-xs">
            <span v-if="lastDeployment.get(project.id)" class="flex items-center gap-1.5">
              <UBadge
                :label="lastDeployment.get(project.id)!.status"
                :color="deploymentStatusColor[lastDeployment.get(project.id)!.status]"
                variant="subtle"
                size="sm"
              />
              <span class="text-dimmed">#{{ lastDeployment.get(project.id)!.number }}</span>
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
</template>
