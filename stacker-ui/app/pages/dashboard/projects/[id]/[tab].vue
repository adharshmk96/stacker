<script setup lang="ts">
import type { DropdownMenuItem, NavigationMenuItem } from '@nuxt/ui'
import type { ProjectPayload } from '~/types/project'

/**
 * The project detail page. Each section is its own route (`…/:id/domains`), so
 * a tab is linkable and the back button moves between them — the same shape as
 * the swarm resource pages.
 *
 * Edits go into a draft clone and are only written back to the store when Save
 * is pressed, which keeps the tabs free to be one long form split into parts.
 */

// Keying on the project alone means moving between tabs reuses the component:
// the draft and the selected environment survive the click, and only opening a
// different project starts over.
definePageMeta({ key: route => String(route.params.id) })

const route = useRoute()
const router = useRouter()
const toast = useToast()

const { find, update, remove } = useProjects()
const { items: deployments, enqueue } = useDeployments()

const projectId = computed(() => String(route.params.id))
const project = computed(() => find(projectId.value))

const tabs = [
  { key: 'overview', label: 'Overview', icon: 'i-lucide-layout-dashboard' },
  { key: 'source', label: 'Source', icon: 'i-lucide-git-branch' },
  { key: 'environments', label: 'Environments', icon: 'i-lucide-layers' },
  { key: 'domains', label: 'Domains', icon: 'i-lucide-globe' },
  { key: 'deploy', label: 'Deploy', icon: 'i-lucide-rocket' },
  { key: 'activity', label: 'Activity', icon: 'i-lucide-history' },
  { key: 'settings', label: 'Settings', icon: 'i-lucide-settings' }
] as const

const tab = computed(() => String(route.params.tab))

// An unknown segment is a 404 rather than a silently blank panel.
watchEffect(() => {
  if (!tabs.some(item => item.key === tab.value)) {
    throw createError({ statusCode: 404, statusMessage: 'Unknown project tab', fatal: true })
  }
})

const tabItems = computed<NavigationMenuItem[]>(() =>
  tabs.map(item => ({
    label: item.label,
    icon: item.icon,
    to: `/dashboard/projects/${projectId.value}/${item.key}`
  })))

useHead(() => ({ title: `${project.value?.name ?? 'Project'} · Stacker` }))

/* ---- draft ---- */

/**
 * The store holds deeply reactive objects, so a plain JSON round trip is the
 * clone that works here — `structuredClone` chokes on the proxies.
 */
const clone = <T>(value: T): T => JSON.parse(JSON.stringify(value)) as T

function toPayload(): ProjectPayload | null {
  if (!project.value) return null

  const { id, createdAt, updatedAt, ...payload } = project.value
  return clone(payload)
}

const draft = ref<ProjectPayload | null>(toPayload())

// Switching projects (or a save landing) reloads the draft; switching tabs must
// not, or half-finished edits would vanish on every click.
watch(projectId, () => {
  draft.value = toPayload()
  selectedEnvId.value = draft.value?.environments[0]?.id ?? ''
})

const dirty = computed(() =>
  !!draft.value && JSON.stringify(draft.value) !== JSON.stringify(toPayload()))

/* ---- environment selection, shared by the per-environment tabs ---- */

const selectedEnvId = ref(draft.value?.environments[0]?.id ?? '')

const selectedEnv = computed(() =>
  draft.value?.environments.find(env => env.id === selectedEnvId.value)
  ?? draft.value?.environments[0])

function addEnvironment(name: string) {
  if (!draft.value) return

  const env = blankEnvironment(name)
  draft.value.environments.push(env)
  selectedEnvId.value = env.id
}

function removeEnvironment(id: string) {
  if (!draft.value || draft.value.environments.length === 1) return

  draft.value.environments = draft.value.environments.filter(env => env.id !== id)
  if (selectedEnvId.value === id) selectedEnvId.value = draft.value.environments[0]!.id
}

const isGit = computed(() => draft.value?.sourceKind === 'git')

/* ---- save and deploy ---- */

function save() {
  if (!draft.value) return null

  const saved = update(projectId.value, clone(draft.value))
  toast.add({
    title: 'Project saved',
    description: saved.name,
    icon: 'i-lucide-check-circle',
    color: 'success'
  })
  return saved
}

function discard() {
  draft.value = toPayload()
}

/** Placeholder: no build runs, the deployment is queued so the list moves. */
function deploy(environmentId: string) {
  const saved = dirty.value ? save() : project.value
  if (!saved) return

  const env = saved.environments.find(item => item.id === environmentId) ?? saved.environments[0]!
  enqueue(saved.id, saved.name, env.name)

  toast.add({
    title: 'Deployment queued',
    description: `${saved.name} · ${env.name}`,
    icon: 'i-lucide-rocket',
    color: 'success'
  })
  router.push('/dashboard/deployments')
}

const deployItems = computed<DropdownMenuItem[][]>(() => [
  (draft.value?.environments ?? []).map(env => ({
    label: env.name,
    icon: 'i-lucide-layers',
    onSelect: () => deploy(env.id)
  }))
])

/* ---- settings ---- */

const deleteOpen = ref(false)

function confirmDelete() {
  remove(projectId.value)
  deleteOpen.value = false
  router.push('/dashboard/projects')
}

/* ---- activity ---- */

const history = computed(() =>
  deployments.value
    .filter(deployment => deployment.projectId === projectId.value)
    .sort((a, b) => b.startedAt.localeCompare(a.startedAt)))

const formatTime = (value: string) =>
  new Date(value).toLocaleString(undefined, {
    day: 'numeric', month: 'short', hour: '2-digit', minute: '2-digit'
  })
</script>

<template>
  <UDashboardPanel id="project-detail" :ui="{ body: 'relative' }">
    <template #header>
      <UDashboardNavbar :title="project?.name ?? 'Project'">
        <template #leading>
          <UDashboardSidebarCollapse />
        </template>

        <template #trailing>
          <UBadge
            v-if="project"
            :label="project.sourceKind === 'git' ? project.git.repo : 'compose file'"
            color="neutral"
            variant="subtle"
            class="font-mono text-[11px]"
          />
        </template>

        <template #right>
          <UButton
            label="All projects"
            icon="i-lucide-arrow-left"
            color="neutral"
            variant="ghost"
            to="/dashboard/projects"
          />
          <UDropdownMenu v-if="project" :items="deployItems">
            <UButton
              label="Deploy"
              icon="i-lucide-rocket"
              trailing-icon="i-lucide-chevron-down"
              class="shadow-lg shadow-primary/20"
            />
          </UDropdownMenu>
        </template>
      </UDashboardNavbar>

      <UDashboardToolbar>
        <template #left>
          <UNavigationMenu :items="tabItems" highlight class="-mb-px" />
        </template>
      </UDashboardToolbar>
    </template>

    <template #body>
      <div class="stacker-grid pointer-events-none absolute inset-0" />

      <div v-if="project && draft" class="w-full shrink-0 space-y-6">
        <!-- Overview -->
        <ProjectOverviewSection
          v-if="tab === 'overview'"
          :project="project"
          :deployments="history"
          @deploy="deploy"
        />

        <!-- Source -->
        <section
          v-else-if="tab === 'source'"
          class="rounded-lg border border-default bg-default/60 p-5 backdrop-blur"
        >
          <header class="mb-4">
            <h2 class="text-sm font-semibold text-highlighted">Source</h2>
            <p class="text-sm text-muted">
              Where the compose file comes from. Every environment deploys this same source.
            </p>
          </header>

          <ProjectSourceSection :draft="draft" />
        </section>

        <!-- Environments -->
        <section
          v-else-if="tab === 'environments'"
          class="rounded-lg border border-default bg-default/60 p-5 backdrop-blur"
        >
          <header class="mb-4">
            <h2 class="text-sm font-semibold text-highlighted">Environments</h2>
            <p class="text-sm text-muted">Variables and secrets, per environment.</p>
          </header>

          <ProjectEnvSwitcher
            v-model="selectedEnvId"
            :environments="draft.environments"
            editable
            class="mb-5"
            @add="addEnvironment"
            @remove="removeEnvironment"
          />

          <ProjectEnvironmentSection
            v-if="selectedEnv"
            :key="selectedEnv.id"
            :environment="selectedEnv"
            :default-branch="draft.git.branch"
            :show-branch="isGit"
          />
        </section>

        <!-- Domains -->
        <section
          v-else-if="tab === 'domains'"
          class="rounded-lg border border-default bg-default/60 p-5 backdrop-blur"
        >
          <header class="mb-4">
            <h2 class="text-sm font-semibold text-highlighted">Domains</h2>
            <p class="text-sm text-muted">Hostnames routed to this project's services.</p>
          </header>

          <ProjectEnvSwitcher
            v-model="selectedEnvId"
            :environments="draft.environments"
            class="mb-5"
          />

          <ProjectDomainsSection v-if="selectedEnv" :key="selectedEnv.id" :environment="selectedEnv" />
        </section>

        <!-- Deploy -->
        <section
          v-else-if="tab === 'deploy'"
          class="rounded-lg border border-default bg-default/60 p-5 backdrop-blur"
        >
          <header class="mb-4">
            <h2 class="text-sm font-semibold text-highlighted">Deploy</h2>
            <p class="text-sm text-muted">When each environment deploys, and how it rolls out.</p>
          </header>

          <ProjectEnvSwitcher
            v-model="selectedEnvId"
            :environments="draft.environments"
            class="mb-5"
          />

          <ProjectDeploySection
            v-if="selectedEnv"
            :key="selectedEnv.id"
            :environment="selectedEnv"
            :is-git="isGit"
          />
        </section>

        <!-- Activity -->
        <section
          v-else-if="tab === 'activity'"
          class="rounded-lg border border-default bg-default/60 p-5 backdrop-blur"
        >
          <header class="mb-3 flex items-center justify-between gap-3">
            <div>
              <h2 class="text-sm font-semibold text-highlighted">Activity</h2>
              <p class="text-sm text-muted">Every deployment of this project.</p>
            </div>
            <UButton
              label="All deployments"
              icon="i-lucide-arrow-up-right"
              size="xs"
              color="neutral"
              variant="ghost"
              to="/dashboard/deployments"
            />
          </header>

          <p v-if="!history.length" class="text-sm text-dimmed">
            This project has never been deployed.
          </p>

          <div
            v-for="deployment in history"
            :key="deployment.id"
            class="flex items-center gap-3 border-t border-default py-2.5 text-sm first:border-t-0"
          >
            <UBadge
              :label="deployment.status"
              :color="deploymentStatusColor[deployment.status]"
              variant="subtle"
              size="sm"
            />
            <span class="font-mono text-xs text-dimmed">#{{ deployment.number }}</span>
            <span class="font-mono text-xs text-toned">{{ deployment.environment }}</span>
            <span class="font-mono text-xs text-dimmed">{{ deployment.revision }}</span>
            <span class="min-w-0 flex-1 truncate text-muted">{{ deployment.message }}</span>
            <span class="text-xs text-dimmed">{{ formatTime(deployment.startedAt) }}</span>
          </div>
        </section>

        <!-- Settings -->
        <template v-else-if="tab === 'settings'">
          <section class="rounded-lg border border-default bg-default/60 p-5 backdrop-blur">
            <header class="mb-4">
              <h2 class="text-sm font-semibold text-highlighted">General</h2>
            </header>

            <div class="grid gap-4 sm:grid-cols-2">
              <UFormField label="Name">
                <UInput v-model="draft.name" class="w-full" />
              </UFormField>

              <UFormField label="Description" hint="Optional">
                <UInput v-model="draft.description" class="w-full" />
              </UFormField>
            </div>

            <dl class="mt-5 grid gap-4 border-t border-default pt-4 sm:grid-cols-2">
              <div>
                <dt class="text-xs uppercase tracking-[0.08em] text-dimmed">Created</dt>
                <dd class="mt-1 text-sm text-toned">{{ formatTime(project.createdAt) }}</dd>
              </div>
              <div>
                <dt class="text-xs uppercase tracking-[0.08em] text-dimmed">Last updated</dt>
                <dd class="mt-1 text-sm text-toned">{{ formatTime(project.updatedAt) }}</dd>
              </div>
            </dl>
          </section>

          <section class="rounded-lg border border-error/40 bg-default/60 p-5 backdrop-blur">
            <h2 class="text-sm font-semibold text-highlighted">Delete project</h2>
            <p class="mb-4 mt-1 text-sm text-muted">
              Removes the project and its configuration. Running services are left untouched.
            </p>
            <UButton
              label="Delete project"
              icon="i-lucide-trash-2"
              color="error"
              variant="subtle"
              @click="deleteOpen = true"
            />
          </section>
        </template>

        <!-- save bar, shared by every editing tab -->
        <div
          v-if="dirty"
          class="sticky bottom-0 flex flex-wrap items-center justify-end gap-2 border-t border-default bg-default/80 py-3 backdrop-blur"
        >
          <p class="mr-auto text-sm text-muted">Unsaved changes</p>
          <UButton label="Discard" color="neutral" variant="ghost" @click="discard" />
          <UButton
            label="Save"
            color="neutral"
            variant="subtle"
            icon="i-lucide-save"
            @click="save"
          />
          <UButton
            label="Save and deploy"
            icon="i-lucide-rocket"
            class="shadow-lg shadow-primary/20"
            @click="deploy(selectedEnvId)"
          />
        </div>
      </div>

      <div v-else class="flex shrink-0 flex-col items-center gap-3 py-12">
        <UIcon name="i-lucide-package-x" class="size-8 text-dimmed" />
        <p class="text-sm text-muted">This project no longer exists.</p>
        <UButton label="Back to projects" color="neutral" variant="subtle" to="/dashboard/projects" />
      </div>
    </template>
  </UDashboardPanel>

  <UModal
    v-model:open="deleteOpen"
    title="Delete project"
    :description="`This removes ${project?.name} from stacker. It cannot be undone.`"
  >
    <template #footer>
      <div class="flex w-full justify-end gap-2">
        <UButton label="Cancel" color="neutral" variant="ghost" @click="deleteOpen = false" />
        <UButton label="Delete" color="error" icon="i-lucide-trash-2" @click="confirmDelete" />
      </div>
    </template>
  </UModal>
</template>
