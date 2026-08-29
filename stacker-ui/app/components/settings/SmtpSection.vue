<script setup lang="ts">
/**
 * SMTP tab: outbound mail server for password resets and notifications.
 */
const toast = useToast()
const api = useApi()

type SmtpSettings = {
  enabled: boolean
  host: string
  port: number
  encryption: string
  username: string
  hasPassword: boolean
  fromName: string
  fromEmail: string
}

const smtp = reactive({
  enabled: false,
  host: '',
  port: 587,
  encryption: 'starttls',
  username: '',
  password: '',
  fromName: 'Stacker',
  fromEmail: ''
})

const saved = ref('')
const loading = ref(true)
const saving = ref(false)
const hasStoredPassword = ref(false)

const dirty = computed(() => JSON.stringify(smtpPayload()) !== saved.value)

function smtpPayload() {
  return {
    enabled: smtp.enabled,
    host: smtp.host,
    port: smtp.port,
    encryption: smtp.encryption,
    username: smtp.username,
    password: smtp.password,
    fromName: smtp.fromName,
    fromEmail: smtp.fromEmail
  }
}

onMounted(async () => {
  try {
    const data = await api.get<SmtpSettings>('/settings/smtp')
    smtp.enabled = data.enabled
    smtp.host = data.host
    smtp.port = data.port || 587
    smtp.encryption = data.encryption || 'starttls'
    smtp.username = data.username
    smtp.password = ''
    smtp.fromName = data.fromName
    smtp.fromEmail = data.fromEmail
    hasStoredPassword.value = data.hasPassword
    saved.value = JSON.stringify(smtpPayload())
  } catch (err) {
    toast.add({
      title: 'Could not load SMTP settings',
      description: err instanceof Error ? err.message : undefined,
      color: 'error'
    })
  } finally {
    loading.value = false
  }
})

const encryptions = [
  { label: 'STARTTLS', value: 'starttls' },
  { label: 'SSL / TLS', value: 'tls' },
  { label: 'None', value: 'none' }
]

async function save() {
  saving.value = true
  try {
    const data = await api.put<SmtpSettings>('/settings/smtp', smtpPayload())
    smtp.password = ''
    smtp.enabled = data.enabled
    smtp.host = data.host
    smtp.port = data.port
    smtp.encryption = data.encryption
    smtp.username = data.username
    smtp.fromName = data.fromName
    smtp.fromEmail = data.fromEmail
    hasStoredPassword.value = data.hasPassword
    saved.value = JSON.stringify(smtpPayload())
    toast.add({ title: 'SMTP settings saved', icon: 'i-lucide-check-circle', color: 'success' })
  } catch (err) {
    toast.add({
      title: 'Could not save SMTP settings',
      description: err instanceof Error ? err.message : undefined,
      color: 'error'
    })
  } finally {
    saving.value = false
  }
}

function discard() {
  Object.assign(smtp, JSON.parse(saved.value))
  smtp.password = ''
}

const testTo = ref('')
const testing = ref(false)

async function sendTest() {
  testing.value = true
  try {
    await api.post('/settings/smtp/test', { to: testTo.value })
    toast.add({
      title: 'Test email sent',
      description: testTo.value,
      icon: 'i-lucide-mail-check',
      color: 'success'
    })
  } catch (err) {
    toast.add({
      title: 'Could not send test email',
      description: err instanceof Error ? err.message : undefined,
      color: 'error'
    })
  } finally {
    testing.value = false
  }
}

</script>

<template>
  <div v-if="loading" class="flex justify-center py-12">
    <UIcon name="i-lucide-loader-circle" class="size-6 animate-spin text-dimmed" />
  </div>

  <template v-else>
    <SettingsSection
      title="SMTP server"
      description="Every email stacker sends — notifications and password resets — goes through this server."
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
            value-key="value"
            :disabled="!smtp.enabled"
            class="w-full"
          />
        </UFormField>

        <UFormField label="Username">
          <UInput v-model="smtp.username" :disabled="!smtp.enabled" class="w-full" />
        </UFormField>

        <UFormField label="Password" :hint="hasStoredPassword && !smtp.password ? 'Leave blank to keep the stored password' : 'Write-only'">
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
        <UButton label="Discard" color="neutral" variant="ghost" :disabled="!dirty || saving" @click="discard" />
        <UButton label="Save" icon="i-lucide-save" :loading="saving" :disabled="!dirty" @click="save" />
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
        {{ smtp.fromName }} &lt;{{ smtp.fromEmail || 'no-reply@example.com' }}&gt;
      </p>
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
  </template>
</template>
