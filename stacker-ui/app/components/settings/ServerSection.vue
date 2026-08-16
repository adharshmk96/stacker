<script setup lang="ts">
interface ServerSettings {
  instance: { hostname: string, version: string, builtAt?: string, startedAt: string, docker?: string, os?: string }
  domain: string
}

const api = useApi()
const toast = useToast()
const settings = ref<ServerSettings | null>(null)
const loading = ref(true)
const loadError = ref('')
const domain = ref('')
const savedDomain = ref('')
const savingDomain = ref(false)
const restarting = ref<'stacker' | 'traefik' | null>(null)
const resetOpen = ref(false)
const now = ref(Date.now())
let clock: ReturnType<typeof setInterval> | undefined

const domainDirty = computed(() => domain.value.trim().toLowerCase() !== savedDomain.value)
const uptime = computed(() => {
  if (!settings.value) return '—'
  const ms = Math.max(0, now.value - new Date(settings.value.instance.startedAt).getTime())
  const days = Math.floor(ms / 86_400_000)
  const hours = Math.floor((ms % 86_400_000) / 3_600_000)
  const minutes = Math.floor((ms % 3_600_000) / 60_000)
  return days ? `${days}d ${hours}h` : `${hours}h ${minutes}m`
})

async function load() {
  loading.value = true
  loadError.value = ''
  try {
    settings.value = await api.get<ServerSettings>('/server')
    domain.value = settings.value.domain
    savedDomain.value = settings.value.domain
  } catch (error) {
    loadError.value = error instanceof Error ? error.message : 'Could not load server settings'
  } finally {
    loading.value = false
  }
}

async function saveDomain() {
  savingDomain.value = true
  try {
    const result = await api.put<{ domain: string }>('/server/domain', { domain: domain.value })
    domain.value = result.domain
    savedDomain.value = result.domain
    toast.add({ title: 'Domain updated', description: `Traefik is now serving ${result.domain}`, icon: 'i-lucide-check-circle', color: 'success' })
  } catch (error) {
    toast.add({ title: 'Could not update domain', description: error instanceof Error ? error.message : undefined, icon: 'i-lucide-circle-alert', color: 'error' })
  } finally {
    savingDomain.value = false
  }
}

async function restart(target: 'stacker' | 'traefik') {
  restarting.value = target
  try {
    await api.post('/server/restart', { target })
    toast.add({
      title: `${target === 'stacker' ? 'Stacker' : 'Traefik'} restart started`,
      description: target === 'stacker' ? 'This dashboard may be unavailable briefly.' : undefined,
      icon: 'i-lucide-rotate-cw',
      color: 'success'
    })
  } catch (error) {
    toast.add({ title: `Could not restart ${target}`, description: error instanceof Error ? error.message : undefined, icon: 'i-lucide-circle-alert', color: 'error' })
  } finally {
    restarting.value = null
  }
}

const formatDate = (value?: string) => value
  ? new Date(value).toLocaleDateString(undefined, { day: 'numeric', month: 'short', year: 'numeric' })
  : '—'

onMounted(() => {
  load()
  clock = setInterval(() => { now.value = Date.now() }, 60_000)
})
onUnmounted(() => clearInterval(clock))
</script>

<template>
  <UAlert
    v-if="loadError"
    title="Could not load server information"
    :description="loadError"
    icon="i-lucide-circle-alert"
    color="error"
    :actions="[{ label: 'Retry', onClick: load }]"
  />

  <SettingsSection title="Instance" description="Live information from this Stacker server.">
    <div v-if="loading" class="grid gap-4 sm:grid-cols-3">
      <USkeleton v-for="index in 6" :key="index" class="h-11" />
    </div>
    <dl v-else-if="settings" class="grid gap-4 sm:grid-cols-3">
      <div v-for="field in [
        { label: 'Hostname', value: settings.instance.hostname },
        { label: 'Version', value: settings.instance.version },
        { label: 'Uptime', value: uptime },
        { label: 'Docker', value: settings.instance.docker || 'Unavailable' },
        { label: 'Operating system', value: settings.instance.os || 'Unavailable' },
        { label: 'Built', value: formatDate(settings.instance.builtAt) }
      ]" :key="field.label">
        <dt class="text-xs uppercase tracking-[0.08em] text-dimmed">{{ field.label }}</dt>
        <dd class="mt-1 font-mono text-sm text-highlighted">{{ field.value }}</dd>
      </div>
    </dl>
  </SettingsSection>

  <SettingsSection title="Domain" description="The hostname Traefik uses for this dashboard and API. DNS must point to this server.">
    <UFormField label="Stacker domain" hint="Hostname only — no https:// or path" class="max-w-xl">
      <UInput v-model="domain" placeholder="stacker.203.0.113.10.sslip.io" class="w-full" />
    </UFormField>
    <template #footer>
      <UButton label="Discard" color="neutral" variant="ghost" :disabled="!domainDirty" @click="domain = savedDomain" />
      <UButton label="Save domain" icon="i-lucide-save" :loading="savingDomain" :disabled="!domainDirty || !domain.trim()" @click="saveDomain" />
    </template>
  </SettingsSection>

  <SettingsSection title="Maintenance" description="Restart Stacker services running in Docker Swarm.">
    <div class="grid gap-3 sm:grid-cols-2">
      <div v-for="item in [
        { target: 'stacker' as const, title: 'Restart Stacker', description: 'The dashboard will be unavailable briefly.' },
        { target: 'traefik' as const, title: 'Restart Traefik', description: 'Web traffic may pause while the proxy restarts.' }
      ]" :key="item.target" class="flex items-center gap-3 rounded-md border border-default p-3">
        <div class="min-w-0 leading-tight">
          <p class="text-sm text-toned">{{ item.title }}</p>
          <p class="text-xs text-dimmed">{{ item.description }}</p>
        </div>
        <UButton
          label="Restart"
          icon="i-lucide-rotate-cw"
          color="neutral"
          variant="subtle"
          size="sm"
          class="ms-auto"
          :loading="restarting === item.target"
          :disabled="restarting !== null"
          @click="restart(item.target)"
        />
      </div>
    </div>
  </SettingsSection>

  <SettingsSection title="Danger zone" description="Reset all Stacker data and return this installation to first run." danger>
    <UButton label="Reset all data" icon="i-lucide-rotate-ccw" color="error" variant="subtle" @click="resetOpen = true" />
  </SettingsSection>

  <SettingsResetDataModal v-model:open="resetOpen" />
</template>
