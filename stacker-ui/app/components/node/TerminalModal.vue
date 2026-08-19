<script setup lang="ts">
import '@xterm/xterm/css/xterm.css'
import type { Node } from '~/types/node'

/**
 * An interactive shell on a node, in the browser.
 *
 * The session lives for as long as this modal is open: closing it closes the
 * websocket, which ends the shell. That is deliberate — a shell nobody is
 * watching is a shell still holding an ssh connection open.
 */

const props = defineProps<{ node?: Node | null }>()

const open = defineModel<boolean>('open', { default: false })

const { status, error, open: openTerminal, reconnect, dispose } = useNodeTerminal()

const host = useTemplateRef<HTMLElement>('host')

const statusMeta: Record<typeof status.value, { label: string, color: 'neutral' | 'success' | 'error' }> = {
  connecting: { label: 'Connecting…', color: 'neutral' },
  open: { label: 'Connected', color: 'success' },
  closed: { label: 'Disconnected', color: 'neutral' },
  error: { label: 'Failed', color: 'error' }
}

const target = computed(() =>
  props.node?.local ? 'this machine' : `${props.node?.ssh}:${props.node?.port}`)

// The container only exists once the modal's content is mounted, so the
// terminal is built after that tick rather than on the open flag alone.
watch(open, async (isOpen) => {
  if (!isOpen) {
    dispose()
    return
  }

  await nextTick()
  if (host.value && props.node) openTerminal(host.value, props.node)
})

onBeforeUnmount(dispose)
</script>

<template>
  <UModal
    v-model:open="open"
    :title="`Terminal — ${node?.name ?? ''}`"
    :description="`Shell on ${target}`"
    :ui="{ content: 'max-w-5xl' }"
  >
    <template #body>
      <div class="space-y-3">
        <div class="flex items-center justify-between gap-2">
          <UBadge
            :label="statusMeta[status].label"
            :color="statusMeta[status].color"
            variant="subtle"
            :icon="status === 'open' ? 'i-lucide-terminal' : 'i-lucide-plug-zap'"
          />

          <UButton
            v-if="status === 'closed' || status === 'error'"
            label="Reconnect"
            icon="i-lucide-refresh-cw"
            size="xs"
            color="neutral"
            variant="subtle"
            @click="node && reconnect(node)"
          />
        </div>

        <UAlert
          v-if="error"
          :description="error"
          title="Could not open the shell"
          icon="i-lucide-triangle-alert"
          color="error"
          variant="subtle"
        />

        <div
          ref="host"
          class="h-[60vh] overflow-hidden rounded-lg border border-default bg-black p-2"
        />
      </div>
    </template>

    <template #footer>
      <div class="flex w-full items-center justify-between gap-2">
        <p class="text-xs text-dimmed">
          Closing this window ends the session.
        </p>
        <UButton label="Close" color="neutral" variant="ghost" @click="open = false" />
      </div>
    </template>
  </UModal>
</template>
