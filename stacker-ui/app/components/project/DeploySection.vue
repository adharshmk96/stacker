<script setup lang="ts">
import type { Environment } from '~/types/project'

/** When an environment deploys, and how the rollout behaves. Edits in place. */
const props = defineProps<{
  environment: Environment
  /** Push and tag triggers only make sense on a git source */
  isGit?: boolean
}>()

const triggerItems = computed(() => [
  { label: 'Manual only', value: 'manual', description: 'Deploys when you press the button' },
  {
    label: 'On push',
    value: 'push',
    description: 'Every push to the environment branch',
    disabled: !props.isGit
  },
  {
    label: 'On tag',
    value: 'tag',
    description: 'Pushes of a tag matching a pattern',
    disabled: !props.isGit
  },
  { label: 'On a schedule', value: 'schedule', description: 'A cron expression, in UTC' }
])

const isSchedule = computed(() => props.environment.trigger.kind === 'schedule')

const strategyItems = [
  {
    label: 'Rolling',
    value: 'rolling',
    description: 'Replace tasks one at a time — no downtime'
  },
  {
    label: 'Recreate',
    value: 'recreate',
    description: 'Stop everything, then start the new tasks'
  }
]
</script>

<template>
  <div class="space-y-8">
    <div>
      <h3 class="mb-1 text-xs font-semibold uppercase tracking-[0.08em] text-dimmed">
        When to deploy
      </h3>
      <p class="mb-3 text-sm text-muted">
        What puts <span class="font-mono">{{ environment.name }}</span> in the deployment queue.
      </p>

      <URadioGroup
        v-model="environment.trigger.kind"
        :items="triggerItems"
        value-key="value"
        variant="card"
        :ui="{ fieldset: 'grid sm:grid-cols-2 gap-2' }"
      />

      <UFormField
        v-if="environment.trigger.kind === 'tag' || isSchedule"
        :label="isSchedule ? 'Cron expression' : 'Tag pattern'"
        :help="isSchedule
          ? 'Five fields — minute, hour, day of month, month, day of week — in UTC. Ranges, lists and */n are supported.'
          : 'A glob matched against the pushed tag. Blank matches every tag.'"
        class="mt-3"
      >
        <UInput
          v-model="environment.trigger.pattern"
          :placeholder="isSchedule ? '0 3 * * *' : 'v*'"
          class="w-full font-mono sm:w-64"
        />
      </UFormField>
    </div>

    <div>
      <h3 class="mb-1 text-xs font-semibold uppercase tracking-[0.08em] text-dimmed">
        Rollout
      </h3>
      <p class="mb-3 text-sm text-muted">How the new version replaces the running one.</p>

      <URadioGroup
        v-model="environment.deploy.strategy"
        :items="strategyItems"
        value-key="value"
        variant="card"
        :ui="{ fieldset: 'grid sm:grid-cols-2 gap-2' }"
      />

      <div class="mt-4 grid gap-4 sm:grid-cols-2">
        <UFormField label="Replicas" hint="Per service">
          <UInput
            v-model.number="environment.deploy.replicas"
            type="number"
            min="1"
            class="w-full font-mono"
          />
        </UFormField>

        <UFormField label="Health check grace" hint="Seconds">
          <UInput
            v-model.number="environment.deploy.healthGraceSec"
            type="number"
            min="0"
            class="w-full font-mono"
          />
        </UFormField>

        <UFormField
          label="Placement constraint"
          hint="Optional"
          class="sm:col-span-2"
          description="Where the tasks may run — blank means anywhere in the swarm."
        >
          <UInput
            v-model="environment.deploy.placement"
            placeholder="node.labels.tier==edge"
            class="w-full font-mono"
          />
        </UFormField>
      </div>

      <div class="mt-4 space-y-3 border-t border-default pt-4">
        <USwitch
          v-model="environment.deploy.autoRollback"
          label="Roll back automatically"
          description="Restore the previous version when the new tasks fail their health check."
        />
        <USwitch
          v-model="environment.deploy.alwaysPull"
          label="Always pull images"
          description="Fetch the image again even when the tag has not changed."
        />
      </div>
    </div>
  </div>
</template>
