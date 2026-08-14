<script setup lang="ts">
import type { Vps } from '~/types/vps'

const props = defineProps<{ vps?: Vps | null }>()
const emit = defineEmits<{ deleted: [string] }>()

const open = defineModel<boolean>('open', { default: false })

const { remove } = useVps()
const toast = useToast()

const deleting = ref(false)

async function onConfirm() {
  if (!props.vps) return

  deleting.value = true

  try {
    const { id, name } = props.vps
    await remove(id)
    toast.add({ title: 'VPS deleted', description: name, icon: 'i-lucide-trash-2' })
    emit('deleted', id)
    open.value = false
  } catch (error) {
    toast.add({
      title: 'Could not delete VPS',
      description: error instanceof Error ? error.message : undefined,
      icon: 'i-lucide-circle-alert',
      color: 'error'
    })
  } finally {
    deleting.value = false
  }
}
</script>

<template>
  <UModal v-model:open="open" title="Delete VPS">
    <template #body>
      <p class="text-sm text-muted">
        <strong class="text-highlighted">{{ vps?.name }}</strong> ({{ vps?.ssh }}) will be removed
        from stacker. The server itself is untouched, but any stack targeting it will need a new
        host. This cannot be undone.
      </p>
    </template>

    <template #footer>
      <div class="flex justify-end gap-2 w-full">
        <UButton label="Cancel" color="neutral" variant="ghost" @click="open = false" />
        <UButton label="Delete" color="error" :loading="deleting" @click="onConfirm" />
      </div>
    </template>
  </UModal>
</template>
