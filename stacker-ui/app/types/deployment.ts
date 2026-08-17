/**
 * Deployments, as the stacker server records them (`/api/deployments`).
 *
 * A run is a record of what happened: the project and environment names are
 * copied onto it, so renaming a project afterwards does not rewrite its history.
 */

export type DeploymentStatus = 'queued' | 'running' | 'succeeded' | 'failed' | 'cancelled'

/** What put the deployment in the queue */
export type DeploymentTriggerSource = 'manual' | 'push' | 'tag' | 'schedule'

export interface Deployment {
  id: string
  /** Short human reference, counted per project — `#42` */
  number: number
  projectId: string
  projectName: string
  environmentId: string
  /** Environment name at the time of the run */
  environment: string
  /** The swarm stack the run deployed */
  stack: string
  status: DeploymentStatus
  triggeredBy: DeploymentTriggerSource
  /** Who or what started it — a username, or the trigger */
  actor: string
  /** Commit sha that was built, or `compose` for an inline source */
  revision: string
  message: string
  /** Why a failed run failed, short enough for a toast; the log has the rest */
  error?: string
  startedAt: string
  /** Absent while queued or running */
  finishedAt?: string
  /** Wall-clock seconds; absent while still running */
  durationSec?: number
}

/**
 * A slice of a run's output.
 *
 * Logs are read with a cursor rather than re-fetched: `next` goes back on the
 * following poll and only the lines after it come down, so following a live
 * build costs one short response per tick.
 */
export interface LogChunk {
  deploymentId: string
  status: DeploymentStatus
  lines: string[]
  /** Cursor to send on the next poll */
  next: number
  /** True once no more lines will arrive */
  done: boolean
}

/** Statuses a run can still move out of — what the pollers watch for. */
export function isLive(status: DeploymentStatus) {
  return status === 'queued' || status === 'running'
}
