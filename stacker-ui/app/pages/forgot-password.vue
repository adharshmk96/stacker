<script setup lang="ts">
/**
 * Stacker has no mail transport, so the reset link is written to the server log
 * instead of sent. The panel says so plainly — otherwise the operator would sit
 * waiting for an email that is never coming.
 */
definePageMeta({ layout: 'auth' })
useHead({ title: 'Forgot password · Stacker' })

const { forgotPassword } = useAuth()

const email = ref('')
const pending = ref(false)
const error = ref<string | null>(null)
const sent = ref(false)

async function submit() {
  if (!email.value.trim() || pending.value) return

  pending.value = true
  error.value = null

  try {
    await forgotPassword(email.value.trim())
    // The server answers the same either way, so no address is confirmed here.
    sent.value = true
  } catch (err: any) {
    error.value = err.message
  } finally {
    pending.value = false
  }
}
</script>

<template>
  <div>
    <h1 class="text-base font-semibold text-highlighted">Forgot password</h1>

    <template v-if="sent">
      <p class="mt-1 text-sm text-muted">
        If that address has an account, a reset link has been written to the
        stacker server log. It is valid for one hour.
      </p>

      <div class="mt-4 rounded-md border border-default bg-elevated/50 p-3">
        <p class="font-mono text-[11px] uppercase tracking-[0.12em] text-dimmed">
          Find it with
        </p>
        <code class="mt-1.5 block font-mono text-xs text-highlighted">
          docker logs stacker | grep "reset link"
        </code>
      </div>

      <UButton
        to="/login"
        label="Back to sign in"
        icon="i-lucide-arrow-left"
        color="neutral"
        variant="ghost"
        block
        class="mt-5"
      />
    </template>

    <template v-else>
      <p class="mt-1 text-sm text-muted">
        Stacker cannot send mail, so the reset link goes to the server log.
      </p>

      <form class="mt-6 space-y-4" @submit.prevent="submit">
        <UFormField label="Email">
          <UInput
            v-model="email"
            type="email"
            autocomplete="email"
            autofocus
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
          label="Send reset link"
          icon="i-lucide-mail"
          block
          :loading="pending"
          :disabled="!email.trim()"
        />
      </form>

      <div class="mt-5 border-t border-default pt-4 text-center">
        <ULink to="/login" class="text-sm text-muted hover:text-highlighted">
          Back to sign in
        </ULink>
      </div>
    </template>
  </div>
</template>
