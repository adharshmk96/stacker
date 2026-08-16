<script setup lang="ts">
/**
 * Account tab: the profile form and the password form are deliberately separate
 * — each submits on its own, so changing a name never touches credentials.
 *
 * Placeholder only: nothing is persisted, the buttons just toast.
 */
const toast = useToast()

/* ---- profile ---- */

const profile = reactive({
  name: 'Adharsh M',
  email: 'adharshmk96@gmail.com',
  username: 'adharsh'
})

const savedProfile = ref(JSON.stringify(profile))
const profileDirty = computed(() => JSON.stringify(profile) !== savedProfile.value)

function saveProfile() {
  savedProfile.value = JSON.stringify(profile)
  toast.add({ title: 'Profile updated', icon: 'i-lucide-check-circle', color: 'success' })
}

function resetProfile() {
  Object.assign(profile, JSON.parse(savedProfile.value))
}

/* ---- password ---- */

const password = reactive({ current: '', next: '', confirm: '' })

const mismatch = computed(() =>
  !!password.confirm && password.next !== password.confirm)

const tooShort = computed(() => !!password.next && password.next.length < 8)

const canSubmitPassword = computed(() =>
  !!password.current && password.next.length >= 8 && !mismatch.value)

function changePassword() {
  if (!canSubmitPassword.value) return

  password.current = ''
  password.next = ''
  password.confirm = ''
  toast.add({ title: 'Password changed', icon: 'i-lucide-shield-check', color: 'success' })
}
</script>

<template>
  <SettingsSection
    title="Profile"
    description="How you appear across stacker."
  >
    <div class="grid gap-4 sm:grid-cols-2">
      <UFormField label="Name">
        <UInput v-model="profile.name" class="w-full" />
      </UFormField>

      <UFormField label="Username" hint="Used in URLs">
        <UInput v-model="profile.username" class="w-full" />
      </UFormField>

      <UFormField label="Email" class="sm:col-span-2">
        <UInput v-model="profile.email" type="email" class="w-full" />
      </UFormField>
    </div>

    <template #footer>
      <UButton
        label="Discard"
        color="neutral"
        variant="ghost"
        :disabled="!profileDirty"
        @click="resetProfile"
      />
      <UButton
        label="Save changes"
        icon="i-lucide-save"
        :disabled="!profileDirty"
        @click="saveProfile"
      />
    </template>
  </SettingsSection>

  <SettingsSection
    title="Password"
    description="At least 8 characters. You stay signed in on this device."
  >
    <div class="grid max-w-md gap-4">
      <UFormField label="Current password">
        <UInput v-model="password.current" type="password" class="w-full" />
      </UFormField>

      <UFormField
        label="New password"
        :error="tooShort ? 'Use at least 8 characters.' : undefined"
      >
        <UInput v-model="password.next" type="password" class="w-full" />
      </UFormField>

      <UFormField
        label="Confirm new password"
        :error="mismatch ? 'Passwords do not match.' : undefined"
      >
        <UInput v-model="password.confirm" type="password" class="w-full" />
      </UFormField>
    </div>

    <template #footer>
      <UButton
        label="Change password"
        icon="i-lucide-shield-check"
        :disabled="!canSubmitPassword"
        @click="changePassword"
      />
    </template>
  </SettingsSection>

  <SettingsSection
    title="Delete account"
    description="Removes your account and every project it owns. This cannot be undone."
    danger
  >
    <UButton label="Delete account" icon="i-lucide-trash-2" color="error" variant="subtle" disabled />
  </SettingsSection>
</template>
