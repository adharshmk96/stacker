<script setup lang="ts">
import type { Session } from '~/types/auth'

/**
 * Account tab. The profile form, the password form and the session list each
 * submit on their own — changing a name never touches credentials, and
 * revoking a device is not a save.
 */
const {
  user,
  updateProfile,
  changePassword,
  sessions: fetchSessions,
  revokeSession
} = useAuth()
const toast = useToast()

/* ---- profile ---- */

const profile = reactive({ name: '', email: '', username: '' })
const savedProfile = ref('')
const savingProfile = ref(false)

// The user arrives from /auth/me, which may still be in flight on first paint.
// This watches `user` specifically rather than using watchEffect: an effect
// would also track the form it writes to, and re-baseline on every keystroke.
watch(user, (value) => {
  if (!value) return
  Object.assign(profile, {
    name: value.name,
    email: value.email,
    username: value.username
  })
  savedProfile.value = JSON.stringify(profile)
}, { immediate: true })

const profileDirty = computed(() => JSON.stringify(profile) !== savedProfile.value)

async function saveProfile() {
  savingProfile.value = true

  try {
    await updateProfile({ ...profile })
    savedProfile.value = JSON.stringify(profile)
    toast.add({ title: 'Profile updated', icon: 'i-lucide-check-circle', color: 'success' })
  } catch (error) {
    toast.add({
      title: 'Could not update profile',
      description: error instanceof Error ? error.message : undefined,
      icon: 'i-lucide-circle-alert',
      color: 'error'
    })
  } finally {
    savingProfile.value = false
  }
}

function resetProfile() {
  Object.assign(profile, JSON.parse(savedProfile.value))
}

/* ---- password ---- */

const password = reactive({ current: '', next: '', confirm: '' })
const savingPassword = ref(false)

const mismatch = computed(() => !!password.confirm && password.next !== password.confirm)
const tooShort = computed(() => !!password.next && password.next.length < 8)

const canSubmitPassword = computed(() =>
  !!password.current && password.next.length >= 8 && !mismatch.value)

async function submitPassword() {
  if (!canSubmitPassword.value) return

  savingPassword.value = true

  try {
    await changePassword({ currentPassword: password.current, newPassword: password.next })
    password.current = ''
    password.next = ''
    password.confirm = ''
    // The server revoked every other device, so the list is stale now.
    await loadSessions()
    toast.add({
      title: 'Password changed',
      description: 'Other devices have been signed out.',
      icon: 'i-lucide-shield-check',
      color: 'success'
    })
  } catch (error) {
    toast.add({
      title: 'Could not change password',
      description: error instanceof Error ? error.message : undefined,
      icon: 'i-lucide-circle-alert',
      color: 'error'
    })
  } finally {
    savingPassword.value = false
  }
}

/* ---- sessions ---- */

const sessions = ref<Session[]>([])
const loadingSessions = ref(false)
const revoking = ref<string | null>(null)

async function loadSessions() {
  loadingSessions.value = true
  try {
    sessions.value = await fetchSessions()
  } catch {
    sessions.value = []
  } finally {
    loadingSessions.value = false
  }
}

onMounted(loadSessions)

async function revoke(session: Session) {
  revoking.value = session.id

  try {
    await revokeSession(session.id)
    sessions.value = sessions.value.filter(item => item.id !== session.id)
    toast.add({ title: 'Session revoked', icon: 'i-lucide-log-out' })
  } catch (error) {
    toast.add({
      title: 'Could not revoke session',
      description: error instanceof Error ? error.message : undefined,
      icon: 'i-lucide-circle-alert',
      color: 'error'
    })
  } finally {
    revoking.value = null
  }
}

/** The user agent in full is unreadable in a row; the product name is enough. */
function deviceLabel(agent: string) {
  if (!agent) return 'Unknown device'
  const browser = /(Firefox|Edg|Chrome|Safari)\/[\d.]+/.exec(agent)?.[1]
  const platform = /\(([^;)]+)/.exec(agent)?.[1]
  return [browser === 'Edg' ? 'Edge' : browser, platform].filter(Boolean).join(' · ') || agent
}

function when(iso: string) {
  return new Date(iso).toLocaleString()
}

/* ---- reset ---- */

const resetOpen = ref(false)
</script>

<template>
  <SettingsSection title="Profile" description="How you appear across stacker.">
    <div class="grid gap-4 sm:grid-cols-2">
      <UFormField label="Name">
        <UInput v-model="profile.name" class="w-full" />
      </UFormField>

      <UFormField label="Username" hint="Also accepted when signing in">
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
        :loading="savingProfile"
        :disabled="!profileDirty"
        @click="saveProfile"
      />
    </template>
  </SettingsSection>

  <SettingsSection
    title="Password"
    description="At least 8 characters. Every other device is signed out; you stay signed in here."
  >
    <div class="grid max-w-md gap-4">
      <UFormField label="Current password">
        <UInput
          v-model="password.current"
          type="password"
          autocomplete="current-password"
          class="w-full"
        />
      </UFormField>

      <UFormField
        label="New password"
        :error="tooShort ? 'Use at least 8 characters.' : undefined"
      >
        <UInput
          v-model="password.next"
          type="password"
          autocomplete="new-password"
          class="w-full"
        />
      </UFormField>

      <UFormField
        label="Confirm new password"
        :error="mismatch ? 'Passwords do not match.' : undefined"
      >
        <UInput
          v-model="password.confirm"
          type="password"
          autocomplete="new-password"
          class="w-full"
        />
      </UFormField>
    </div>

    <template #footer>
      <UButton
        label="Change password"
        icon="i-lucide-shield-check"
        :loading="savingPassword"
        :disabled="!canSubmitPassword"
        @click="submitPassword"
      />
    </template>
  </SettingsSection>

  <SettingsSection
    title="Sessions"
    description="Devices signed in to this install. Revoking one takes effect immediately."
  >
    <template #header-right>
      <UButton
        icon="i-lucide-refresh-cw"
        color="neutral"
        variant="ghost"
        size="xs"
        :loading="loadingSessions"
        aria-label="Refresh sessions"
        @click="loadSessions"
      />
    </template>

    <p v-if="!sessions.length" class="text-sm text-muted">
      {{ loadingSessions ? 'Loading…' : 'No active sessions.' }}
    </p>

    <ul v-else class="divide-y divide-default">
      <li
        v-for="session in sessions"
        :key="session.id"
        class="flex items-center justify-between gap-3 py-3 first:pt-0 last:pb-0"
      >
        <div class="min-w-0">
          <p class="flex items-center gap-2 text-sm text-highlighted">
            {{ deviceLabel(session.userAgent) }}
            <UBadge
              v-if="session.current"
              label="This device"
              color="success"
              variant="subtle"
              size="sm"
            />
          </p>
          <p class="mt-0.5 truncate font-mono text-[11px] text-dimmed">
            {{ session.ip || 'unknown ip' }} · last seen {{ when(session.lastSeenAt) }}
          </p>
        </div>

        <UButton
          v-if="!session.current"
          label="Revoke"
          color="neutral"
          variant="subtle"
          size="xs"
          :loading="revoking === session.id"
          @click="revoke(session)"
        />
      </li>
    </ul>
  </SettingsSection>

  <SettingsSection
    title="Reset all data"
    description="Empties the database — nodes, SSH keys, sessions and your account. Stacker returns to first run."
    danger
  >
    <UButton
      label="Reset all data"
      icon="i-lucide-rotate-ccw"
      color="error"
      variant="subtle"
      @click="resetOpen = true"
    />
  </SettingsSection>

  <SettingsResetDataModal v-model:open="resetOpen" />
</template>
