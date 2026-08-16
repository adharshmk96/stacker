<script setup lang="ts">
/** The other half of forgot-password: `?token=…` comes from the logged link. */
definePageMeta({ layout: 'auth' })
useHead({ title: 'Reset password · Stacker' })

const { resetPassword } = useAuth()
const route = useRoute()

const token = computed(() => String(route.query.token ?? ''))

const form = reactive({ password: '', confirm: '' })
const pending = ref(false)
const error = ref<string | null>(null)
const done = ref(false)

const tooShort = computed(() => !!form.password && form.password.length < 8)
const mismatch = computed(() => !!form.confirm && form.password !== form.confirm)

const canSubmit = computed(() =>
  !!token.value && form.password.length >= 8 && !mismatch.value)

async function submit() {
  if (!canSubmit.value || pending.value) return

  pending.value = true
  error.value = null

  try {
    await resetPassword(token.value, form.password)
    // Every session was revoked server-side, so signing in again is the only
    // way on from here.
    done.value = true
  } catch (err: any) {
    error.value = err.message
  } finally {
    pending.value = false
  }
}
</script>

<template>
  <div>
    <h1 class="text-base font-semibold text-highlighted">Set a new password</h1>

    <template v-if="done">
      <p class="mt-1 text-sm text-muted">
        Your password has been changed and every signed-in device was signed out.
      </p>
      <UButton to="/login" label="Sign in" icon="i-lucide-log-in" block class="mt-5" />
    </template>

    <template v-else-if="!token">
      <p class="mt-1 text-sm text-muted">
        This link is missing its token. Request a new one.
      </p>
      <UButton
        to="/forgot-password"
        label="Request a new link"
        icon="i-lucide-mail"
        color="neutral"
        variant="subtle"
        block
        class="mt-5"
      />
    </template>

    <template v-else>
      <p class="mt-1 text-sm text-muted">At least 8 characters.</p>

      <form class="mt-6 space-y-4" @submit.prevent="submit">
        <UFormField
          label="New password"
          :error="tooShort ? 'Use at least 8 characters.' : undefined"
        >
          <UInput
            v-model="form.password"
            type="password"
            autocomplete="new-password"
            autofocus
            class="w-full"
          />
        </UFormField>

        <UFormField
          label="Confirm password"
          :error="mismatch ? 'Passwords do not match.' : undefined"
        >
          <UInput
            v-model="form.confirm"
            type="password"
            autocomplete="new-password"
            class="w-full"
          />
        </UFormField>

        <UAlert
          v-if="error"
          :description="error"
          icon="i-lucide-triangle-alert"
          color="error"
          variant="subtle"
        />

        <UButton
          type="submit"
          label="Reset password"
          icon="i-lucide-shield-check"
          block
          :loading="pending"
          :disabled="!canSubmit"
        />
      </form>
    </template>
  </div>
</template>
