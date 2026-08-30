<script setup lang="ts">
import type { ProjectPayload } from '~/types/project'

/**
 * Source fields — the repository or the pasted compose file.
 *
 * Every section component edits the page's draft in place rather than emitting
 * a copy back: the draft is a plain reactive clone of the project, and the page
 * decides when it is saved.
 */
const props = defineProps<{ draft: ProjectPayload }>()

const sourceItems = [
  {
    label: 'Git provider',
    value: 'git',
    icon: 'i-lucide-git-branch',
    description: 'Pull the compose file from a repository'
  },
  {
    label: 'Compose file',
    value: 'compose',
    icon: 'i-lucide-file-code',
    description: 'Paste the YAML directly'
  }
] as const

const providerItems = [
  { label: 'GitHub', value: 'github', icon: 'i-lucide-github' },
  { label: 'GitLab', value: 'gitlab', icon: 'i-lucide-gitlab' },
  { label: 'Bitbucket', value: 'bitbucket', icon: 'i-lucide-git-pull-request' },
  { label: 'Gitea', value: 'gitea', icon: 'i-lucide-git-fork' }
]

const isGit = computed(() => props.draft.sourceKind === 'git')

/* ---- repository picker ---- */

/**
 * The repository is chosen from the connected GitHub App's installation when
 * there is one, and typed in when there is not.
 *
 * It stays a combobox rather than a plain select in either case: the field also
 * accepts a clone URL for a provider stacker cannot enumerate, and a repository
 * that was added to the installation since this list was fetched must not be
 * unreachable until someone reloads the page.
 */
const github = useGitHub()

onMounted(() => {
  // Only GitHub can be enumerated, so nothing is fetched for other providers.
  if (props.draft.git.provider === 'github') void github.load(true)
})

// Loaded lazily, so switching the provider to GitHub after mount still fills in.
watch(() => props.draft.git.provider, (provider) => {
  if (provider === 'github') void github.load(true)
})

const repositoryItems = computed(() => {
  const items = github.repositories.value.map(repo => ({
    label: repo.fullName,
    value: repo.fullName,
    icon: repo.private ? 'i-lucide-lock' : 'i-lucide-book-marked',
    branch: repo.defaultBranch
  }))

  // A repository already on the project but absent from the list — renamed,
  // removed from the installation, or another provider entirely — is added so
  // the field shows what is actually configured instead of looking empty.
  const current = props.draft.git.repo.trim()
  if (current && !items.some(item => item.value === current)) {
    items.unshift({ label: current, value: current, icon: 'i-lucide-git-branch', branch: '' })
  }
  return items
})

/** The picker is only worth showing once there is something to pick. */
const canPickRepository = computed(() =>
  props.draft.git.provider === 'github' && github.repositories.value.length > 0)

/**
 * GitHub is selected but nothing is connected. Worth saying so: a repository
 * typed in by hand still deploys if it is public, and the reason a private one
 * fails to clone is exactly this.
 */
const needsConnection = computed(() =>
  props.draft.git.provider === 'github'
  && !github.pending.value
  && !github.app.value?.installationId)

/**
 * Picking a repository fills in its default branch, which is the point of
 * reading the list — a repository still on `master` would otherwise fail its
 * first clone against the `main` the form defaults to.
 *
 * An edited branch is left alone: the whole reason branch is a field is that a
 * project may deploy something other than the default.
 */
function onRepositoryChange(value: string) {
  const picked = repositoryItems.value.find(item => item.value === value)
  if (!picked?.branch) return

  const branch = props.draft.git.branch.trim()
  if (branch === '' || branch === 'main' || branch === autofilledBranch) {
    props.draft.git.branch = picked.branch
    autofilledBranch = picked.branch
  }
}

// Remembers what was filled in, so picking a second repository replaces a branch
// this component chose but never one the user typed.
let autofilledBranch = ''

/** Free text typed into the combobox is accepted as-is. */
function onRepositoryCreate(value: string) {
  props.draft.git.repo = value.trim()
  onRepositoryChange(props.draft.git.repo)
}
</script>

<template>
  <div class="space-y-5">
    <div class="grid gap-3 sm:grid-cols-2">
      <button
        v-for="item in sourceItems"
        :key="item.value"
        type="button"
        class="flex items-start gap-3 rounded-lg border p-3 text-left transition-colors"
        :class="draft.sourceKind === item.value
          ? 'border-primary bg-primary/5'
          : 'border-default hover:bg-elevated/40'"
        @click="draft.sourceKind = item.value"
      >
        <UIcon
          :name="item.icon"
          class="mt-0.5 size-4.5"
          :class="draft.sourceKind === item.value ? 'text-primary' : 'text-dimmed'"
        />
        <span class="leading-tight">
          <span class="block text-sm font-medium text-highlighted">{{ item.label }}</span>
          <span class="block text-xs text-muted">{{ item.description }}</span>
        </span>
      </button>
    </div>

    <div v-if="isGit" class="grid gap-4 sm:grid-cols-2">
      <UFormField label="Provider" name="git.provider">
        <USelectMenu
          v-model="draft.git.provider"
          :items="providerItems"
          value-key="value"
          class="w-full"
        />
      </UFormField>

      <UFormField
        label="Repository"
        name="git.repo"
        required
        :help="canPickRepository
          ? 'From your connected GitHub App. Type to search, or enter any clone URL.'
          : undefined"
      >
        <USelectMenu
          v-if="canPickRepository"
          v-model="draft.git.repo"
          :items="repositoryItems"
          value-key="value"
          create-item
          icon="i-lucide-github"
          placeholder="Select a repository"
          class="w-full font-mono"
          @update:model-value="onRepositoryChange"
          @create="onRepositoryCreate"
        />

        <!-- No connection to read, so the field is what it always was. -->
        <UInput
          v-else
          v-model="draft.git.repo"
          placeholder="acme/storefront"
          class="w-full font-mono"
          :loading="github.pending.value && draft.git.provider === 'github'"
        />

        <template v-if="needsConnection" #help>
          <span class="text-dimmed">
            Public repositories work as typed.
            <NuxtLink to="/dashboard/settings/git-provider" class="text-primary hover:underline">
              Connect GitHub
            </NuxtLink>
            to pick from a list and deploy private ones.
          </span>
        </template>
      </UFormField>

      <UFormField label="Branch" name="git.branch" required hint="Default for all environments">
        <UInput
          v-model="draft.git.branch"
          icon="i-lucide-git-branch"
          placeholder="main"
          class="w-full font-mono"
        />
      </UFormField>

      <UFormField label="Path to compose file" name="git.composePath" required>
        <UInput
          v-model="draft.git.composePath"
          icon="i-lucide-file-code"
          placeholder="docker-compose.yml"
          class="w-full font-mono"
        />
      </UFormField>
    </div>

    <UFormField v-else label="Compose file" name="compose" required>
      <UTextarea
        v-model="draft.compose"
        :rows="14"
        placeholder="services:&#10;  web:&#10;    image: nginx:alpine"
        class="w-full font-mono"
        :ui="{ base: 'text-xs' }"
      />
    </UFormField>
  </div>
</template>
