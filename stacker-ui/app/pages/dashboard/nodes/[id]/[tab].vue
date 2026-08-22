<script setup lang="ts">
import type { NavigationMenuItem } from '@nuxt/ui'

definePageMeta({ key: route => String(route.params.id) })

const route = useRoute()
const id = computed(() => String(route.params.id))
const tab = computed(() => String(route.params.tab))
const { items, load, find } = useNodes()
const { summary, dashboard, pending, error, load: loadMonitoring } = useNodeMonitoring()
const range = ref('24h')
const ranges = [
  { label: '10 minutes', value: '10m' }, { label: '1 hour', value: '1h' }, { label: '12 hours', value: '12h' },
  { label: '24 hours', value: '24h' }, { label: '7 days', value: '7d' }, { label: '30 days', value: '30d' }
]
const node = computed(() => find(id.value))
const tabs = [{ label: 'Monitoring', icon: 'i-lucide-chart-no-axes-combined', to: `/dashboard/nodes/${id.value}/monitoring` }]
const tabItems = computed<NavigationMenuItem[]>(() => tabs)
let refreshTimer: ReturnType<typeof setInterval> | undefined

const refreshMonitoring = () => {
  if (pending.value || document.hidden) return
  void loadMonitoring(id.value, range.value)
}

const refreshWhenVisible = () => {
  if (!document.hidden) refreshMonitoring()
}

watchEffect(() => {
  if (tab.value !== 'monitoring') throw createError({ statusCode: 404, statusMessage: 'Unknown node tab', fatal: true })
})
onMounted(async () => {
  await load()
  if (!node.value) throw createError({ statusCode: 404, statusMessage: 'Node not found', fatal: true })
  await loadMonitoring(id.value, range.value)
  refreshTimer = setInterval(refreshMonitoring, 30_000)
  document.addEventListener('visibilitychange', refreshWhenVisible)
})
onBeforeUnmount(() => {
  if (refreshTimer) clearInterval(refreshTimer)
  document.removeEventListener('visibilitychange', refreshWhenVisible)
})
watch(range, () => loadMonitoring(id.value, range.value))
useHead(() => ({ title: `${node.value?.name ?? 'Node'} · Stacker` }))

const percent = (value?: number) => value == null ? 0 : Math.max(0, Math.min(100, value))
const formatPercent = (value?: number) => value == null ? '—' : `${value.toFixed(1)}%`
const formatUptime = (value?: number) => {
  if (value == null) return '—'
  const days = Math.floor(value / 86400)
  return days ? `${days}d` : `${Math.floor(value / 3600)}h`
}
</script>

<template>
  <UDashboardPanel id="node-detail" :ui="{ body: 'relative' }">
    <template #header>
      <UDashboardNavbar :title="node?.name ?? 'Node'">
        <template #leading><UButton icon="i-lucide-arrow-left" color="neutral" variant="ghost" to="/dashboard/nodes" aria-label="Back to nodes" /></template>
        <template #right><UButton label="Refresh" icon="i-lucide-refresh-cw" color="neutral" variant="subtle" :loading="pending" @click="loadMonitoring(id, range)" /></template>
      </UDashboardNavbar>
      <UDashboardToolbar>
        <template #left>
          <UNavigationMenu :items="tabItems" variant="link" />
          <span class="inline-flex items-center gap-1.5 text-xs text-dimmed">
            <i class="size-2 rounded-full bg-emerald-400" /> Live · 30s
          </span>
        </template>
        <template #right><USelect v-model="range" :items="ranges" value-key="value" aria-label="Monitoring resolution" class="w-36" /></template>
      </UDashboardToolbar>
    </template>
    <template #body>
      <div class="stacker-grid pointer-events-none absolute inset-0" />
      <UAlert v-if="error" title="Monitoring is unavailable" :description="error" icon="i-lucide-chart-no-axes-column-increasing" color="warning" variant="subtle" class="mb-4" />
      <UAlert v-else-if="summary && !summary.available" title="Monitoring is not ready" :description="summary.message" icon="i-lucide-clock-3" color="neutral" variant="subtle" class="mb-4" />
      <template v-else-if="summary?.available">
        <div class="grid gap-4 md:grid-cols-3">
          <section v-for="item in [{ label: 'CPU usage', value: summary.cpu, color: 'bg-sky-400' }, { label: 'Memory usage', value: summary.memory, color: 'bg-violet-400' }, { label: 'Disk usage', value: summary.disk, color: 'bg-emerald-400' }]" :key="item.label" class="rounded-lg border border-default bg-default/60 p-4 backdrop-blur">
            <div class="flex items-baseline justify-between"><p class="text-sm text-muted">{{ item.label }}</p><strong class="text-xl text-highlighted">{{ formatPercent(item.value) }}</strong></div>
            <div class="mt-3 h-2 overflow-hidden rounded-full bg-elevated"><div class="h-full rounded-full transition-all" :class="item.color" :style="{ width: `${percent(item.value)}%` }" /></div>
          </section>
        </div>
        <div class="mt-4 grid gap-4 sm:grid-cols-2">
          <section class="rounded-lg border border-default bg-default/60 p-4 backdrop-blur"><p class="text-sm text-muted">Load average (1m)</p><p class="mt-1 text-2xl font-semibold text-highlighted">{{ summary.load1?.toFixed(2) ?? '—' }}</p></section>
          <section class="rounded-lg border border-default bg-default/60 p-4 backdrop-blur"><p class="text-sm text-muted">Uptime</p><p class="mt-1 text-2xl font-semibold text-highlighted">{{ formatUptime(summary.uptime) }}</p></section>
        </div>
        <div class="mt-4 grid gap-4 xl:grid-cols-2">
          <section v-for="chart in [{ title: 'CPU usage', data: dashboard?.cpu }, { title: 'Memory usage', data: dashboard?.memory }, { title: 'Disk space', data: dashboard?.disk }, { title: 'Disk read / write', data: dashboard?.diskIo }, { title: 'Network traffic', data: dashboard?.network }, { title: 'Container CPU usage', data: dashboard?.containers }, { title: 'Container memory usage', data: dashboard?.containerMemory }]" :key="chart.title" class="min-w-0 rounded-lg border border-default bg-default/60 p-4 backdrop-blur">
            <h2 class="mb-1 text-sm font-semibold text-highlighted">{{ chart.title }}</h2>
            <p class="mb-3 text-xs text-dimmed">{{ range }} history</p>
            <NodeMetricsChart :series="chart.data ?? []" />
          </section>
        </div>
      </template>
      <div v-else-if="pending" class="flex h-64 items-center justify-center"><UIcon name="i-lucide-loader-circle" class="size-6 animate-spin text-dimmed" /></div>
    </template>
  </UDashboardPanel>
</template>
