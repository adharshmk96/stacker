<script setup lang="ts">
import type { Node } from '~/types/node'

/**
 * The swarm at a glance: managers on top, workers below, joined by connectors.
 *
 * Nodes come from `/api/nodes`, where role, address and errors are already read
 * from docker; the task counts come from `/api/swarm/tasks`, which the server
 * stamps with the stacker node each task is running on.
 */

const { items, pending, load } = useNodes()
const tasks = useSwarmApi()

onMounted(() => {
  load()
  // A failure here only costs the counts, so it is never surfaced: the alert
  // for an unreadable swarm belongs to the list below, not to this summary.
  tasks.load('tasks')
})

/** Running tasks per node id — what the node is actually carrying right now. */
const taskCounts = computed(() => {
  const counts: Record<string, number> = {}
  for (const task of tasks.rows.value) {
    if (task.state !== 'running' || !task.nodeId) continue
    counts[task.nodeId] = (counts[task.nodeId] ?? 0) + 1
  }
  return counts
})

const managers = computed(() => items.value.filter(node => node.swarmRole === 'manager'))
const workers = computed(() => items.value.filter(node => node.swarmRole === 'worker'))
const unconfigured = computed(() => items.value.filter(node => node.swarmRole === 'none'))

const taskCount = (node: Node) => taskCounts.value[node.id] ?? 0

const leader = computed(() => managers.value[0])
</script>

<template>
  <section
    class="rounded-lg border border-default bg-default/60 p-5 backdrop-blur"
    aria-label="Swarm topology"
  >
    <div class="mb-5 flex items-center justify-between gap-3">
      <div>
        <h2 class="font-medium text-highlighted">Topology</h2>
        <p class="text-sm text-muted">
          {{ managers.length }} {{ managers.length === 1 ? 'manager' : 'managers' }} ·
          {{ workers.length }} {{ workers.length === 1 ? 'worker' : 'workers' }}
        </p>
      </div>

      <UButton
        label="Manage nodes"
        icon="i-lucide-server"
        color="neutral"
        variant="subtle"
        size="sm"
        to="/dashboard/nodes"
      />
    </div>

    <div v-if="pending && !items.length" class="flex justify-center py-10">
      <UIcon name="i-lucide-loader-circle" class="size-6 animate-spin text-dimmed" />
    </div>

    <div
      v-else-if="!managers.length && !workers.length"
      class="flex flex-col items-center gap-3 py-8 text-center"
    >
      <UIcon name="i-lucide-boxes" class="size-8 text-dimmed" />
      <p class="text-sm text-muted">
        No swarm yet. Enable it on the local node to create the control plane.
      </p>
      <UButton label="Go to Nodes" color="neutral" variant="subtle" to="/dashboard/nodes" />
    </div>

    <div v-else class="flex flex-col items-center">
      <!-- managers -->
      <div class="flex flex-wrap justify-center gap-4">
        <div
          v-for="node in managers"
          :key="node.id"
          class="w-56 rounded-lg border border-primary/40 bg-primary/5 p-3 ring-1 ring-inset ring-primary/10"
        >
          <div class="flex items-center gap-2">
            <UIcon name="i-lucide-crown" class="size-4 shrink-0 text-primary" />
            <p class="truncate font-medium text-highlighted">{{ node.name }}</p>
            <UBadge
              v-if="node.id === leader?.id"
              label="Leader"
              color="primary"
              variant="subtle"
              size="sm"
              class="ml-auto"
            />
          </div>
          <p class="mt-1 truncate font-mono text-xs text-dimmed">
            {{ node.swarmAddr ? `${node.swarmAddr}:2377` : 'address unknown' }}
          </p>
          <div class="mt-2 flex items-center gap-2">
            <UBadge label="Manager" color="primary" variant="subtle" size="sm" />
            <span class="font-mono text-xs text-dimmed">{{ taskCount(node) }} {{ taskCount(node) === 1 ? 'task' : 'tasks' }}</span>
            <UIcon
              v-if="node.swarmError"
              name="i-lucide-triangle-alert"
              class="size-4 text-warning"
              :title="node.swarmError"
            />
          </div>
        </div>
      </div>

      <!-- connector: manager row down to the worker row -->
      <template v-if="workers.length">
        <div class="h-6 w-px bg-default" />
        <div class="h-px w-full max-w-3xl bg-default" />
        <div class="flex w-full max-w-3xl justify-center">
          <div class="h-6 w-px bg-default" />
        </div>
      </template>

      <!-- workers -->
      <div v-if="workers.length" class="flex flex-wrap justify-center gap-4">
        <div
          v-for="node in workers"
          :key="node.id"
          class="w-56 rounded-lg border border-default bg-elevated/40 p-3"
        >
          <div class="flex items-center gap-2">
            <UIcon name="i-lucide-server" class="size-4 shrink-0 text-success" />
            <p class="truncate font-medium text-highlighted">{{ node.name }}</p>
          </div>
          <p class="mt-1 truncate font-mono text-xs text-dimmed">
            {{ node.local ? 'this machine' : node.ssh }}
          </p>
          <div class="mt-2 flex items-center gap-2">
            <UBadge label="Worker" color="success" variant="subtle" size="sm" />
            <span class="font-mono text-xs text-dimmed">{{ taskCount(node) }} {{ taskCount(node) === 1 ? 'task' : 'tasks' }}</span>
            <UIcon
              v-if="node.swarmError"
              name="i-lucide-triangle-alert"
              class="size-4 text-warning"
              :title="node.swarmError"
            />
          </div>
        </div>
      </div>

      <p v-if="unconfigured.length" class="mt-5 text-xs text-dimmed">
        {{ unconfigured.length }}
        {{ unconfigured.length === 1 ? 'node is' : 'nodes are' }}
        registered but not in the swarm.
      </p>
    </div>
  </section>
</template>
