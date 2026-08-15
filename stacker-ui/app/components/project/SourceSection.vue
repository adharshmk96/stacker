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

      <UFormField label="Repository" name="git.repo" required>
        <UInput v-model="draft.git.repo" placeholder="acme/storefront" class="w-full font-mono" />
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
