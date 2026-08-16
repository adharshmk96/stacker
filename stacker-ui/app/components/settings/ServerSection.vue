<script setup lang="ts">
/**
 * Server tab: the stacker server itself — where it answers, what version it
 * runs, and the handful of operations (restart, update, maintenance) you need
 * when it misbehaves.
 *
 * Placeholder: nothing is persisted or called, the buttons toast and the
 * long-running ones fake a pending state so it can be seen.
 */
const toast = useToast()

/* ---- instance ---- */

const instance = {
  hostname: 'stacker-01',
  version: '0.9.4',
  latest: '0.10.0',
  builtAt: '2026-07-28T09:12:00Z',
  startedAt: '2026-08-11T04:02:00Z',
  docker: '27.1.2',
  os: 'Ubuntu 24.04 LTS'
}

const uptime = computed(() => {
  const ms = Date.now() - new Date(instance.startedAt).getTime()
  const days = Math.floor(ms / 86_400_000)
  const hours = Math.floor((ms % 86_400_000) / 3_600_000)
  return `${days}d ${hours}h`
})

const updateAvailable = computed(() => instance.version !== instance.latest)

/* ---- domain ---- */

const domain = reactive({
  primary: 'stacker.example.dev',
  wildcard: '*.apps.example.dev',
  forceHttps: true,
  tls: 'letsencrypt',
  acmeEmail: 'adharshmk96@gmail.com'
})

const tlsModes = [
  { label: "Let's Encrypt (automatic)", value: 'letsencrypt' },
  { label: 'Custom certificate', value: 'custom' },
  { label: 'None (HTTP only)', value: 'none' }
]

const savedDomain = ref(JSON.stringify(domain))
const domainDirty = computed(() => JSON.stringify(domain) !== savedDomain.value)

function saveDomain() {
  savedDomain.value = JSON.stringify(domain)
  toast.add({ title: 'Domain settings saved', icon: 'i-lucide-check-circle', color: 'success' })
}

function discardDomain() {
  Object.assign(domain, JSON.parse(savedDomain.value))
}

/** DNS records the operator has to create for the domains above. */
const dnsRecords = [
  { type: 'A', name: 'stacker', value: '203.0.113.24', status: 'verified' },
  { type: 'A', name: '*.apps', value: '203.0.113.24', status: 'verified' },
  { type: 'CAA', name: '@', value: '0 issue "letsencrypt.org"', status: 'missing' }
] as const

const dnsColor = { verified: 'success', pending: 'warning', missing: 'error' } as const

const verifying = ref(false)

async function verifyDns() {
  verifying.value = true
  await new Promise(resolve => setTimeout(resolve, 800))
  verifying.value = false
  toast.add({ title: 'DNS re-checked', description: '2 of 3 records resolve', icon: 'i-lucide-globe', color: 'info' })
}

/* ---- certificates ---- */

const certificates = [
  { domain: 'stacker.example.dev', issuer: "Let's Encrypt", expiresAt: '2026-10-19T00:00:00Z' },
  { domain: '*.apps.example.dev', issuer: "Let's Encrypt", expiresAt: '2026-09-02T00:00:00Z' }
]

const daysLeft = (value: string) =>
  Math.round((new Date(value).getTime() - Date.now()) / 86_400_000)

const renewing = ref(false)

async function renewCertificates() {
  renewing.value = true
  await new Promise(resolve => setTimeout(resolve, 900))
  renewing.value = false
  toast.add({ title: 'Certificates renewed', icon: 'i-lucide-shield-check', color: 'success' })
}

/* ---- maintenance ---- */

const maintenance = ref(false)

watch(maintenance, value => {
  toast.add({
    title: value ? 'Maintenance mode on' : 'Maintenance mode off',
    description: value
      ? 'Deployments are paused and the dashboard is read-only.'
      : 'Normal operation resumed.',
    icon: value ? 'i-lucide-construction' : 'i-lucide-play',
    color: value ? 'warning' : 'success'
  })
})

const busy = ref<'restart' | 'update' | 'reload' | null>(null)

async function run(action: 'restart' | 'update' | 'reload') {
  busy.value = action
  await new Promise(resolve => setTimeout(resolve, 1000))
  busy.value = null

  const messages = {
    restart: { title: 'Server restarted', description: `${instance.hostname} is back up` },
    update: { title: `Updated to ${instance.latest}`, description: 'Restarted into the new version' },
    reload: { title: 'Proxy configuration reloaded', description: 'Routes and certificates re-read' }
  } as const

  toast.add({ ...messages[action], icon: 'i-lucide-check-circle', color: 'success' })
}

/* ---- logs ---- */

const logLevel = ref('info')
const logLevels = [
  { label: 'Error', value: 'error' },
  { label: 'Warn', value: 'warn' },
  { label: 'Info', value: 'info' },
  { label: 'Debug', value: 'debug' }
]

const formatDate = (value: string) =>
  new Date(value).toLocaleDateString(undefined, { day: 'numeric', month: 'short', year: 'numeric' })
</script>

<template>
  <SettingsSection
    title="Instance"
    description="The stacker server running this dashboard."
  >
    <template #header-right>
      <UBadge
        :label="maintenance ? 'Maintenance' : 'Healthy'"
        :color="maintenance ? 'warning' : 'success'"
        variant="subtle"
        :icon="maintenance ? 'i-lucide-construction' : 'i-lucide-heart-pulse'"
      />
    </template>

    <dl class="grid gap-4 sm:grid-cols-3">
      <div v-for="field in [
        { label: 'Hostname', value: instance.hostname },
        { label: 'Version', value: `v${instance.version}` },
        { label: 'Uptime', value: uptime },
        { label: 'Docker', value: instance.docker },
        { label: 'Operating system', value: instance.os },
        { label: 'Built', value: formatDate(instance.builtAt) }
      ]" :key="field.label">
        <dt class="text-xs uppercase tracking-[0.08em] text-dimmed">{{ field.label }}</dt>
        <dd class="mt-1 font-mono text-sm text-highlighted">{{ field.value }}</dd>
      </div>
    </dl>
  </SettingsSection>

  <SettingsSection
    title="Domain"
    description="Where the dashboard and API answer, and the wildcard deployments get their subdomains from."
  >
    <div class="grid gap-4 sm:grid-cols-2">
      <UFormField label="Primary domain" hint="Dashboard and API">
        <UInput v-model="domain.primary" placeholder="stacker.example.dev" class="w-full" />
      </UFormField>

      <UFormField label="Wildcard domain" hint="Deployments">
        <UInput v-model="domain.wildcard" placeholder="*.apps.example.dev" class="w-full" />
      </UFormField>

      <UFormField label="TLS">
        <USelect v-model="domain.tls" :items="tlsModes" class="w-full" />
      </UFormField>

      <UFormField
        label="ACME contact email"
        hint="Expiry notices"
      >
        <UInput
          v-model="domain.acmeEmail"
          type="email"
          :disabled="domain.tls !== 'letsencrypt'"
          class="w-full"
        />
      </UFormField>
    </div>

    <div class="mt-4 flex items-center justify-between rounded-md border border-default bg-elevated/40 p-3">
      <div class="leading-tight">
        <p class="text-sm text-toned">Force HTTPS</p>
        <p class="text-xs text-dimmed">Redirect every plain HTTP request to https.</p>
      </div>
      <USwitch v-model="domain.forceHttps" :disabled="domain.tls === 'none'" />
    </div>

    <template #footer>
      <UButton label="Discard" color="neutral" variant="ghost" :disabled="!domainDirty" @click="discardDomain" />
      <UButton label="Save" icon="i-lucide-save" :disabled="!domainDirty" @click="saveDomain" />
    </template>
  </SettingsSection>

  <SettingsSection
    title="DNS records"
    description="Point these at the server before saving a new domain."
  >
    <template #header-right>
      <UButton
        label="Re-check"
        icon="i-lucide-refresh-cw"
        size="xs"
        color="neutral"
        variant="ghost"
        :loading="verifying"
        @click="verifyDns"
      />
    </template>

    <div
      v-for="record in dnsRecords"
      :key="`${record.type}-${record.name}`"
      class="flex flex-wrap items-center gap-3 border-t border-default py-2.5 text-sm first:border-t-0"
    >
      <UBadge :label="record.type" color="neutral" variant="subtle" size="sm" class="font-mono" />
      <span class="font-mono text-xs text-toned">{{ record.name }}</span>
      <span class="min-w-0 flex-1 truncate font-mono text-xs text-dimmed">{{ record.value }}</span>
      <UBadge :label="record.status" :color="dnsColor[record.status]" variant="subtle" size="sm" />
    </div>
  </SettingsSection>

  <SettingsSection
    title="Certificates"
    description="Issued for the domains above. Renewal runs automatically 30 days before expiry."
  >
    <template #header-right>
      <UButton
        label="Renew now"
        icon="i-lucide-shield-check"
        size="xs"
        color="neutral"
        variant="subtle"
        :loading="renewing"
        :disabled="domain.tls === 'none'"
        @click="renewCertificates"
      />
    </template>

    <div
      v-for="cert in certificates"
      :key="cert.domain"
      class="flex flex-wrap items-center gap-3 border-t border-default py-2.5 text-sm first:border-t-0"
    >
      <UIcon name="i-lucide-lock" class="size-4 text-dimmed" />
      <span class="font-mono text-xs text-toned">{{ cert.domain }}</span>
      <span class="text-xs text-dimmed">{{ cert.issuer }}</span>
      <UBadge
        :label="`${daysLeft(cert.expiresAt)} days left`"
        :color="daysLeft(cert.expiresAt) < 21 ? 'warning' : 'neutral'"
        variant="subtle"
        size="sm"
        class="ms-auto"
      />
    </div>
  </SettingsSection>

  <SettingsSection
    title="Updates"
    description="Stacker updates in place — the running containers are untouched."
  >
    <div class="flex flex-wrap items-center gap-3 rounded-md border border-default bg-elevated/40 p-4">
      <div class="flex size-9 items-center justify-center rounded-md bg-elevated ring-1 ring-default">
        <UIcon
          :name="updateAvailable ? 'i-lucide-arrow-up-circle' : 'i-lucide-check'"
          class="size-5"
          :class="updateAvailable ? 'text-primary' : 'text-highlighted'"
        />
      </div>
      <div class="min-w-0 leading-tight">
        <p class="font-medium text-highlighted">
          {{ updateAvailable ? `v${instance.latest} is available` : 'Up to date' }}
        </p>
        <p class="font-mono text-xs text-dimmed">Running v{{ instance.version }}</p>
      </div>
      <div class="ms-auto flex gap-2">
        <UButton
          label="Release notes"
          icon="i-lucide-arrow-up-right"
          color="neutral"
          variant="subtle"
          size="sm"
        />
        <UButton
          :label="updateAvailable ? `Update to v${instance.latest}` : 'Check for updates'"
          :icon="updateAvailable ? 'i-lucide-download' : 'i-lucide-refresh-cw'"
          size="sm"
          :loading="busy === 'update'"
          :disabled="busy !== null"
          @click="run('update')"
        />
      </div>
    </div>
  </SettingsSection>

  <SettingsSection
    title="Maintenance"
    description="Pause the moving parts while you work on the host."
  >
    <div class="flex items-center justify-between rounded-md border border-default bg-elevated/40 p-3">
      <div class="leading-tight">
        <p class="text-sm text-toned">Maintenance mode</p>
        <p class="text-xs text-dimmed">Pauses deployments and makes the dashboard read-only.</p>
      </div>
      <USwitch v-model="maintenance" />
    </div>

    <UFormField label="Log level" class="mt-4 max-w-56">
      <USelect v-model="logLevel" :items="logLevels" class="w-full" />
    </UFormField>

    <div class="mt-5 grid gap-3 sm:grid-cols-2">
      <div class="flex items-center gap-3 rounded-md border border-default p-3">
        <div class="min-w-0 leading-tight">
          <p class="text-sm text-toned">Reload proxy</p>
          <p class="text-xs text-dimmed">Re-reads routes and certificates. No downtime.</p>
        </div>
        <UButton
          label="Reload"
          icon="i-lucide-rotate-cw"
          color="neutral"
          variant="subtle"
          size="sm"
          class="ms-auto"
          :loading="busy === 'reload'"
          :disabled="busy !== null"
          @click="run('reload')"
        />
      </div>

      <div class="flex items-center gap-3 rounded-md border border-default p-3">
        <div class="min-w-0 leading-tight">
          <p class="text-sm text-toned">Restart server</p>
          <p class="text-xs text-dimmed">The dashboard is unreachable for a few seconds.</p>
        </div>
        <UButton
          label="Restart"
          icon="i-lucide-power"
          color="warning"
          variant="subtle"
          size="sm"
          class="ms-auto"
          :loading="busy === 'restart'"
          :disabled="busy !== null"
          @click="run('restart')"
        />
      </div>
    </div>
  </SettingsSection>

  <SettingsSection
    title="Danger zone"
    description="These cannot be undone."
    danger
  >
    <div class="flex flex-wrap items-center gap-3 py-1">
      <div class="min-w-0 leading-tight">
        <p class="text-sm text-toned">Reset server configuration</p>
        <p class="text-xs text-dimmed">
          Clears domains, registries and integrations. Projects and volumes are kept.
        </p>
      </div>
      <UButton label="Reset" icon="i-lucide-eraser" color="error" variant="subtle" size="sm" class="ms-auto" />
    </div>

    <div class="mt-3 flex flex-wrap items-center gap-3 border-t border-default pt-3">
      <div class="min-w-0 leading-tight">
        <p class="text-sm text-toned">Shut down server</p>
        <p class="text-xs text-dimmed">
          Stops stacker. It has to be started again from the host shell.
        </p>
      </div>
      <UButton label="Shut down" icon="i-lucide-power-off" color="error" size="sm" class="ms-auto" />
    </div>
  </SettingsSection>
</template>
