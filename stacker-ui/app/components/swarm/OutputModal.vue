<script setup lang="ts">
/**
 * Shows what a read-only action came back with — logs, an inspect dump, a
 * config's content.
 *
 * Docker's output is plain text of unpredictable width and length, so it gets a
 * scroll box of its own rather than being wrapped into prose, and a copy button
 * because the usual next step is pasting it somewhere.
 */

const props = defineProps<{ title: string, output: string }>()

const open = defineModel<boolean>('open', { default: false })

const toast = useToast()

async function copy() {
  try {
    await navigator.clipboard.writeText(props.output)
    toast.add({ title: 'Copied', icon: 'i-lucide-copy' })
  } catch {
    toast.add({ title: 'Could not copy', icon: 'i-lucide-circle-alert', color: 'error' })
  }
}
</script>

<template>
  <UModal v-model:open="open" :title="title" :ui="{ content: 'max-w-4xl' }">
    <template #body>
      <pre class="max-h-[60vh] overflow-auto rounded-lg border border-default bg-elevated/40 p-3 font-mono text-xs leading-relaxed text-toned">{{ output }}</pre>
    </template>

    <template #footer>
      <div class="flex w-full justify-end gap-2">
        <UButton label="Copy" icon="i-lucide-copy" color="neutral" variant="subtle" @click="copy" />
        <UButton label="Close" color="neutral" variant="ghost" @click="open = false" />
      </div>
    </template>
  </UModal>
</template>
