<script setup lang="ts">
/**
 * Confirms a destructive row action.
 *
 * Every remove here runs against a live swarm and none of them can be undone,
 * so the menu never fires one straight from a click.
 */

defineProps<{
  /** e.g. "Remove service" */
  title: string
  /** What is about to be removed, named */
  target: string
  /** One line on the consequence */
  description: string
  loading?: boolean
}>()

const emit = defineEmits<{ confirm: [] }>()

const open = defineModel<boolean>('open', { default: false })
</script>

<template>
  <UModal v-model:open="open" :title="title">
    <template #body>
      <p class="text-sm text-muted">
        <strong class="text-highlighted">{{ target }}</strong> {{ description }}
      </p>
    </template>

    <template #footer>
      <div class="flex w-full justify-end gap-2">
        <UButton label="Cancel" color="neutral" variant="ghost" @click="open = false" />
        <UButton :label="title" color="error" :loading="loading" @click="emit('confirm')" />
      </div>
    </template>
  </UModal>
</template>
