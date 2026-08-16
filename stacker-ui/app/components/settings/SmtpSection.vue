<script setup lang="ts">
/**
 * SMTP tab: the outbound mail server every stacker email is sent through —
 * notifications, password resets, invites.
 *
 * Placeholder: nothing is persisted, the buttons just toast.
 */
const toast = useToast()

const smtp = reactive({
  enabled: true,
  host: 'smtp.postmarkapp.com',
  port: 587,
  encryption: 'starttls',
  username: 'stacker',
  password: '',
  fromName: 'Stacker',
  fromEmail: 'no-reply@stacker.dev'
})

const encryptions = [
  { label: 'STARTTLS', value: 'starttls' },
  { label: 'SSL / TLS', value: 'tls' },
  { label: 'None', value: 'none' }
]

const saved = ref(JSON.stringify(smtp))
const dirty = computed(() => JSON.stringify(smtp) !== saved.value)

function save() {
  saved.value = JSON.stringify(smtp)
  toast.add({ title: 'SMTP settings saved', icon: 'i-lucide-check-circle', color: 'success' })
}

function discard() {
  Object.assign(smtp, JSON.parse(saved.value))
}

/* ---- test send ---- */

const testTo = ref('adharshmk96@gmail.com')
const testing = ref(false)

async function sendTest() {
  testing.value = true
  // Placeholder: a short wait so the pending state is visible.
  await new Promise(resolve => setTimeout(resolve, 700))
  testing.value = false

  toast.add({
    title: 'Test email sent',
    description: testTo.value,
    icon: 'i-lucide-mail-check',
    color: 'success'
  })
}

/** Last delivery attempts, newest first. */
const log = [
  { id: 'm1', to: 'adharshmk96@gmail.com', subject: 'Deployment failed · api-gateway', status: 'delivered', at: '2026-08-16T07:41:00Z' },
  { id: 'm2', to: 'adharshmk96@gmail.com', subject: 'Node offline · node-02', status: 'delivered', at: '2026-08-15T22:03:00Z' },
  { id: 'm3', to: 'ops@example.com', subject: 'You have been invited to stacker', status: 'bounced', at: '2026-08-14T11:19:00Z' }
] as const

const statusColor = {
  delivered: 'success',
  bounced: 'error',
  queued: 'neutral'
} as const

const formatTime = (value: string) =>
  new Date(value).toLocaleString(undefined, {
    day: 'numeric', month: 'short', hour: '2-digit', minute: '2-digit'
  })
</script>

<template>
  <SettingsSection
    title="SMTP server"
    description="Every email stacker sends — notifications, invites, password resets — goes through this server."
  >
    <template #header-right>
      <USwitch v-model="smtp.enabled" label="Enabled" />
    </template>

    <div class="grid gap-4 sm:grid-cols-2" :class="smtp.enabled ? undefined : 'opacity-50'">
      <UFormField label="Host" class="sm:col-span-2">
        <UInput v-model="smtp.host" :disabled="!smtp.enabled" placeholder="smtp.example.com" class="w-full" />
      </UFormField>

      <UFormField label="Port">
        <UInput v-model.number="smtp.port" type="number" :disabled="!smtp.enabled" class="w-full" />
      </UFormField>

      <UFormField label="Encryption">
        <USelect
          v-model="smtp.encryption"
          :items="encryptions"
          :disabled="!smtp.enabled"
          class="w-full"
        />
      </UFormField>

      <UFormField label="Username">
        <UInput v-model="smtp.username" :disabled="!smtp.enabled" class="w-full" />
      </UFormField>

      <UFormField label="Password" hint="Write-only">
        <UInput
          v-model="smtp.password"
          type="password"
          :disabled="!smtp.enabled"
          placeholder="••••••••"
          class="w-full"
        />
      </UFormField>
    </div>

    <template #footer>
      <UButton label="Discard" color="neutral" variant="ghost" :disabled="!dirty" @click="discard" />
      <UButton label="Save" icon="i-lucide-save" :disabled="!dirty" @click="save" />
    </template>
  </SettingsSection>

  <SettingsSection
    title="Sender"
    description="What recipients see in the From line."
  >
    <div class="grid gap-4 sm:grid-cols-2">
      <UFormField label="From name">
        <UInput v-model="smtp.fromName" :disabled="!smtp.enabled" class="w-full" />
      </UFormField>

      <UFormField label="From address">
        <UInput v-model="smtp.fromEmail" type="email" :disabled="!smtp.enabled" class="w-full" />
      </UFormField>
    </div>

    <p class="mt-4 rounded-md border border-default bg-elevated/40 p-3 font-mono text-xs text-toned">
      {{ smtp.fromName }} &lt;{{ smtp.fromEmail }}&gt;
    </p>

    <template #footer>
      <UButton label="Save" icon="i-lucide-save" :disabled="!dirty" @click="save" />
    </template>
  </SettingsSection>

  <SettingsSection
    title="Send a test email"
    description="Confirms the credentials above before you rely on them."
  >
    <div class="flex flex-wrap items-end gap-3">
      <UFormField label="Recipient" class="min-w-64 flex-1">
        <UInput v-model="testTo" type="email" :disabled="!smtp.enabled" class="w-full" />
      </UFormField>
      <UButton
        label="Send test"
        icon="i-lucide-send"
        :loading="testing"
        :disabled="!smtp.enabled || !testTo"
        @click="sendTest"
      />
    </div>
  </SettingsSection>

  <SettingsSection
    title="Recent emails"
    description="The last delivery attempts from this server."
  >
    <div
      v-for="entry in log"
      :key="entry.id"
      class="flex flex-wrap items-center gap-3 border-t border-default py-2.5 text-sm first:border-t-0"
    >
      <UBadge
        :label="entry.status"
        :color="statusColor[entry.status]"
        variant="subtle"
        size="sm"
      />
      <span class="font-mono text-xs text-dimmed">{{ entry.to }}</span>
      <span class="min-w-0 flex-1 truncate text-muted">{{ entry.subject }}</span>
      <span class="text-xs text-dimmed">{{ formatTime(entry.at) }}</span>
    </div>
  </SettingsSection>
</template>
