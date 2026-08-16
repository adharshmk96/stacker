<script setup lang="ts">
/**
 * Backup tab: what stacker snapshots (its own config database, and optionally
 * named volumes), where the snapshots go, and how long they are kept.
 *
 * Placeholder: nothing is persisted, the buttons just toast.
 */
const toast = useToast()

const backup = reactive({
  enabled: true,
  schedule: 'daily',
  time: '03:00',
  retentionDays: 30,
  includeVolumes: true,
  encrypt: true,
  destination: 's3',
  s3: {
    bucket: 'stacker-backups',
    region: 'ap-south-1',
    endpoint: 'https://s3.ap-south-1.amazonaws.com',
    accessKeyId: 'AKIA••••••••••••',
    secretAccessKey: ''
  },
  localPath: '/var/lib/stacker/backups'
})

const schedules = [
  { label: 'Hourly', value: 'hourly' },
  { label: 'Daily', value: 'daily' },
  { label: 'Weekly', value: 'weekly' }
]

const destinations = [
  { label: 'S3-compatible', value: 's3', icon: 'i-lucide-cloud' },
  { label: 'Local disk', value: 'local', icon: 'i-lucide-hard-drive' }
]

const saved = ref(JSON.stringify(backup))
const dirty = computed(() => JSON.stringify(backup) !== saved.value)

function save() {
  saved.value = JSON.stringify(backup)
  toast.add({ title: 'Backup settings saved', icon: 'i-lucide-check-circle', color: 'success' })
}

function discard() {
  Object.assign(backup, JSON.parse(saved.value))
}

/* ---- manual run ---- */

const running = ref(false)

async function runNow() {
  running.value = true
  // Placeholder: a short wait so the pending state is visible.
  await new Promise(resolve => setTimeout(resolve, 900))
  running.value = false

  toast.add({ title: 'Backup started', icon: 'i-lucide-database-backup', color: 'success' })
}

/** Snapshots already taken, newest first. */
const snapshots = ref([
  { id: 'b1', name: '2026-08-16-0300.tar.zst', size: '184 MB', status: 'complete', at: '2026-08-16T03:00:00Z' },
  { id: 'b2', name: '2026-08-15-0300.tar.zst', size: '181 MB', status: 'complete', at: '2026-08-15T03:00:00Z' },
  { id: 'b3', name: '2026-08-14-0300.tar.zst', size: '—', status: 'failed', at: '2026-08-14T03:00:00Z' }
])

const statusColor = {
  complete: 'success',
  failed: 'error',
  running: 'warning'
} as const

type SnapshotStatus = keyof typeof statusColor

const restoreOpen = ref(false)
const selected = ref<(typeof snapshots.value)[number] | null>(null)

function askRestore(snapshot: (typeof snapshots.value)[number]) {
  selected.value = snapshot
  restoreOpen.value = true
}

function confirmRestore() {
  restoreOpen.value = false
  toast.add({
    title: 'Restore queued',
    description: selected.value?.name,
    icon: 'i-lucide-history',
    color: 'warning'
  })
}

const formatTime = (value: string) =>
  new Date(value).toLocaleString(undefined, {
    day: 'numeric', month: 'short', hour: '2-digit', minute: '2-digit'
  })
</script>

<template>
  <SettingsSection
    title="Scheduled backups"
    description="Snapshots of the stacker configuration database — projects, nodes, keys and settings."
  >
    <template #header-right>
      <USwitch v-model="backup.enabled" label="Enabled" />
    </template>

    <div class="grid gap-4 sm:grid-cols-2" :class="backup.enabled ? undefined : 'opacity-50'">
      <UFormField label="Frequency">
        <USelect v-model="backup.schedule" :items="schedules" :disabled="!backup.enabled" class="w-full" />
      </UFormField>

      <UFormField label="Run at" :hint="backup.schedule === 'hourly' ? 'Ignored when hourly' : undefined">
        <UInput
          v-model="backup.time"
          type="time"
          :disabled="!backup.enabled || backup.schedule === 'hourly'"
          class="w-full"
        />
      </UFormField>

      <UFormField label="Retention" hint="Days">
        <UInput
          v-model.number="backup.retentionDays"
          type="number"
          :disabled="!backup.enabled"
          class="w-full"
        />
      </UFormField>
    </div>

    <div class="mt-4 space-y-1">
      <USwitch
        v-model="backup.includeVolumes"
        :disabled="!backup.enabled"
        label="Include named volumes"
        description="Much larger snapshots, but service data is recoverable."
      />
      <USwitch
        v-model="backup.encrypt"
        :disabled="!backup.enabled"
        label="Encrypt at rest"
        description="AES-256 using a key derived from the server secret."
      />
    </div>

    <template #footer>
      <UButton label="Discard" color="neutral" variant="ghost" :disabled="!dirty" @click="discard" />
      <UButton label="Save" icon="i-lucide-save" :disabled="!dirty" @click="save" />
    </template>
  </SettingsSection>

  <SettingsSection title="Destination" description="Where snapshots are written.">
    <div class="grid gap-3 sm:grid-cols-2">
      <button
        v-for="option in destinations"
        :key="option.value"
        type="button"
        class="flex items-center gap-2.5 rounded-md border p-3 text-sm transition-colors"
        :class="backup.destination === option.value
          ? 'border-primary bg-primary/5 text-highlighted'
          : 'border-default text-toned hover:bg-elevated/40'"
        @click="backup.destination = option.value"
      >
        <UIcon :name="option.icon" class="size-4" />
        {{ option.label }}
        <UIcon
          v-if="backup.destination === option.value"
          name="i-lucide-check"
          class="ms-auto size-4 text-primary"
        />
      </button>
    </div>

    <div v-if="backup.destination === 's3'" class="mt-4 grid gap-4 sm:grid-cols-2">
      <UFormField label="Bucket">
        <UInput v-model="backup.s3.bucket" class="w-full" />
      </UFormField>

      <UFormField label="Region">
        <UInput v-model="backup.s3.region" class="w-full" />
      </UFormField>

      <UFormField label="Endpoint" class="sm:col-span-2" hint="For MinIO, R2 or Spaces">
        <UInput v-model="backup.s3.endpoint" class="w-full" />
      </UFormField>

      <UFormField label="Access key ID">
        <UInput v-model="backup.s3.accessKeyId" class="w-full" />
      </UFormField>

      <UFormField label="Secret access key" hint="Write-only">
        <UInput v-model="backup.s3.secretAccessKey" type="password" placeholder="••••••••" class="w-full" />
      </UFormField>
    </div>

    <UFormField v-else label="Directory" class="mt-4">
      <UInput v-model="backup.localPath" class="w-full max-w-xl font-mono text-xs" />
    </UFormField>

    <template #footer>
      <UButton label="Test destination" color="neutral" variant="subtle" icon="i-lucide-plug-zap" disabled />
      <UButton label="Save" icon="i-lucide-save" :disabled="!dirty" @click="save" />
    </template>
  </SettingsSection>

  <SettingsSection title="Snapshots" description="Kept for the retention window, then pruned.">
    <template #header-right>
      <UButton
        label="Back up now"
        icon="i-lucide-database-backup"
        size="sm"
        :loading="running"
        @click="runNow"
      />
    </template>

    <div
      v-for="snapshot in snapshots"
      :key="snapshot.id"
      class="flex flex-wrap items-center gap-3 border-t border-default py-2.5 text-sm first:border-t-0"
    >
      <UBadge
        :label="snapshot.status"
        :color="statusColor[snapshot.status as SnapshotStatus]"
        variant="subtle"
        size="sm"
      />
      <span class="font-mono text-xs text-toned">{{ snapshot.name }}</span>
      <span class="text-xs text-dimmed">{{ snapshot.size }}</span>
      <span class="ms-auto text-xs text-dimmed">{{ formatTime(snapshot.at) }}</span>
      <UButton
        label="Restore"
        icon="i-lucide-history"
        color="neutral"
        variant="ghost"
        size="sm"
        :disabled="snapshot.status !== 'complete'"
        @click="askRestore(snapshot)"
      />
    </div>
  </SettingsSection>

  <UModal
    v-model:open="restoreOpen"
    title="Restore snapshot"
    :description="`This replaces the current stacker configuration with ${selected?.name}. Running services are not touched.`"
  >
    <template #footer>
      <div class="flex w-full justify-end gap-2">
        <UButton label="Cancel" color="neutral" variant="ghost" @click="restoreOpen = false" />
        <UButton label="Restore" color="warning" icon="i-lucide-history" @click="confirmRestore" />
      </div>
    </template>
  </UModal>
</template>
