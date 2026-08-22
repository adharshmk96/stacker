export interface MetricSummary {
  available: boolean
  message?: string
  cpu?: number
  memory?: number
  disk?: number
  load1?: number
  uptime?: number
}

export interface MetricPoint {
  at: number
  value: number
}

export interface MetricSeries {
  name: string
  unit: string
  points: MetricPoint[]
}

export interface MonitoringDashboard {
  range: string
  cpu: MetricSeries[]
  memory: MetricSeries[]
  disk: MetricSeries[]
  diskIo: MetricSeries[]
  network: MetricSeries[]
  containers: MetricSeries[]
  containerMemory: MetricSeries[]
}
