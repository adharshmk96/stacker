<script setup lang="ts">
import type { MetricSeries } from '~/types/monitoring'

const props = withDefaults(defineProps<{
  series: MetricSeries[]
  empty?: string
}>(), { empty: 'No readings in this period.' })

const palette = ['#38bdf8', '#a78bfa', '#34d399', '#fbbf24', '#fb7185', '#fb923c']
const width = 720
const height = 190
const pad = 12
const values = computed(() => props.series.flatMap(item => item.points.map(point => point.value)))
const max = computed(() => Math.max(...values.value, 1))
const min = computed(() => Math.min(...values.value, 0))
const span = computed(() => Math.max(max.value - min.value, 1))
const starts = computed(() => props.series.flatMap(item => item.points.map(point => point.at)))
// Timestamps are Unix milliseconds. Including `0` in the lower bound made
// every real reading render at the far-right edge of the chart.
const first = computed(() => starts.value.length ? Math.min(...starts.value) : 0)
const last = computed(() => starts.value.length ? Math.max(...starts.value) : 1)
const duration = computed(() => Math.max(last.value - first.value, 1))

function path(points: MetricSeries['points']) {
  if (!points.length) return ''
  return points.map((point, index) => {
    const x = pad + ((point.at - first.value) / duration.value) * (width - pad * 2)
    const y = height - pad - ((point.value - min.value) / span.value) * (height - pad * 2)
    return `${index ? 'L' : 'M'}${x.toFixed(1)},${y.toFixed(1)}`
  }).join(' ')
}

const formatter = new Intl.NumberFormat(undefined, { maximumFractionDigits: 1 })
const valueLabel = (value: number, unit: string) => {
  if (unit === 'B' || unit === 'B/s') {
    const units = ['B', 'KB', 'MB', 'GB', 'TB']
    let amount = value
    let index = 0
    while (Math.abs(amount) >= 1024 && index < units.length - 1) { amount /= 1024; index++ }
    return `${formatter.format(amount)} ${units[index]}${unit === 'B/s' ? '/s' : ''}`
  }
  return `${formatter.format(value)}${unit ? ` ${unit}` : ''}`
}
</script>

<template>
  <div v-if="values.length" class="min-w-0">
    <svg viewBox="0 0 720 190" preserveAspectRatio="none" class="h-48 w-full overflow-visible" role="img">
      <line v-for="line in 4" :key="line" x1="12" x2="708" :y1="line * 42" :y2="line * 42" stroke="currentColor" class="text-default" stroke-opacity=".7" />
      <path
        v-for="(item, index) in series"
        :key="item.name"
        :d="path(item.points)"
        fill="none"
        :stroke="palette[index % palette.length]"
        stroke-width="2.5"
        vector-effect="non-scaling-stroke"
      />
    </svg>
    <div class="mt-3 flex flex-wrap gap-x-4 gap-y-1 text-xs text-dimmed">
      <span v-for="(item, index) in series" :key="item.name" class="inline-flex items-center gap-1.5">
        <i class="size-2 rounded-full" :style="{ backgroundColor: palette[index % palette.length] }" />
        {{ item.name }}
        <span v-if="item.points.length" class="text-toned">{{ valueLabel(item.points.at(-1)!.value, item.unit) }}</span>
      </span>
    </div>
  </div>
  <p v-else class="flex h-48 items-center justify-center text-sm text-dimmed">{{ empty }}</p>
</template>
