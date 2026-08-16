<script setup lang="ts">
const toast = useToast()
const route = useRoute()
const github = useGitHub()
const name = ref(`stacker-${window.location.hostname.replace(/[^a-z0-9-]/gi, '-')}`)
const organization = ref('')
const submitting = ref(false)
const created = computed(() => Boolean(github.app.value?.appId))
const connected = computed(() => Boolean(github.app.value?.installationId))

onMounted(async () => {
  await github.load()
  if (route.query.github === 'created') toast.add({ title: 'GitHub App created', description: 'Install it to choose repository access.', color: 'success' })
  if (route.query.github === 'installed') toast.add({ title: 'GitHub connected', description: 'Repository access is ready.', color: 'success' })
  if (route.query.github === 'error') toast.add({ title: 'GitHub connection failed', description: 'Check the server logs and try again.', color: 'error' })
})

async function createApp() {
  submitting.value = true
  try { await github.create(name.value.trim(), organization.value.trim()) }
  catch (err: any) {
    toast.add({ title: 'Could not create GitHub App', description: err.message, color: 'error' })
    submitting.value = false
  }
}

async function disconnect() {
  try {
    await github.disconnect()
    toast.add({ title: 'GitHub disconnected', description: 'The app still exists on GitHub and can be deleted there.', color: 'warning' })
  } catch (err: any) { toast.add({ title: 'Could not disconnect', description: err.message, color: 'error' }) }
}

async function refresh() {
  try { await github.loadRepositories(); toast.add({ title: 'Repositories refreshed', color: 'success' }) }
  catch (err: any) { toast.add({ title: 'Refresh failed', description: err.message, color: 'error' }) }
}

const formatDate = (value: string) => new Date(value).toLocaleDateString(undefined, { day: 'numeric', month: 'short', year: 'numeric' })
</script>

<template>
  <SettingsSection title="GitHub" description="Create and install a private GitHub App to deploy your repositories.">
    <template #header-right>
      <UBadge :label="connected ? 'Connected' : created ? 'Install required' : 'Not connected'" :color="connected ? 'success' : created ? 'warning' : 'neutral'" variant="subtle" />
    </template>

    <div v-if="github.pending.value" class="flex items-center justify-center py-10">
      <UIcon name="i-lucide-loader-circle" class="size-6 animate-spin text-dimmed" />
    </div>
    <div v-else-if="github.error.value" class="rounded-md border border-error/30 bg-error/5 p-4 text-sm text-error">
      {{ github.error.value }}
      <UButton label="Retry" size="xs" color="neutral" variant="soft" class="ms-3" @click="github.load" />
    </div>

    <template v-else-if="connected && github.app.value">
      <div class="flex flex-wrap items-center gap-3 rounded-md border border-default bg-elevated/40 p-4">
        <div class="flex size-9 items-center justify-center rounded-md bg-elevated ring-1 ring-default"><UIcon name="i-lucide-github" class="size-5 text-highlighted" /></div>
        <div class="min-w-0 leading-tight">
          <p class="font-medium text-highlighted">{{ github.app.value.account }}</p>
          <p class="font-mono text-xs text-dimmed">{{ github.app.value.name }} · installed {{ formatDate(github.app.value.updatedAt) }}</p>
        </div>
        <div class="ms-auto flex gap-2">
          <UButton label="Configure on GitHub" icon="i-lucide-arrow-up-right" color="neutral" variant="subtle" size="sm" :to="`https://github.com/settings/installations/${github.app.value.installationId}`" external />
          <UButton label="Disconnect" icon="i-lucide-unplug" color="error" variant="ghost" size="sm" @click="disconnect" />
        </div>
      </div>
      <div class="mt-5">
        <div class="mb-2 flex items-center justify-between">
          <p class="text-xs uppercase tracking-[0.08em] text-dimmed">Repository access · {{ github.app.value.repositorySelection }}</p>
          <UButton label="Refresh" icon="i-lucide-refresh-cw" size="xs" color="neutral" variant="ghost" @click="refresh" />
        </div>
        <a v-for="repo in github.repositories.value" :key="repo.id" :href="repo.htmlUrl" target="_blank" rel="noreferrer" class="flex items-center gap-3 border-t border-default py-2.5 text-sm first:border-t-0">
          <UIcon :name="repo.private ? 'i-lucide-lock' : 'i-lucide-book-open'" class="size-4 text-dimmed" />
          <span class="font-mono text-xs text-toned">{{ repo.fullName }}</span>
          <UIcon name="i-lucide-arrow-up-right" class="ms-auto size-3.5 text-dimmed" />
        </a>
        <p v-if="!github.repositories.value.length" class="py-5 text-center text-sm text-muted">No repositories are available to this installation.</p>
      </div>
    </template>

    <div v-else-if="created && github.app.value" class="flex flex-col items-center gap-3 rounded-md border border-dashed border-warning/40 py-10">
      <UIcon name="i-lucide-github" class="size-8 text-dimmed" />
      <p class="max-w-md text-center text-sm text-muted">The GitHub App <strong>{{ github.app.value.name }}</strong> was created. Install it and choose repository access.</p>
      <UButton label="Install GitHub App" icon="i-lucide-github" @click="github.install" />
      <UButton label="Start over" color="neutral" variant="ghost" size="sm" @click="disconnect" />
    </div>

    <form v-else class="mx-auto max-w-lg space-y-4 rounded-md border border-dashed border-default p-5" @submit.prevent="createApp">
      <UFormField label="GitHub App name" help="Must be unique across GitHub.">
        <UInput v-model="name" class="w-full" required maxlength="100" placeholder="stacker-my-server" />
      </UFormField>
      <UFormField label="Organization (optional)" help="Leave empty to create it on your personal account.">
        <UInput v-model="organization" class="w-full" maxlength="100" placeholder="my-organization" />
      </UFormField>
      <UButton type="submit" block label="Create GitHub App" icon="i-lucide-github" :loading="submitting" />
      <p class="text-center text-xs text-dimmed">You will continue on GitHub, then return here to install the app.</p>
    </form>
  </SettingsSection>
</template>
