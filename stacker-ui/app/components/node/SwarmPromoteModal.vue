<script setup lang="ts">
import type { Node } from '~/types/node'

/**
 * Confirms promoting a worker to a manager.
 *
 * Promotion is not just "more capable" — managers form a raft quorum, so the
 * count matters and so does reachability. An even number of managers loses the
 * swarm as soon as one drops, and a manager the others cannot reach on 2377
 * takes the quorum down with it. Docker accepts the promotion either way and
 * the damage surfaces later, so the warning belongs here, before the click.
 */

const props = defineProps<{
  node?: Node | null
  /** Managers before this promotion */
  managerCount: number
}>()

const open = defineModel<boolean>('open', { default: false })

const { promoteSwarm } = useNodes()
const toast = useToast()

const submitting = ref(false)

/** Raft needs a majority, so an even number of managers is never the right target. */
const wouldBeEven = computed(() => (props.managerCount + 1) % 2 === 0)

async function onConfirm() {
  if (!props.node) return

  submitting.value = true

  try {
    const result = await promoteSwarm(props.node)
    toast.add({
      title: 'Node promoted',
      description: result.message,
      icon: 'i-lucide-crown',
      color: 'success'
    })
    open.value = false
  } catch (error) {
    toast.add({
      title: 'Could not promote the node',
      description: error instanceof Error ? error.message : undefined,
      icon: 'i-lucide-circle-alert',
      color: 'error'
    })
  } finally {
    submitting.value = false
  }
}
</script>

<template>
  <UModal v-model:open="open" title="Promote to manager">
    <template #body>
      <div class="space-y-4">
        <p class="text-sm text-muted">
          <strong class="text-highlighted">{{ node?.name }}</strong> will join the swarm's control
          plane, taking the swarm from {{ managerCount }} to {{ managerCount + 1 }}
          {{ managerCount + 1 === 1 ? 'manager' : 'managers' }}.
        </p>

        <UAlert
          v-if="wouldBeEven"
          title="That would be an even number of managers"
          :description="`Managers vote, so a swarm needs more than half of them online. With ${managerCount + 1}, losing one leaves no majority and the swarm stops accepting changes. Keep an odd number — 1, 3 or 5.`"
          icon="i-lucide-triangle-alert"
          color="warning"
          variant="subtle"
        />

        <UAlert
          title="This node must be reachable by the other managers"
          description="Managers talk to each other on port 2377. If they cannot reach this one, the swarm loses its leader and needs manual recovery."
          icon="i-lucide-network"
          color="neutral"
          variant="subtle"
        />
      </div>
    </template>

    <template #footer>
      <div class="flex w-full justify-end gap-2">
        <UButton label="Cancel" color="neutral" variant="ghost" @click="open = false" />
        <UButton
          label="Promote"
          icon="i-lucide-crown"
          :color="wouldBeEven ? 'warning' : 'primary'"
          :loading="submitting"
          @click="onConfirm"
        />
      </div>
    </template>
  </UModal>
</template>
