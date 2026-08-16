<script setup lang="ts">
import type { Node, ProvisionJob, StepState } from '~/types/node'

/**
 * Configures a remote node in one click: check the host, install docker if it
 * is missing, then join it to the installer-created swarm.
 *
 * Opening it starts the run — that is the single click the feature is built
 * around — unless one is already going, in which case it attaches to it. The
 * run happens on the server and can take minutes, so this polls the checklist
 * rather than waiting on one request. Closing the modal cancels nothing: the
 * run carries on and the checklist is still there when the modal is reopened.
 */

const props = defineProps<{ node?: Node | null }>()
const emit = defineEmits<{ configured: [Node] }>()

const open = defineModel<boolean>('open', { default: false })

const { configureSwarm, provisionStatus, load } = useNodes()
const toast = useToast()

/**
 * A node that already holds a role is being *re*-configured, which is a
 * different thing to explain: the run is a repair, and on a healthy host it
 * changes nothing because every step re-checks before it acts.
 */
const isRerun = computed(() => !!props.node && props.node.swarmRole !== 'none')

const title = computed(() => {
  if (isRerun.value) return 'Reconfigure node'
  return 'Configure node'
})

const job = ref<ProvisionJob | null>(null)
const starting = ref(false)
const running = computed(() => job.value?.state === 'running')

let poller: ReturnType<typeof setTimeout> | null = null

function stopPolling() {
  if (poller) clearTimeout(poller)
  poller = null
}

// Reopening shows whatever the last run left behind, including one still going,
// so a user who closed the modal mid-install can look again.
watch(open, async (value) => {
  if (!value) {
    stopPolling()
    return
  }

  job.value = null
  if (!props.node) return

  try {
    job.value = await provisionStatus(props.node)
  } catch {
    // A node that has never been configured has no run to report, which is
    // the normal first-time case and not worth surfacing.
  }

  // Attach to a run already in flight rather than starting a second one;
  // otherwise this click is the one that starts it.
  if (running.value) {
    poll()
    return
  }
  await onStart()
})

onBeforeUnmount(stopPolling)

/**
 * Polls until the run settles. A failed poll is not fatal — the next tick
 * usually succeeds, and giving up would strand the checklist mid-run.
 */
function poll() {
  stopPolling()

  poller = setTimeout(async () => {
    if (!props.node) return

    try {
      job.value = await provisionStatus(props.node)
    } catch {
      // Keep the last known checklist and try again.
    }

    if (running.value) {
      poll()
      return
    }
    await onFinished()
  }, 1500)
}

async function onFinished() {
  // The node's swarm role changed on the server; pull the row back so the
  // table behind the modal matches.
  await load(true)

  const current = job.value
  if (!current) return

  if (current.state === 'succeeded') {
    toast.add({
      title: 'Node configured',
      description: current.message,
      icon: 'i-lucide-boxes',
      color: 'success'
    })
    if (props.node) emit('configured', props.node)
    return
  }

  toast.add({
    title: 'Could not configure the node',
    description: current.message,
    icon: 'i-lucide-circle-alert',
    color: 'error'
  })
}

async function onStart() {
  if (!props.node) return

  starting.value = true

  try {
    job.value = await configureSwarm(props.node)
    poll()
  } catch (error) {
    toast.add({
      title: 'Could not start configuring',
      description: error instanceof Error ? error.message : undefined,
      icon: 'i-lucide-circle-alert',
      color: 'error'
    })
  } finally {
    starting.value = false
  }
}

const stepMeta: Record<StepState, { icon: string, class: string, spin?: boolean }> = {
  pending: { icon: 'i-lucide-circle-dashed', class: 'text-dimmed' },
  running: { icon: 'i-lucide-loader-circle', class: 'text-primary', spin: true },
  done: { icon: 'i-lucide-circle-check', class: 'text-success' },
  skipped: { icon: 'i-lucide-circle-minus', class: 'text-dimmed' },
  warned: { icon: 'i-lucide-triangle-alert', class: 'text-warning' },
  failed: { icon: 'i-lucide-circle-x', class: 'text-error' }
}

/** Label for the primary button, which doubles as the retry after a failure. */
const actionLabel = computed(() => {
  if (running.value) return 'Configuring…'
  if (job.value?.state === 'failed') return 'Try again'
  if (isRerun.value) return 'Reconfigure'
  return 'Configure node'
})
</script>

<template>
  <UModal v-model:open="open" :title="title">
    <template #body>
      <div class="space-y-4">
        <p v-if="isRerun" class="text-sm text-muted">
          <strong class="text-highlighted">{{ node?.name }}</strong> is already a
          {{ node?.swarmRole }}, so this re-runs the same checks: anything still in place is
          skipped, and anything missing — docker, the daemon, swarm membership — is put back.
        </p>
        <p v-else class="text-sm text-muted">
          <strong class="text-highlighted">{{ node?.name }}</strong> will be checked over SSH, given
          docker if it is missing, and joined to the swarm as a worker.
        </p>

        <!-- What runs on the host, shown before it runs. -->
        <div class="rounded-md border border-default bg-elevated/40 p-3">
          <p class="text-xs text-dimmed">
            Docker is installed with the official script, as root or via sudo:
          </p>
          <code class="mt-1 block font-mono text-xs text-toned">
            curl -fsSL https://get.docker.com | sh
          </code>
        </div>

        <UAlert
          v-if="node?.keyStatus !== 'ok'"
          title="Connection not verified"
          description="Stacker has not confirmed key authentication for this node. Test the connection first."
          icon="i-lucide-triangle-alert"
          color="warning"
          variant="subtle"
        />

        <!-- The checklist, live while the run is going. -->
        <ul v-if="job" class="space-y-2 rounded-md border border-default p-3">
          <li v-for="step in job.steps" :key="step.key" class="flex items-start gap-3">
            <UIcon
              :name="stepMeta[step.state].icon"
              class="mt-0.5 size-4 shrink-0"
              :class="[stepMeta[step.state].class, stepMeta[step.state].spin && 'animate-spin']"
            />
            <div class="min-w-0 leading-tight">
              <p
                class="text-sm"
                :class="step.state === 'pending' ? 'text-dimmed' : 'text-highlighted'"
              >
                {{ step.title }}
              </p>
              <p v-if="step.detail" class="break-words text-xs text-muted">
                {{ step.detail }}
              </p>
            </div>
          </li>
        </ul>

        <UAlert
          v-if="job && job.state === 'failed'"
          :description="job.message"
          title="Configuration failed"
          icon="i-lucide-circle-alert"
          color="error"
          variant="subtle"
        />

        <p v-if="running" class="text-xs text-dimmed">
          Installing docker can take a few minutes. You can close this — the run carries on.
        </p>
      </div>
    </template>

    <template #footer>
      <div class="flex w-full justify-end gap-2">
        <UButton
          :label="running ? 'Close' : 'Cancel'"
          color="neutral"
          variant="ghost"
          @click="open = false"
        />
        <UButton
          v-if="job?.state !== 'succeeded'"
          :label="actionLabel"
          icon="i-lucide-boxes"
          :loading="starting || running"
          :disabled="running"
          @click="onStart"
        />
        <UButton v-else label="Done" icon="i-lucide-check" @click="open = false" />
      </div>
    </template>
  </UModal>
</template>
