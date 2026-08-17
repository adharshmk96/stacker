<script setup lang="ts">
import type { FormError, FormSubmitEvent } from '@nuxt/ui'
import type { ProjectPayload } from '~/types/project'

/**
 * The create form: the minimum needed to deploy something — name, source and a
 * first environment. Domains and rollout settings live on the project's own
 * tabs, so this page stays short.
 */
const emit = defineEmits<{
  /** `deploy` is true when the user pressed "Save and deploy" */
  submit: [payload: ProjectPayload, deploy: boolean]
  cancel: []
}>()

const state = reactive<ProjectPayload>(blankProject())

const ipFrom = (host: string) => host.match(/(?:^|\.)(\d{1,3}(?:\.\d{1,3}){3})(?:\.|$)/)?.[1] ?? ''
const dashboardIp = ref('')
let generatedHost = ''

watch([() => state.name, dashboardIp], ([name, ip]) => {
  const domain = state.environments[0]!.domains[0] ?? blankDomain()
  const hostName = name.trim().toLowerCase().replaceAll('_', '-')
  const nextHost = hostName && ip ? `${hostName}.${ip}.nip.io` : ''

  if (!state.environments[0]!.domains.length) state.environments[0]!.domains.push(domain)
  if (!domain.host || domain.host === generatedHost) domain.host = nextHost
  generatedHost = nextHost
})

onMounted(async () => {
  try {
    const settings = await useApi().get<{ instance: { ip?: string } }>('/server')
    dashboardIp.value = ipFrom(settings.instance.ip ?? '')
  } catch {
    // The required-field validation remains the fallback when server settings
    // are unavailable, instead of replacing a host the user enters manually.
  }
})

const isGit = computed(() => state.sourceKind === 'git')

const selectedEnvId = ref(state.environments[0]!.id)

const selectedEnv = computed(() =>
  state.environments.find(env => env.id === selectedEnvId.value)!)

function addEnvironment(name: string) {
  const env = blankEnvironment(name)
  state.environments.push(env)
  selectedEnvId.value = env.id
}

function removeEnvironment(id: string) {
  // A project with no environment has nothing to deploy, so the last one stays.
  if (state.environments.length === 1) return

  state.environments = state.environments.filter(env => env.id !== id)
  if (selectedEnvId.value === id) selectedEnvId.value = state.environments[0]!.id
}

function validate(state: ProjectPayload): FormError[] {
  const errors: FormError[] = []

  if (!state.name.trim()) {
    errors.push({ name: 'name', message: 'Name is required' })
  } else if (!/^[a-z0-9][a-z0-9_-]*$/i.test(state.name.trim())) {
    errors.push({ name: 'name', message: 'Use letters, numbers, dashes and underscores' })
  }

  if (state.sourceKind === 'git') {
    if (!state.git.repo.trim()) {
      errors.push({ name: 'git.repo', message: 'Repository is required' })
    }
    if (!state.git.branch.trim()) {
      errors.push({ name: 'git.branch', message: 'Branch is required' })
    }
    if (!state.git.composePath.trim()) {
      errors.push({ name: 'git.composePath', message: 'Path to the compose file is required' })
    }
  } else if (!state.compose.trim()) {
    errors.push({ name: 'compose', message: 'Paste a compose file' })
  }

  // Only one environment is on screen at a time, so an error on another one
  // would have nowhere to render — select it first, then report it.
  const unnamed = state.environments.find(env => !env.name.trim())
  if (unnamed) {
    selectedEnvId.value = unnamed.id
    errors.push({ name: 'env.name', message: 'Every environment needs a name' })
    return errors
  }

  const missingHost = state.environments.find(env => !env.domains[0]?.host.trim())
  if (missingHost) {
    selectedEnvId.value = missingHost.id
    errors.push({ name: 'env.host', message: 'Host is required' })
    return errors
  }

  // A host with nothing behind it would be published and route nowhere.
  const hostless = state.environments.find((env) => {
    const domain = env.domains[0]
    return domain?.host.trim() && !domain.service.trim()
  })
  if (hostless) {
    selectedEnvId.value = hostless.id
    errors.push({ name: 'env.service', message: 'Which service should this host route to?' })
  }

  return errors
}

/** Which button was pressed — the form submits through both. */
const deployAfterSave = ref(false)

function onSubmit(event: FormSubmitEvent<ProjectPayload>) {
  // A JSON round trip, not structuredClone: the form state is a reactive proxy.
  emit('submit', JSON.parse(JSON.stringify(event.data)) as ProjectPayload, deployAfterSave.value)
  deployAfterSave.value = false
}
</script>

<template>
  <UForm
    id="project-form"
    :state="state"
    :validate="validate"
    class="space-y-6"
    @submit="onSubmit"
    @error="deployAfterSave = false"
  >
    <section class="rounded-lg border border-default bg-default/60 p-5 backdrop-blur">
      <header class="mb-4">
        <h2 class="text-sm font-semibold text-highlighted">Project</h2>
        <p class="text-sm text-muted">What this stack is called and what it runs.</p>
      </header>

      <div class="grid gap-4 sm:grid-cols-2">
        <UFormField label="Name" name="name" required>
          <UInput v-model="state.name" placeholder="storefront" class="w-full" autofocus />
        </UFormField>

        <UFormField label="Description" name="description" hint="Optional">
          <UInput v-model="state.description" placeholder="Public web store" class="w-full" />
        </UFormField>
      </div>
    </section>

    <section class="rounded-lg border border-default bg-default/60 p-5 backdrop-blur">
      <header class="mb-4">
        <h2 class="text-sm font-semibold text-highlighted">Source</h2>
        <p class="text-sm text-muted">Where the compose file comes from.</p>
      </header>

      <ProjectSourceSection :draft="state" />
    </section>

    <section class="rounded-lg border border-default bg-default/60 p-5 backdrop-blur">
      <header class="mb-4">
        <h2 class="text-sm font-semibold text-highlighted">Environments</h2>
        <p class="text-sm text-muted">
          Each environment deploys the same source with its own host, variables and secrets.
          Rollout settings and extra domains can be tuned once the project exists.
        </p>
      </header>

      <ProjectEnvSwitcher
        v-model="selectedEnvId"
        :environments="state.environments"
        editable
        class="mb-5"
        @add="addEnvironment"
        @remove="removeEnvironment"
      />

      <ProjectEnvironmentSection
        :key="selectedEnv.id"
        :environment="selectedEnv"
        :default-branch="state.git.branch"
        :show-branch="isGit"
        show-host
        host-required
      />
    </section>

    <div class="flex flex-wrap items-center justify-end gap-2 border-t border-default pt-4">
      <UButton label="Cancel" color="neutral" variant="ghost" @click="emit('cancel')" />
      <UButton
        label="Save"
        type="submit"
        color="neutral"
        variant="subtle"
        icon="i-lucide-save"
        @click="deployAfterSave = false"
      />
      <UButton
        label="Save and deploy"
        type="submit"
        icon="i-lucide-rocket"
        class="shadow-lg shadow-primary/20"
        @click="deployAfterSave = true"
      />
    </div>
  </UForm>
</template>
