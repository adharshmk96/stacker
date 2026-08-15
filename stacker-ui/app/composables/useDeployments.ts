import type { Deployment, DeploymentStatus } from '~/types/deployment'

/**
 * Deployments — placeholder only, same story as `useProjects`: a seeded
 * in-memory list, no server behind it.
 */

const minutesAgo = (minutes: number) =>
  new Date(Date.now() - minutes * 60_000).toISOString()

const items = ref<Deployment[]>([
  {
    id: 'd-1',
    number: 48,
    projectId: 'p-storefront',
    projectName: 'storefront',
    environment: 'production',
    status: 'running',
    triggeredBy: 'tag',
    actor: 'v1.8.0',
    revision: '9f2c1ab',
    message: 'Release 1.8.0',
    startedAt: minutesAgo(2)
  },
  {
    id: 'd-2',
    number: 47,
    projectId: 'p-storefront',
    projectName: 'storefront',
    environment: 'staging',
    status: 'succeeded',
    triggeredBy: 'push',
    actor: 'adharsh',
    revision: '3ba77de',
    message: 'Cache product listings',
    startedAt: minutesAgo(35),
    finishedAt: minutesAgo(34),
    durationSec: 74
  },
  {
    id: 'd-3',
    number: 12,
    projectId: 'p-metrics',
    projectName: 'metrics',
    environment: 'production',
    status: 'failed',
    triggeredBy: 'manual',
    actor: 'adharsh',
    revision: 'compose',
    message: 'Bump prometheus to v3',
    startedAt: minutesAgo(180),
    finishedAt: minutesAgo(179),
    durationSec: 41
  },
  {
    id: 'd-4',
    number: 46,
    projectId: 'p-storefront',
    projectName: 'storefront',
    environment: 'staging',
    status: 'cancelled',
    triggeredBy: 'push',
    actor: 'adharsh',
    revision: 'ce01f4a',
    message: 'Try new base image',
    startedAt: minutesAgo(420),
    finishedAt: minutesAgo(419),
    durationSec: 18
  },
  {
    id: 'd-5',
    number: 11,
    projectId: 'p-metrics',
    projectName: 'metrics',
    environment: 'production',
    status: 'succeeded',
    triggeredBy: 'schedule',
    actor: 'nightly',
    revision: 'compose',
    message: 'Nightly redeploy',
    startedAt: minutesAgo(1500),
    finishedAt: minutesAgo(1498),
    durationSec: 96
  }
])

/** Badge colour per status, shared by the list and the project page. */
export const deploymentStatusColor: Record<DeploymentStatus, 'primary' | 'success' | 'error' | 'neutral' | 'warning'> = {
  queued: 'neutral',
  running: 'primary',
  succeeded: 'success',
  failed: 'error',
  cancelled: 'warning'
}

export function useDeployments() {
  /** Records a deployment for the "Save and deploy" flow so the list moves. */
  function enqueue(projectId: string, projectName: string, environment: string) {
    const deployment: Deployment = {
      id: `d-${crypto.randomUUID().slice(0, 8)}`,
      number: Math.max(0, ...items.value.map(item => item.number)) + 1,
      projectId,
      projectName,
      environment,
      status: 'queued',
      triggeredBy: 'manual',
      actor: 'you',
      revision: 'pending',
      message: 'Triggered from the project form',
      startedAt: new Date().toISOString()
    }
    items.value = [deployment, ...items.value]
    return deployment
  }

  return { items, enqueue }
}
