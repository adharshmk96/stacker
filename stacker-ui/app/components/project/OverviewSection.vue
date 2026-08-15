<script setup lang="ts">
import type { Deployment } from '~/types/deployment'
import type { Project } from '~/types/project'

/** Read-only summary of a project: source, environments, latest activity. */
const props = defineProps<{
  project: Project
  /** This project's deployments, newest first */
  deployments: Deployment[]
}>()

const emit = defineEmits<{ deploy: [environmentId: string] }>()

const triggerLabel = (project: Project, envId: string) => {
  const env = project.environments.find(item => item.id === envId)
  if (!env) return ''

  switch (env.trigger.kind) {
    case 'push': return `push to ${env.branch || project.git.branch}`
    case 'tag': return `tag ${env.trigger.pattern || '*'}`
    case 'schedule': return `cron ${env.trigger.pattern || '—'}`
    default: return 'manual'
  }
}

/** Newest deployment per environment name — the row's status. */
const latest = computed(() => {
  const map = new Map<string, Deployment>()
  for (const deployment of props.deployments) {
    if (!map.has(deployment.environment)) map.set(deployment.environment, deployment)
  }
  return map
})

const formatTime = (value: string) =>
  new Date(value).toLocaleString(undefined, {
    day: 'numeric', month: 'short', hour: '2-digit', minute: '2-digit'
  })
</script>

<template>
  <div class="space-y-6">
    <!-- source -->
    <section class="rounded-lg border border-default bg-default/60 p-5 backdrop-blur">
      <h2 class="mb-4 text-sm font-semibold text-highlighted">Source</h2>

      <dl v-if="project.sourceKind === 'git'" class="grid gap-4 sm:grid-cols-3">
        <div>
          <dt class="text-xs uppercase tracking-[0.08em] text-dimmed">Repository</dt>
          <dd class="mt-1 font-mono text-sm text-toned">
            {{ project.git.provider }} · {{ project.git.repo }}
          </dd>
        </div>
        <div>
          <dt class="text-xs uppercase tracking-[0.08em] text-dimmed">Branch</dt>
          <dd class="mt-1 font-mono text-sm text-toned">{{ project.git.branch }}</dd>
        </div>
        <div>
          <dt class="text-xs uppercase tracking-[0.08em] text-dimmed">Compose file</dt>
          <dd class="mt-1 font-mono text-sm text-toned">{{ project.git.composePath }}</dd>
        </div>
      </dl>

      <p v-else class="font-mono text-sm text-toned">
        Pasted compose file · {{ project.compose.split('\n').length }} lines
      </p>
    </section>

    <!-- environments -->
    <section class="rounded-lg border border-default bg-default/60 p-5 backdrop-blur">
      <h2 class="mb-4 text-sm font-semibold text-highlighted">Environments</h2>

      <div
        v-for="env in project.environments"
        :key="env.id"
        class="flex flex-wrap items-center gap-x-4 gap-y-2 border-t border-default py-3 first:border-t-0 first:pt-0"
      >
        <span class="flex w-32 items-center gap-2 font-mono text-sm text-highlighted">
          <UIcon name="i-lucide-layers" class="size-4 text-primary" />
          {{ env.name }}
        </span>

        <span class="min-w-0 flex-1 truncate font-mono text-xs text-dimmed">
          <template v-if="env.domains.length">
            {{ env.domains.map(domain => domain.host).join(', ') }}
          </template>
          <template v-else>no domain</template>
        </span>

        <UBadge
          :label="triggerLabel(project, env.id)"
          color="neutral"
          variant="subtle"
          class="font-mono text-[11px]"
        />

        <span class="font-mono text-xs text-dimmed">
          {{ env.deploy.replicas }}× {{ env.deploy.strategy }}
        </span>

        <UBadge
          v-if="latest.get(env.name)"
          :label="latest.get(env.name)!.status"
          :color="deploymentStatusColor[latest.get(env.name)!.status]"
          variant="subtle"
        />
        <span v-else class="text-xs text-dimmed">never deployed</span>

        <UButton
          label="Deploy"
          icon="i-lucide-rocket"
          size="xs"
          color="neutral"
          variant="subtle"
          @click="emit('deploy', env.id)"
        />
      </div>
    </section>

    <!-- activity -->
    <section class="rounded-lg border border-default bg-default/60 p-5 backdrop-blur">
      <h2 class="mb-3 text-sm font-semibold text-highlighted">Recent deployments</h2>

      <p v-if="!deployments.length" class="text-sm text-dimmed">
        This project has never been deployed.
      </p>

      <div
        v-for="deployment in deployments.slice(0, 5)"
        :key="deployment.id"
        class="flex items-center gap-3 border-t border-default py-2.5 text-sm first:border-t-0"
      >
        <UBadge
          :label="deployment.status"
          :color="deploymentStatusColor[deployment.status]"
          variant="subtle"
          size="sm"
        />
        <span class="font-mono text-xs text-dimmed">#{{ deployment.number }}</span>
        <span class="font-mono text-xs text-toned">{{ deployment.environment }}</span>
        <span class="min-w-0 flex-1 truncate text-muted">{{ deployment.message }}</span>
        <span class="text-xs text-dimmed">{{ formatTime(deployment.startedAt) }}</span>
      </div>
    </section>
  </div>
</template>
