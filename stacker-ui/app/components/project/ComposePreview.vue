<script setup lang="ts">
import type { ProjectPayload } from '~/types/project'

const props = defineProps<{ draft: ProjectPayload }>()

const preview = computed(() => props.draft.sourceKind === 'compose'
  ? composePreview(props.draft.compose)
  : { services: [] })
</script>

<template>
  <section class="rounded-lg border border-default bg-default/60 p-5 backdrop-blur">
    <header class="mb-4">
      <h2 class="text-sm font-semibold text-highlighted">Compose preview</h2>
      <p class="text-sm text-muted">Services and deployment metadata detected from the compose file.</p>
    </header>

    <p v-if="draft.sourceKind === 'git'" class="text-sm text-dimmed">
      The compose file is read from the repository when the project deploys. Paste a compose file to preview it here.
    </p>
    <p v-else-if="preview.error" class="text-sm text-error">{{ preview.error }}</p>
    <p v-else-if="!preview.services.length" class="text-sm text-dimmed">Paste a compose file to see its services.</p>
    <div v-else class="space-y-2">
      <div
        v-for="service in preview.services"
        :key="service.name"
        class="rounded-md border border-default bg-elevated/30 px-3 py-2"
      >
        <div class="flex flex-wrap items-center gap-x-3 gap-y-1">
          <span class="font-mono text-sm font-medium text-highlighted">{{ service.name }}</span>
          <span v-if="service.image" class="font-mono text-xs text-muted">{{ service.image }}</span>
          <span v-if="service.builds" class="text-xs text-muted">builds locally</span>
          <span v-if="service.replicas !== undefined" class="text-xs text-muted">{{ service.replicas }} replicas</span>
        </div>
        <p v-if="service.ports.length" class="mt-1 font-mono text-xs text-dimmed">
          Ports: {{ service.ports.join(', ') }}
        </p>
      </div>
    </div>
  </section>
</template>
