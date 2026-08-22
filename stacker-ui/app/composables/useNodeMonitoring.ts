import type { MetricSummary, MonitoringDashboard } from '~/types/monitoring'

export function useNodeMonitoring() {
  const api = useApi()
  const summary = ref<MetricSummary | null>(null)
  const dashboard = ref<MonitoringDashboard | null>(null)
  const pending = ref(false)
  const error = ref<string | null>(null)

  async function load(id: string, range: string) {
    pending.value = true
    error.value = null
    try {
      const [nextSummary, nextDashboard] = await Promise.all([
        api.get<MetricSummary>(`/nodes/${id}/metrics/summary`),
        api.get<MonitoringDashboard>(`/nodes/${id}/metrics/dashboard?range=${range}`)
      ])
      summary.value = nextSummary
      dashboard.value = nextDashboard
    } catch (err) {
      error.value = err instanceof Error ? err.message : 'Could not load monitoring data'
    } finally {
      pending.value = false
    }
  }

  return { summary, dashboard, pending, error, load }
}
