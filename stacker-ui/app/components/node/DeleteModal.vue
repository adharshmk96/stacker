<script setup lang="ts">
import type { Node } from '~/types/node'

const props = defineProps<{ node?: Node | null }>()
const emit = defineEmits<{ deleted: [string] }>()

const open = defineModel<boolean>('open', { default: false })

const { remove } = useNodes()
const toast = useToast()

const deleting = ref(false)

async function onConfirm() {
  if (!props.node) return

  deleting.value = true

  try {
    const { id, name } = props.node
    await remove(id)
    toast.add({ title: 'Node deleted', description: name, icon: 'i-lucide-trash-2' })
    emit('deleted', id)
    open.value = false
  } catch (error) {
    toast.add({
      title: 'Could not delete node',
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
  <UModal v-model:open="open" title="Delete node">
    <template #body>
      <p class="text-sm text-muted">
        <strong class="text-highlighted">{{ node?.name }}</strong> ({{ node?.ssh }}) will be removed
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
