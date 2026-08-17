<script setup lang="ts">
import type { Deployment } from '~/types/deployment'
import type { Project, ProjectStatus } from '~/types/project'

/**
 * Read-only summary of a project: source, what each environment is running right
 * now, and recent activity.
 *
 * The status comes in as a prop rather than being fetched here, so the whole page
 * reads one poll instead of every section running its own.
 */
const props = defineProps<{
  project: Project
  status: ProjectStatus | null
  /** This project's deployments, newest first */
  deployments: Deployment[]
  /** Environment ids with a run in flight — their buttons are disabled */
  busy?: string[]
}>()

const emit = defineEmits<{
  deploy: [environmentId: string]
  stop: [environmentId: string]
  logs: [deployment: Deployment]
}>()

const triggerLabel = (envId: string) => {
  const env = props.project.environments.find(item => item.id === envId)
  if (!env) return ''

  switch (env.trigger.kind) {
    case 'push': return `push to ${env.branch || props.project.git.branch}`
    case 'tag': return `tag ${env.trigger.pattern || '*'}`
    case 'schedule': return `cron ${env.trigger.pattern || '—'}`
    default: return 'manual'
  }
}

/** Live status per environment id, for the rows. */
const statusOf = (envId: string) =>
  props.status?.environments.find(item => item.environmentId === envId) ?? null

const isBusy = (envId: string) => props.busy?.includes(envId) ?? false

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
        Stored compose file · {{ project.compose.split('\n').length }} lines
      </p>
    </section>

    <!-- environments -->
    <section class="rounded-lg border border-default bg-default/60 p-5 backdrop-blur">
      <h2 class="mb-4 text-sm font-semibold text-highlighted">Environments</h2>

      <div
        v-for="env in project.environments"
        :key="env.id"
        class="border-t border-default py-3 first:border-t-0 first:pt-0"
      >
        <div class="flex flex-wrap items-center gap-x-4 gap-y-2">
          <span class="flex w-32 items-center gap-2 font-mono text-sm text-highlighted">
            <UIcon name="i-lucide-layers" class="size-4 text-primary" />
            {{ env.name }}
          </span>

          <span class="min-w-0 flex-1 truncate font-mono text-xs text-dimmed">
            <template v-if="env.domains.length">
              <a
                v-for="domain in env.domains"
                :key="domain.id"
                :href="`${domain.tls === 'none' ? 'http' : 'https'}://${domain.host}`"
                target="_blank"
                rel="noopener"
                class="mr-2 hover:text-primary"
              >{{ domain.host }}</a>
            </template>
            <template v-else>no domain</template>
          </span>

          <UBadge
            :label="triggerLabel(env.id)"
            color="neutral"
            variant="subtle"
            class="font-mono text-[11px]"
          />

          <!-- What docker reports, not what the last deploy intended. -->
          <UBadge
            v-if="statusOf(env.id)"
            :label="runtimeStateLabel[statusOf(env.id)!.state]"
            :color="runtimeStateColor[statusOf(env.id)!.state]"
            variant="subtle"
          />
          <span v-else class="text-xs text-dimmed">no reading yet</span>

          <span v-if="statusOf(env.id)?.desired" class="font-mono text-xs text-toned">
            {{ statusOf(env.id)!.running }}/{{ statusOf(env.id)!.desired }} tasks
          </span>

          <UButton
            v-if="statusOf(env.id)?.state !== 'stopped'"
            label="Stop"
            icon="i-lucide-square"
            size="xs"
            color="neutral"
            variant="ghost"
            :disabled="isBusy(env.id)"
            @click="emit('stop', env.id)"
          />
          <UButton
            label="Deploy"
            icon="i-lucide-rocket"
            size="xs"
            color="neutral"
            variant="subtle"
            :loading="isBusy(env.id)"
            @click="emit('deploy', env.id)"
          />
        </div>

        <!-- per-service detail, once there is something running -->
        <div v-if="statusOf(env.id)?.services.length" class="mt-2 flex flex-wrap gap-1.5 pl-8">
          <UBadge
            v-for="service in statusOf(env.id)!.services"
            :key="service.name"
            :color="service.running === service.desired ? 'neutral' : 'error'"
            variant="outline"
            class="font-mono text-[11px]"
            :label="`${service.name} ${service.running}/${service.desired} · ${service.image}`"
          />
        </div>

        <p v-if="statusOf(env.id)?.message" class="mt-2 pl-8 text-xs text-dimmed">
          {{ statusOf(env.id)!.message }}
        </p>
      </div>
    </section>

    <!-- activity -->
    <section class="rounded-lg border border-default bg-default/60 p-5 backdrop-blur">
      <h2 class="mb-3 text-sm font-semibold text-highlighted">Recent deployments</h2>

      <p v-if="!deployments.length" class="text-sm text-dimmed">
        This project has never been deployed.
      </p>

      <button
        v-for="deployment in deployments.slice(0, 5)"
        :key="deployment.id"
        type="button"
        class="flex w-full items-center gap-3 border-t border-default py-2.5 text-left text-sm transition-colors first:border-t-0 hover:bg-elevated/40"
        @click="emit('logs', deployment)"
      >
        <UBadge
          :label="deployment.status"
          :color="deploymentStatusColor[deployment.status]"
          variant="subtle"
          size="sm"
        />
        <span class="font-mono text-xs text-dimmed">#{{ deployment.number }}</span>
        <span class="font-mono text-xs text-toned">{{ deployment.environment }}</span>
        <span class="min-w-0 flex-1 truncate text-muted">
          {{ deployment.message || deployment.error || deployment.revision }}
        </span>
        <span class="text-xs text-dimmed">{{ formatTime(deployment.startedAt) }}</span>
        <UIcon name="i-lucide-scroll-text" class="size-4 text-dimmed" />
      </button>
    </section>
  </div>
</template>
