<script setup lang="ts">
import type { SshKey } from '~/types/sshKey'

const props = defineProps<{ sshKey?: SshKey | null }>()
const emit = defineEmits<{ deleted: [string] }>()

const open = defineModel<boolean>('open', { default: false })

const { remove } = useSshKeys()
const { items: vpsItems } = useVps()
const toast = useToast()

/** Servers that would lose their credential if this key goes away */
const inUseBy = computed(() =>
  props.sshKey ? vpsItems.value.filter(vps => vps.sshKeyId === props.sshKey!.id) : [])

const deleting = ref(false)

async function onConfirm() {
  if (!props.sshKey) return

  deleting.value = true

  try {
    const { id, name } = props.sshKey
    await remove(id)
    toast.add({ title: 'SSH key deleted', description: name, icon: 'i-lucide-trash-2' })
    emit('deleted', id)
    open.value = false
  } catch (error) {
    toast.add({
      title: 'Could not delete SSH key',
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
  <UModal v-model:open="open" title="Delete SSH key">
    <template #body>
      <div class="space-y-3">
        <p class="text-sm text-muted">
          <strong class="text-highlighted">{{ sshKey?.name }}</strong> will be removed from stacker,
          including its private half. The public key stays in
          <code class="font-mono text-xs">authorized_keys</code> on any host it was installed on —
          remove it there too. This cannot be undone.
        </p>

        <div
          v-if="inUseBy.length"
          class="flex items-start gap-2 rounded-md border border-warning/30 bg-warning/5 px-3 py-2 text-sm text-warning"
        >
          <UIcon name="i-lucide-triangle-alert" class="mt-0.5 size-4 shrink-0" />
          <span>
            Used by {{ inUseBy.length }}
            {{ inUseBy.length === 1 ? 'server' : 'servers' }}:
            <span class="font-mono">{{ inUseBy.map(vps => vps.name).join(', ') }}</span>.
            They will need a new key before the next deploy.
          </span>
        </div>
      </div>
    </template>

    <template #footer>
      <div class="flex justify-end gap-2 w-full">
        <UButton label="Cancel" color="neutral" variant="ghost" @click="open = false" />
        <UButton label="Delete" color="error" :loading="deleting" @click="onConfirm" />
      </div>
    </template>
  </UModal>
</template>
