<script setup lang="ts">
/** Where deployment and node events are delivered. Placeholder data only. */
const toast = useToast()

const events = reactive({
  deploySucceeded: false,
  deployFailed: true,
  nodeOffline: true,
  weeklyDigest: false
})

const webhook = ref('https://hooks.slack.com/services/T000/B000/xxxx')

function save() {
  toast.add({ title: 'Notification settings saved', icon: 'i-lucide-check-circle', color: 'success' })
}
</script>

<template>
  <SettingsSection
    title="Email notifications"
    description="Sent to the address on your account."
  >
    <div class="space-y-1">
      <USwitch v-model="events.deployFailed" label="Deployment failed" />
      <USwitch v-model="events.deploySucceeded" label="Deployment succeeded" />
      <USwitch v-model="events.nodeOffline" label="A node goes offline" />
      <USwitch v-model="events.weeklyDigest" label="Weekly digest" />
    </div>

    <template #footer>
      <UButton label="Save" icon="i-lucide-save" @click="save" />
    </template>
  </SettingsSection>

  <SettingsSection
    title="Webhook"
    description="stacker POSTs a JSON payload for every event you enabled above."
  >
    <UFormField label="Endpoint URL">
      <UInput v-model="webhook" class="w-full max-w-xl" placeholder="https://…" />
    </UFormField>

    <template #footer>
      <UButton label="Send test event" color="neutral" variant="subtle" icon="i-lucide-send" disabled />
      <UButton label="Save" icon="i-lucide-save" @click="save" />
    </template>
  </SettingsSection>
</template>
