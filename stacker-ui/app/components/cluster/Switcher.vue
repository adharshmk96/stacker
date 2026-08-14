<script setup lang="ts">
import type { DropdownMenuItem } from '@nuxt/ui'

defineProps<{ collapsed?: boolean }>()

const { items: clusters, active, activeId, select } = useClusters()

// One static cluster today, so everything below "Clusters" is disabled — the
// shape is here, the actions land when clusters become real.
const items = computed<DropdownMenuItem[][]>(() => [
  [{ label: 'Clusters', type: 'label' }],
  clusters.value.map(cluster => ({
    label: cluster.name,
    icon: 'i-lucide-boxes',
    type: 'checkbox' as const,
    checked: cluster.id === activeId.value,
    onUpdateChecked: (checked: boolean) => {
      if (checked) select(cluster.id)
    },
    // Without this, re-picking the active cluster would uncheck it.
    onSelect: (event: Event) => event.preventDefault()
  })),
  [
    { label: 'New cluster', icon: 'i-lucide-plus', disabled: true },
    { label: 'Manage clusters', icon: 'i-lucide-settings-2', disabled: true }
  ]
])
</script>

<template>
  <UDropdownMenu
    :items="items"
    :content="{ align: 'start', side: 'bottom' }"
    :ui="{ content: 'w-56' }"
  >
    <UButton
      color="neutral"
      variant="ghost"
      class="w-full"
      :class="collapsed ? undefined : 'justify-start'"
      :block="collapsed"
      :aria-label="`Cluster: ${active.name}`"
      :title="collapsed ? active.name : undefined"
    >
      <UIcon name="i-lucide-boxes" class="size-4 shrink-0 text-primary" />

      <span v-if="!collapsed" class="min-w-0 flex-1 text-left leading-tight">
        <span class="block truncate text-[13px] font-medium text-highlighted">
          {{ active.name }}
        </span>
        <span class="block font-mono text-[10px] text-dimmed">cluster</span>
      </span>

      <UIcon
        v-if="!collapsed"
        name="i-lucide-chevrons-up-down"
        class="size-4 shrink-0 text-dimmed"
      />
    </UButton>
  </UDropdownMenu>
</template>
