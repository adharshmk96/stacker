<script setup lang="ts">
/** Container registries images are pulled from. Placeholder data only. */
const registries = ref([
  { id: 'r1', name: 'Docker Hub', host: 'docker.io', username: 'adharshmk96', default: true },
  { id: 'r2', name: 'GitHub Packages', host: 'ghcr.io', username: 'adharshmk96', default: false }
])
</script>

<template>
  <SettingsSection
    title="Container registries"
    description="Credentials stacker uses to pull private images onto your nodes."
  >
    <template #header-right>
      <UButton label="Add registry" icon="i-lucide-plus" size="sm" disabled />
    </template>

    <div
      v-for="registry in registries"
      :key="registry.id"
      class="flex items-center gap-3 border-t border-default py-3 first:border-t-0"
    >
      <div class="flex size-8 items-center justify-center rounded-md bg-elevated ring-1 ring-default">
        <UIcon name="i-lucide-container" class="size-4 text-primary" />
      </div>
      <div class="leading-tight">
        <p class="text-sm font-medium text-highlighted">{{ registry.name }}</p>
        <p class="font-mono text-xs text-dimmed">{{ registry.host }} · {{ registry.username }}</p>
      </div>
      <UBadge
        v-if="registry.default"
        label="Default"
        color="primary"
        variant="subtle"
        size="sm"
        class="ms-auto"
      />
      <UButton
        icon="i-lucide-trash-2"
        color="error"
        variant="ghost"
        size="sm"
        :class="registry.default ? undefined : 'ms-auto'"
        aria-label="Remove registry"
        disabled
      />
    </div>
  </SettingsSection>
</template>
