<script setup lang="ts">
import type { DropdownMenuItem, NavigationMenuItem } from '@nuxt/ui'
import type { Deployment } from '~/types/deployment'
import type { ProjectPayload } from '~/types/project'
import { isLive } from '~/types/deployment'

/**
 * The project detail page. Each section is its own route (`…/:id/domains`), so a
 * tab is linkable and the back button moves between them — the same shape as the
 * swarm resource pages.
 *
 * Edits go into a draft clone and are only sent to the server when Save is
 * pressed, which keeps the tabs free to be one long form split into parts. The
 * live status polls alongside the draft and never touches it: a poll landing must
 * not overwrite what someone is typing.
 */

// Keying on the project alone means moving between tabs reuses the component:
// the draft and the selected environment survive the click, and only opening a
// different project starts over.
definePageMeta({ key: route => String(route.params.id) })

const route = useRoute()
const router = useRouter()
const toast = useToast()

const {
  items,
  error: loadError,
  load,
  find,
  update,
  remove,
  statusOf,
  refreshProjectStatus,
  deploy: startDeploy,
  stop: stopEnvironment
} = useProjects()

const {
  items: deployments,
  load: loadDeployments,
  refresh: refreshDeployments,
  track,
  cancel: cancelDeployment
} = useDeployments()

const projectId = computed(() => String(route.params.id))
const project = computed(() => find(projectId.value))
const status = computed(() => statusOf(projectId.value))

const loading = ref(true)

onMounted(async () => {
  await Promise.all([load(), loadDeployments()])
  draft.value = toPayload()
  selectedEnvId.value = draft.value?.environments[0]?.id ?? ''
  loading.value = false
})

/**
 * One poll drives both halves of "real time" here: what docker is running, and
 * where the runs are. The deployment list is only re-read while something is
 * actually live, so an idle project settles down to a single status call.
 */
useLivePoll(async () => {
  if (!projectId.value) return

  await refreshProjectStatus(projectId.value)
  if (history.value.some(item => isLive(item.status))) await refreshDeployments()
}, 4000)

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

const draft = ref<ProjectPayload | null>(null)

// Switching projects (or a save landing) reloads the draft; switching tabs must
// not, or half-finished edits would vanish on every click.
watch(projectId, () => {
  draft.value = toPayload()
  selectedEnvId.value = draft.value?.environments[0]?.id ?? ''
})

const dirty = computed(() =>
  !!draft.value && JSON.stringify(draft.value) !== JSON.stringify(toPayload()))

/* ---- environment selection, shared by the per-environment tabs ---- */

const selectedEnvId = ref('')

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

const gitSource = computed(() => draft.value?.git ?? blankProject().git)
const { compose: gitCompose, pending: gitPreviewPending, error: gitPreviewError, services: composeServices } =
  useGitComposePreview(gitSource, isGit)

const previewCompose = computed(() =>
  isGit.value ? gitCompose.value : (draft.value?.compose ?? ''))

/* ---- save and deploy ---- */

const saving = ref(false)

/**
 * Saves the draft. Returns the stored project on success and null on failure, so
 * the deploy flow can tell the two apart — deploying a project whose save was
 * rejected would deploy the last thing that stuck, not what is on screen.
 */
async function save() {
  if (!draft.value) return null

  saving.value = true
  try {
    const saved = await update(projectId.value, clone(draft.value))
    // Re-read the draft from what came back: the server normalises hosts, fills
    // in ids for new rows and redacts secrets, and the form has to show that.
    draft.value = toPayload()
    toast.add({
      title: 'Project saved',
      description: saved.name,
      icon: 'i-lucide-check-circle',
      color: 'success'
    })
    return saved
  } catch (err: any) {
    toast.add({ title: 'Could not save', description: err.message, color: 'error' })
    return null
  } finally {
    saving.value = false
  }
}

function discard() {
  draft.value = toPayload()
}

/** Environment ids with a run in flight, so their buttons say so. */
const busy = computed(() =>
  deployments.value
    .filter(item => item.projectId === projectId.value && isLive(item.status))
    .map(item => item.environmentId))

/**
 * Deploys one environment, saving first when there are unsaved edits — the
 * button says "Save and deploy", and a deploy of stale configuration is the one
 * thing it must not do.
 */
async function deploy(environmentId: string) {
  if (dirty.value && !await save()) return

  const target = project.value?.environments.find(env => env.id === environmentId)
    ?? project.value?.environments[0]
  if (!project.value || !target) return

  try {
    const deployment = await startDeploy(project.value.id, target.id)
    track(deployment)
    toast.add({
      title: 'Deploying',
      description: `${project.value.name} · ${target.name}`,
      icon: 'i-lucide-rocket',
      color: 'success'
    })
    logTarget.value = deployment
  } catch (err: any) {
    toast.add({ title: 'Could not start the deploy', description: err.message, color: 'error' })
  }
}

const stopTarget = ref<string | null>(null)

async function confirmStop() {
  const environmentId = stopTarget.value
  const target = project.value?.environments.find(env => env.id === environmentId)
  if (!project.value || !target) return

  try {
    await stopEnvironment(project.value.id, target.id)
    await refreshProjectStatus(project.value.id)
    toast.add({
      title: 'Stopped',
      description: `${project.value.name} · ${target.name}`,
      icon: 'i-lucide-square',
      color: 'success'
    })
  } catch (err: any) {
    toast.add({ title: 'Could not stop', description: err.message, color: 'error' })
  } finally {
    stopTarget.value = null
  }
}

const deployItems = computed<DropdownMenuItem[][]>(() => [
  (draft.value?.environments ?? []).map(env => ({
    label: env.name,
    icon: 'i-lucide-layers',
    onSelect: () => deploy(env.id)
  }))
])

/* ---- logs ---- */

const logTarget = ref<Deployment | null>(null)

/** The row the modal shows, kept in step with the poll so its status is live. */
const logDeployment = computed(() => {
  if (!logTarget.value) return null
  return deployments.value.find(item => item.id === logTarget.value!.id) ?? logTarget.value
})

async function cancelRun(deployment: Deployment) {
  try {
    await cancelDeployment(deployment.id)
    toast.add({ title: 'Cancelling', description: `#${deployment.number}`, color: 'warning' })
  } catch (err: any) {
    toast.add({ title: 'Could not cancel', description: err.message, color: 'error' })
  }
}

/* ---- settings ---- */

const deleteOpen = ref(false)
const deleting = ref(false)

async function confirmDelete() {
  deleting.value = true
  try {
    await remove(projectId.value)
    deleteOpen.value = false
    await router.push('/dashboard/projects')
  } catch (err: any) {
    toast.add({ title: 'Could not delete', description: err.message, color: 'error' })
  } finally {
    deleting.value = false
  }
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
          <!-- A repository can be a full clone URL, which is long enough to push
               the rest of the navbar off screen, so the badge is capped. -->
          <UBadge
            v-if="project"
            :label="project.sourceKind === 'git' ? project.git.repo : 'compose file'"
            :title="project.sourceKind === 'git' ? project.git.repo : undefined"
            color="neutral"
            variant="subtle"
            class="max-w-48 truncate font-mono text-[11px]"
            :ui="{ label: 'truncate' }"
          />
          <UBadge
            v-if="status"
            :label="runtimeStateLabel[status.state]"
            :color="runtimeStateColor[status.state]"
            variant="subtle"
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
              :loading="saving"
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

      <UAlert
        v-if="loadError"
        :description="loadError"
        title="Could not load the project"
        icon="i-lucide-triangle-alert"
        color="error"
        variant="subtle"
        class="mb-4 shrink-0"
      />

      <div v-if="loading" class="flex shrink-0 justify-center py-12">
        <UIcon name="i-lucide-loader-circle" class="size-6 animate-spin text-dimmed" />
      </div>

      <div v-else-if="project && draft" class="w-full shrink-0 space-y-6">
        <!-- Overview -->
        <ProjectOverviewSection
          v-if="tab === 'overview'"
          :project="project"
          :status="status"
          :deployments="history"
          :busy="busy"
          @deploy="deploy"
          @stop="stopTarget = $event"
          @logs="logTarget = $event"
        />

        <!-- Source -->
        <section
          v-else-if="tab === 'source'"
          class="rounded-lg border border-default bg-default/60 p-5 backdrop-blur"
        >
          <header class="mb-4">
            <h2 class="text-sm font-semibold text-highlighted">Source</h2>
            <p class="text-sm text-muted">
              Where the compose file comes from. Every environment deploys this same source,
              cloned and built on this server for each run.
            </p>
          </header>

          <ProjectSourceSection :draft="draft" />

          <div v-if="previewCompose || gitPreviewPending" class="mt-5 border-t border-default pt-5">
            <ProjectComposePreview
              :draft="draft"
              :compose="previewCompose"
              :loading="gitPreviewPending"
              :error="gitPreviewError"
            />
          </div>
        </section>

        <!-- Environments -->
        <section
          v-else-if="tab === 'environments'"
          class="rounded-lg border border-default bg-default/60 p-5 backdrop-blur"
        >
          <header class="mb-4">
            <h2 class="text-sm font-semibold text-highlighted">Environments</h2>
            <p class="text-sm text-muted">
              Variables and secrets, per environment. Both are passed to every service of the
              stack and are available for <code class="font-mono">${VAR}</code> in the compose file.
            </p>
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
            <p class="text-sm text-muted">
              Hostnames Traefik routes to this project's services. They are published on the
              next deploy.
            </p>
          </header>

          <ProjectEnvSwitcher
            v-model="selectedEnvId"
            :environments="draft.environments"
            class="mb-5"
          />

          <ProjectDomainsSection
            v-if="selectedEnv"
            :key="selectedEnv.id"
            :environment="selectedEnv"
            :service-items="composeServices"
          />
        </section>

        <!-- Deploy -->
        <section
          v-else-if="tab === 'deploy'"
          class="rounded-lg border border-default bg-default/60 p-5 backdrop-blur"
        >
          <header class="mb-4">
            <h2 class="text-sm font-semibold text-highlighted">Deploy</h2>
            <p class="text-sm text-muted">
              How each environment rolls out. Replicas and placement are applied to every
              service that does not pin its own in the compose file.
            </p>
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
              <p class="text-sm text-muted">Every deployment of this project. Select one for its log.</p>
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
            <span class="min-w-0 flex-1 truncate text-muted">
              {{ deployment.message || deployment.error || deployment.actor }}
            </span>
            <span class="text-xs text-dimmed">{{ formatTime(deployment.startedAt) }}</span>
            <UButton
              icon="i-lucide-scroll-text"
              size="xs"
              color="neutral"
              variant="ghost"
              aria-label="View log"
              @click="logTarget = deployment"
            />
            <UButton
              v-if="deployment.status === 'running' || deployment.status === 'queued'"
              icon="i-lucide-x"
              size="xs"
              color="warning"
              variant="ghost"
              aria-label="Cancel"
              @click="cancelRun(deployment)"
            />
          </div>
        </section>

        <!-- Settings -->
        <template v-else-if="tab === 'settings'">
          <section class="rounded-lg border border-default bg-default/60 p-5 backdrop-blur">
            <header class="mb-4">
              <h2 class="text-sm font-semibold text-highlighted">General</h2>
            </header>

            <div class="grid gap-4 sm:grid-cols-2">
              <UFormField label="Name" help="Renaming redeploys under a new stack name.">
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
              Removes the project, stops every stack it deployed and deletes the routes for its
              hostnames.
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
          <UButton label="Discard" color="neutral" variant="ghost" :disabled="saving" @click="discard" />
          <UButton
            label="Save"
            color="neutral"
            variant="subtle"
            icon="i-lucide-save"
            :loading="saving"
            @click="save"
          />
          <UButton
            label="Save and deploy"
            icon="i-lucide-rocket"
            :disabled="saving"
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
    :description="`This removes ${project?.name} from stacker and stops everything it is running. It cannot be undone.`"
  >
    <template #footer>
      <div class="flex w-full justify-end gap-2">
        <UButton label="Cancel" color="neutral" variant="ghost" @click="deleteOpen = false" />
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

  <UModal
    :open="!!stopTarget"
    title="Stop this environment"
    description="The stack is removed and its hostnames stop being routed. The configuration is kept, so it can be deployed again unchanged."
    @update:open="stopTarget = null"
  >
    <template #footer>
      <div class="flex w-full justify-end gap-2">
        <UButton label="Cancel" color="neutral" variant="ghost" @click="stopTarget = null" />
        <UButton label="Stop" color="error" icon="i-lucide-square" @click="confirmStop" />
      </div>
    </template>
  </UModal>

  <UModal
    :open="!!logTarget"
    :title="`Deployment #${logDeployment?.number ?? ''}`"
    :ui="{ content: 'max-w-4xl' }"
    @update:open="logTarget = null"
  >
    <template #body>
      <DeploymentLogViewer v-if="logDeployment" :key="logDeployment.id" :deployment="logDeployment" />
    </template>

    <template #footer>
      <div class="flex w-full justify-end gap-2">
        <UButton
          v-if="logDeployment && (logDeployment.status === 'running' || logDeployment.status === 'queued')"
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
