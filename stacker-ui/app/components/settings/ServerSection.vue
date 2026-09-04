<script setup lang="ts">
import type { ServerSettings, ServerUpdates, ServiceInfo, UpdateCandidate } from '~/types/server'

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
const updates = ref<ServerUpdates | null>(null)
const updatesLoading = ref(true)
const updatesError = ref('')
const updateOpen = ref(false)
const selectedUpdate = ref<UpdateCandidate | null>(null)
const startingUpdate = ref(false)
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
    domain.value = settings.value.traefik.domain
    savedDomain.value = settings.value.traefik.domain
  } catch (error) {
    loadError.value = error instanceof Error ? error.message : 'Could not load server settings'
  } finally {
    loading.value = false
  }
}

async function loadUpdates() {
  updatesLoading.value = true
  updatesError.value = ''
  try {
    updates.value = await api.get<ServerUpdates>('/server/updates')
  } catch (error) {
    updatesError.value = error instanceof Error ? error.message : 'Could not check for updates'
  } finally {
    updatesLoading.value = false
  }
}

function openUpdate(candidate: UpdateCandidate) {
  selectedUpdate.value = candidate
  updateOpen.value = true
}

async function startUpdate() {
  if (!selectedUpdate.value) return
  startingUpdate.value = true
  const before = settings.value?.instance
  try {
    await api.post('/server/updates', { channel: selectedUpdate.value.channel })
    updateOpen.value = false
    toast.add({
      title: `${selectedUpdate.value.channel === 'stable' ? 'Stable' : 'Edge'} update started`,
      description: 'The dashboard will disconnect briefly while Stacker rebuilds and restarts.',
      icon: 'i-lucide-download',
      color: 'success'
    })
    const timer = window.setInterval(async () => {
      try {
        const refreshed = await api.get<ServerSettings>('/server')
        const changed = refreshed.instance.version !== before?.version || refreshed.instance.revision !== before?.revision
        if (!changed) return
        window.clearInterval(timer)
        window.location.reload()
      } catch {
        // A network failure is expected while the Swarm task is replaced.
      }
    }, 5_000)
    window.setTimeout(() => window.clearInterval(timer), 15 * 60_000)
  } catch (error) {
    toast.add({ title: 'Could not start update', description: error instanceof Error ? error.message : undefined, icon: 'i-lucide-circle-alert', color: 'error' })
  } finally {
    startingUpdate.value = false
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
const formatDateTime = (value?: string) => value ? new Date(value).toLocaleString() : '—'
const statusColor = (status: ServiceInfo['status']) => status === 'healthy' ? 'success' : status === 'degraded' ? 'warning' : 'neutral'

onMounted(() => {
  load()
  loadUpdates()
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

  <SettingsSection title="Updates" description="Install a newer Stacker release or move to the latest commit on main.">
    <div v-if="updatesLoading" class="grid gap-3 sm:grid-cols-2">
      <USkeleton v-for="index in 2" :key="index" class="h-24" />
    </div>
    <UAlert
      v-else-if="updatesError"
      title="Could not check for updates"
      :description="updatesError"
      icon="i-lucide-circle-alert"
      color="error"
      :actions="[{ label: 'Retry', onClick: loadUpdates }]"
    />
    <div v-else-if="updates" class="grid gap-3 sm:grid-cols-2">
      <div v-for="candidate in [updates.stable, updates.edge]" :key="candidate.channel" class="rounded-md border border-default p-4">
        <div class="flex items-start justify-between gap-3">
          <div>
            <p class="font-medium text-highlighted">{{ candidate.channel === 'stable' ? 'Stable release' : 'Edge (main)' }}</p>
            <p class="mt-1 font-mono text-xs text-dimmed">
              {{ candidate.channel === 'stable' ? candidate.version : candidate.revision.slice(0, 12) }}
            </p>
          </div>
          <UBadge :label="candidate.available ? 'Available' : 'Up to date'" :color="candidate.available ? 'success' : 'neutral'" variant="subtle" />
        </div>
        <UButton
          class="mt-4"
          :label="candidate.channel === 'stable' ? 'Update stable' : 'Update edge'"
          icon="i-lucide-download"
          color="neutral"
          variant="subtle"
          size="sm"
          :disabled="!candidate.available || updates.updating"
          @click="openUpdate(candidate)"
        />
      </div>
    </div>
    <UAlert
      v-if="updates?.error"
      class="mt-4"
      title="Server update failed"
      :description="updates.error"
      icon="i-lucide-circle-alert"
      color="error"
    />
  </SettingsSection>

  <SettingsSection title="Domain" description="The hostname Traefik uses for this dashboard and API. DNS must point to this server.">
    <UFormField label="Stacker domain" hint="Hostname only — no https:// or path" class="max-w-xl">
      <UInput v-model="domain" placeholder="stacker.203.0.113.10.sslip.io" class="w-full" />
    </UFormField>

    <dl v-if="settings" class="mt-5 grid gap-4 border-t border-default pt-4 sm:grid-cols-2">
      <div v-for="field in [
        { label: 'HTTPS', value: settings.traefik.https ? 'Enabled' : 'Disabled' },
        { label: 'Certificate resolver', value: settings.traefik.certificateResolver || 'None' },
        { label: 'Backend target', value: settings.traefik.backendTarget || 'Unavailable' },
        { label: 'HTTP redirect', value: settings.traefik.httpRedirect ? 'HTTP → HTTPS' : 'Disabled' }
      ]" :key="field.label">
        <dt class="text-xs uppercase tracking-[0.08em] text-dimmed">{{ field.label }}</dt>
        <dd class="mt-1 font-mono text-sm text-highlighted">{{ field.value }}</dd>
      </div>
    </dl>
    <template #footer>
      <UButton label="Discard" color="neutral" variant="ghost" :disabled="!domainDirty" @click="domain = savedDomain" />
      <UButton label="Save domain" icon="i-lucide-save" :loading="savingDomain" :disabled="!domainDirty || !domain.trim()" @click="saveDomain" />
    </template>
  </SettingsSection>

  <SettingsSection title="Services" description="Live Docker Swarm state for Stacker and its Traefik proxy.">
    <div v-if="settings" class="grid gap-3 sm:grid-cols-2">
      <div
        v-for="service in [settings.traefik.stackerService, settings.traefik.traefikService]"
        :key="service.name"
        class="rounded-md border border-default p-4"
      >
        <div class="flex items-center justify-between gap-3">
          <p class="font-medium text-highlighted">{{ service.name.endsWith('_traefik') ? 'Traefik' : 'Stacker' }}</p>
          <UBadge :label="service.status" :color="statusColor(service.status)" variant="subtle" />
        </div>
        <dl class="mt-4 grid gap-3 text-sm">
          <div class="flex justify-between gap-4">
            <dt class="text-dimmed">Replicas</dt>
            <dd class="font-mono text-highlighted">{{ service.running }}/{{ service.desired }}</dd>
          </div>
          <div v-if="service.version" class="flex justify-between gap-4">
            <dt class="text-dimmed">Version</dt>
            <dd class="font-mono text-highlighted">{{ service.version }}</dd>
          </div>
          <div class="flex justify-between gap-4">
            <dt class="text-dimmed">Last updated</dt>
            <dd class="font-mono text-highlighted">{{ formatDateTime(service.updatedAt) }}</dd>
          </div>
        </dl>
      </div>
    </div>

    <dl v-if="settings" class="mt-4 grid gap-4 border-t border-default pt-4 sm:grid-cols-2">
      <div>
        <dt class="text-xs uppercase tracking-[0.08em] text-dimmed">Swarm stack</dt>
        <dd class="mt-1 font-mono text-sm text-highlighted">{{ settings.traefik.stackName }}</dd>
      </div>
      <div>
        <dt class="text-xs uppercase tracking-[0.08em] text-dimmed">Published ports</dt>
        <dd class="mt-1 font-mono text-sm text-highlighted">{{ settings.traefik.publishedPorts.join(', ') || 'Unavailable' }}</dd>
      </div>
    </dl>
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

  <UModal v-model:open="updateOpen" :title="selectedUpdate?.channel === 'stable' ? 'Update Stacker stable' : 'Update Stacker edge'">
    <template #body>
      <p class="text-sm text-muted">
        This rebuilds Stacker and replaces its Docker Swarm task. Your projects, domains, certificates, and stored data stay in place, but the dashboard will be briefly unavailable.
      </p>
      <p v-if="selectedUpdate" class="mt-4 font-mono text-sm text-highlighted">
        Target: {{ selectedUpdate.channel === 'stable' ? selectedUpdate.version : selectedUpdate.revision }}
      </p>
      <UAlert
        v-if="selectedUpdate && selectedUpdate.channel !== (settings?.instance.version === 'main' ? 'edge' : 'stable')"
        class="mt-4"
        title="This switches update channels"
        description="This replaces the current build with the selected channel."
        icon="i-lucide-git-branch"
        color="warning"
      />
    </template>
    <template #footer>
      <div class="flex w-full justify-end gap-2">
        <UButton label="Cancel" color="neutral" variant="ghost" :disabled="startingUpdate" @click="updateOpen = false" />
        <UButton label="Start update" icon="i-lucide-download" :loading="startingUpdate" @click="startUpdate" />
      </div>
    </template>
  </UModal>
</template>
