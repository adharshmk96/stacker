/** Placeholder types — see the note in `types/project.ts`. */

export type DeploymentStatus = 'queued' | 'running' | 'succeeded' | 'failed' | 'cancelled'

/** What put the deployment in the queue */
export type DeploymentTriggerSource = 'manual' | 'push' | 'tag' | 'schedule'

export interface Deployment {
  id: string
  /** Short human reference, e.g. `#42` */
  number: number
  projectId: string
  projectName: string
  environment: string
  status: DeploymentStatus
  triggeredBy: DeploymentTriggerSource
  /** Who or what started it — a user name, or the webhook */
  actor: string
  /** Commit sha, or `compose` when the source is a pasted file */
  revision: string
  message: string
  startedAt: string
  /** Absent while queued or running */
  finishedAt?: string
  /** Wall-clock seconds; absent while still running */
  durationSec?: number
}
