<script setup lang="ts">
/**
 * A compose service's container output, polled as a tail.
 *
 * Docker's own log command has no cursor to resume from, so every poll
 * re-reads the tail and replaces what is shown, unlike a deployment's log
 * which appends from where the last poll left off.
 */
const props = defineProps<{
  projectId: string
  environmentId: string
  service: string
}>()

const { serviceLogs } = useProjects()

const lines = ref<string[]>([])
const failed = ref<string | null>(null)
const loading = ref(true)

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
    const chunk = await serviceLogs(props.projectId, props.environmentId, props.service)
    failed.value = null
    lines.value = chunk.lines
    loading.value = false

    if (pinned.value) {
      await nextTick()
      const element = viewport.value
      if (element) element.scrollTop = element.scrollHeight
    }
  } catch (error: any) {
    failed.value = error?.message ?? 'Could not read the log'
    loading.value = false
  }
}

const { tick } = useLivePoll(() => pull(), 3000)

// Switching services starts over rather than showing the last one's tail
// until the next poll lands.
watch(() => [props.environmentId, props.service], () => {
  lines.value = []
  failed.value = null
  loading.value = true
  pinned.value = true
  void tick()
})
</script>

<template>
  <div class="flex min-h-0 flex-col gap-3">
    <div class="flex flex-wrap items-center gap-2 text-sm">
      <UIcon name="i-lucide-box" class="size-3.5 text-dimmed" />
      <span class="font-mono text-xs text-toned">{{ service }}</span>
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
      <p v-if="loading" class="font-mono text-xs text-dimmed">Loading…</p>
      <p v-else-if="!lines.length" class="font-mono text-xs text-dimmed">No output.</p>

      <pre
        v-for="(line, index) in lines"
        :key="index"
        class="whitespace-pre-wrap break-all font-mono text-[11px] leading-relaxed text-toned"
      >{{ line }}</pre>
    </div>
  </div>
</template>
