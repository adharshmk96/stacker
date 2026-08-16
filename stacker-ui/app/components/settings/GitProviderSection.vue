<script setup lang="ts">
/**
 * Git Provider tab. Connecting is a GitHub App install, so the "connect" button
 * is a hand-off to GitHub rather than a form — nothing is typed here.
 *
 * Placeholder: the connection is local state so both the connected and the
 * empty state can be seen without a backend.
 */
const toast = useToast()

const connected = ref(true)

const installation = reactive({
  account: 'adharshmk96',
  appName: 'stacker-deploy',
  installedAt: '2026-07-02T10:24:00Z',
  scope: 'Selected repositories'
})

const repositories = [
  { name: 'adharshmk96/stacker', private: false, projects: 2 },
  { name: 'adharshmk96/api-gateway', private: true, projects: 1 },
  { name: 'adharshmk96/landing-page', private: true, projects: 0 }
]

function connect() {
  connected.value = true
  toast.add({
    title: 'GitHub App installed',
    description: `Connected as ${installation.account}`,
    icon: 'i-lucide-github',
    color: 'success'
  })
}

function disconnect() {
  connected.value = false
  toast.add({
    title: 'GitHub disconnected',
    description: 'Projects using GitHub sources will stop deploying.',
    icon: 'i-lucide-unplug',
    color: 'warning'
  })
}

const formatDate = (value: string) =>
  new Date(value).toLocaleDateString(undefined, { day: 'numeric', month: 'short', year: 'numeric' })
</script>

<template>
  <SettingsSection
    title="GitHub"
    description="Install the stacker GitHub App to deploy from your repositories."
  >
    <template #header-right>
      <UBadge
        :label="connected ? 'Connected' : 'Not connected'"
        :color="connected ? 'success' : 'neutral'"
        variant="subtle"
        :icon="connected ? 'i-lucide-check' : 'i-lucide-minus'"
      />
    </template>

    <!-- connected -->
    <template v-if="connected">
      <div class="flex flex-wrap items-center gap-3 rounded-md border border-default bg-elevated/40 p-4">
        <div class="flex size-9 items-center justify-center rounded-md bg-elevated ring-1 ring-default">
          <UIcon name="i-lucide-github" class="size-5 text-highlighted" />
        </div>
        <div class="min-w-0 leading-tight">
          <p class="font-medium text-highlighted">{{ installation.account }}</p>
          <p class="font-mono text-xs text-dimmed">
            {{ installation.appName }} · installed {{ formatDate(installation.installedAt) }}
          </p>
        </div>
        <div class="ms-auto flex gap-2">
          <UButton
            label="Configure on GitHub"
            icon="i-lucide-arrow-up-right"
            color="neutral"
            variant="subtle"
            size="sm"
          />
          <UButton
            label="Disconnect"
            icon="i-lucide-unplug"
            color="error"
            variant="ghost"
            size="sm"
            @click="disconnect"
          />
        </div>
      </div>

      <div class="mt-5">
        <div class="mb-2 flex items-center justify-between">
          <p class="text-xs uppercase tracking-[0.08em] text-dimmed">
            Repository access · {{ installation.scope }}
          </p>
          <UButton label="Refresh" icon="i-lucide-refresh-cw" size="xs" color="neutral" variant="ghost" />
        </div>

        <div
          v-for="repo in repositories"
          :key="repo.name"
          class="flex items-center gap-3 border-t border-default py-2.5 text-sm first:border-t-0"
        >
          <UIcon
            :name="repo.private ? 'i-lucide-lock' : 'i-lucide-book-open'"
            class="size-4 text-dimmed"
          />
          <span class="font-mono text-xs text-toned">{{ repo.name }}</span>
          <UBadge
            v-if="repo.projects"
            :label="`${repo.projects} ${repo.projects === 1 ? 'project' : 'projects'}`"
            color="primary"
            variant="subtle"
            size="sm"
            class="ms-auto"
          />
          <span v-else class="ms-auto text-xs text-dimmed">Unused</span>
        </div>
      </div>
    </template>

    <!-- not connected -->
    <div v-else class="flex flex-col items-center gap-3 rounded-md border border-dashed border-default py-10">
      <UIcon name="i-lucide-github" class="size-8 text-dimmed" />
      <p class="max-w-sm text-center text-sm text-muted">
        stacker needs the GitHub App to read your repositories and receive push events.
      </p>
      <UButton
        label="Install GitHub App"
        icon="i-lucide-github"
        class="shadow-lg shadow-primary/20"
        @click="connect"
      />
    </div>
  </SettingsSection>

  <SettingsSection
    title="Other providers"
    description="GitLab and Bitbucket connections are not available yet."
  >
    <div class="grid gap-3 sm:grid-cols-2">
      <div
        v-for="provider in [
          { name: 'GitLab', icon: 'i-lucide-git-branch' },
          { name: 'Bitbucket', icon: 'i-lucide-git-merge' }
        ]"
        :key="provider.name"
        class="flex items-center gap-3 rounded-md border border-default p-3"
      >
        <UIcon :name="provider.icon" class="size-4 text-dimmed" />
        <span class="text-sm text-toned">{{ provider.name }}</span>
        <UBadge label="Soon" color="neutral" variant="subtle" size="sm" class="ms-auto" />
      </div>
    </div>
  </SettingsSection>
</template>
