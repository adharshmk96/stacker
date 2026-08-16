<script setup lang="ts">
/**
 * Asks for the replica count before scaling a service.
 *
 * Scaling to 0 is deliberately allowed: it is how a service is parked without
 * losing its spec, and docker treats it as an ordinary target.
 */

const props = defineProps<{
  service: string
  /** The current `n/m` docker reports, used to seed the field */
  replicas: string
  loading?: boolean
}>()

const emit = defineEmits<{ confirm: [number] }>()

const open = defineModel<boolean>('open', { default: false })

const target = ref(0)

// Docker reports replicas as `running/desired`; the desired half is what the
// user is about to change.
watch(open, (isOpen) => {
  if (!isOpen) return
  const desired = Number(String(props.replicas).split('/')[1])
  target.value = Number.isFinite(desired) ? desired : 1
})
</script>

<template>
  <UModal v-model:open="open" title="Scale service">
    <template #body>
      <div class="space-y-4">
        <p class="text-sm text-muted">
          <strong class="text-highlighted">{{ service }}</strong> currently runs
          <span class="font-mono">{{ replicas }}</span> replicas.
        </p>

        <UFormField label="Replicas" help="The manager starts or stops tasks until the swarm matches this.">
          <UInput v-model.number="target" type="number" min="0" class="w-32" />
        </UFormField>
      </div>
    </template>

    <template #footer>
      <div class="flex w-full justify-end gap-2">
        <UButton label="Cancel" color="neutral" variant="ghost" @click="open = false" />
        <UButton
          label="Scale"
          icon="i-lucide-scaling"
          :loading="loading"
          :disabled="!Number.isFinite(target) || target < 0"
          @click="emit('confirm', target)"
        />
      </div>
    </template>
  </UModal>
</template>
