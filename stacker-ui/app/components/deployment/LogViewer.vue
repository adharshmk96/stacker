<script setup lang="ts">
import type { Deployment } from '~/types/deployment'
import { isLive } from '~/types/deployment'

/**
 * A run's output, followed live.
 *
 * Lines are appended from the cursor the server hands back, so a poll only
 * carries what is new. The view sticks to the bottom while the run is going,
 * unless the reader has scrolled up — scrolling away from the tail is how
 * someone reads a failure, and yanking them back would make that impossible.
 */
const props = defineProps<{ deployment: Deployment }>()

const { logs, refresh } = useDeployments()

const lines = ref<string[]>([])
const status = ref(props.deployment.status)
const failed = ref<string | null>(null)

let cursor = 0

const viewport = useTemplateRef<HTMLElement>('viewport')
const pinned = ref(true)

/** Within a few pixels of the bottom counts as "still following". */
function onScroll() {
  const element = viewport.value
  if (!element) return
  pinned.value = element.scrollHeight - element.scrollTop - element.clientHeight < 24
}

async function pull() {
  try {
    const chunk = await logs(props.deployment.id, cursor)
    failed.value = null
    status.value = chunk.status

    if (chunk.lines.length) {
      lines.value = [...lines.value, ...chunk.lines]
      cursor = chunk.next
      if (pinned.value) {
        await nextTick()
        const element = viewport.value
        if (element) element.scrollTop = element.scrollHeight
      }
    } else {
      cursor = chunk.next
    }

    // The run just finished: the list holds a stale row until it is re-read.
    if (chunk.done && isLive(props.deployment.status)) await refresh()
  } catch (error: any) {
    failed.value = error?.message ?? 'Could not read the log'
  }
}

const { tick } = useLivePoll(() => {
  // A finished run's log is complete, so there is nothing to poll for.
  if (status.value !== 'queued' && status.value !== 'running' && lines.value.length) return
  return pull()
}, 2000)

/**
 * The run's own step markers are written as `==>` and `-->`, so they are the one
 * thing worth colouring: they turn a wall of docker output into something with
 * headings.
 */
function lineClass(line: string) {
  if (line.startsWith('==> failed') || line.startsWith('==> timed out')) return 'text-error'
  if (line.startsWith('==> succeeded')) return 'text-success'
  if (line.startsWith('==>')) return 'text-primary'
  if (line.startsWith('-->')) return 'text-muted'
  return 'text-toned'
}

// Opening a different run starts over rather than appending to the last one.
watch(() => props.deployment.id, () => {
  lines.value = []
  cursor = 0
  status.value = props.deployment.status
  pinned.value = true
  void tick()
})
</script>

<template>
  <div class="flex min-h-0 flex-col gap-3">
    <div class="flex flex-wrap items-center gap-2 text-sm">
      <UBadge :label="status" :color="deploymentStatusColor[status]" variant="subtle" />
      <span class="font-mono text-xs text-dimmed">#{{ deployment.number }}</span>
      <span class="font-mono text-xs text-toned">{{ deployment.environment }}</span>
      <span class="font-mono text-xs text-dimmed">{{ deployment.revision }}</span>
      <span class="min-w-0 flex-1 truncate text-muted">{{ deployment.message }}</span>
      <span v-if="!pinned" class="text-xs text-dimmed">paused — scroll down to follow</span>
    </div>

    <UAlert
      v-if="failed"
      :description="failed"
      icon="i-lucide-triangle-alert"
      color="error"
      variant="subtle"
    />

    <div
      ref="viewport"
      class="max-h-[60vh] min-h-48 flex-1 overflow-y-auto rounded-md border border-default bg-elevated/30 p-3"
      @scroll="onScroll"
    >
      <p v-if="!lines.length" class="font-mono text-xs text-dimmed">
        {{ status === 'queued' ? 'Waiting for the run to start…' : 'No output.' }}
      </p>

      <pre
        v-for="(line, index) in lines"
        :key="index"
        class="whitespace-pre-wrap break-all font-mono text-[11px] leading-relaxed"
        :class="lineClass(line)"
      >{{ line }}</pre>
    </div>

    <p v-if="deployment.error" class="text-sm text-error">{{ deployment.error }}</p>
  </div>
</template>
