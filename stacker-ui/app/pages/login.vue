<script setup lang="ts">
definePageMeta({ layout: 'auth' })
useHead({ title: 'Sign in · Stacker' })

const { login } = useAuth()
const route = useRoute()

const form = reactive({ identifier: '', password: '' })
const pending = ref(false)
const error = ref<string | null>(null)

const canSubmit = computed(() => !!form.identifier.trim() && !!form.password)

async function submit() {
  if (!canSubmit.value || pending.value) return

  pending.value = true
  error.value = null

  try {
    await login({ identifier: form.identifier.trim(), password: form.password })
    // Back to wherever the guard interrupted, or the default landing page.
    await navigateTo(String(route.query.redirect ?? '/dashboard/nodes'))
  } catch (err: any) {
    error.value = err.message
    form.password = ''
  } finally {
    pending.value = false
  }
}
</script>

<template>
  <div>
    <h1 class="text-base font-semibold text-highlighted">Sign in</h1>
    <p class="mt-1 text-sm text-muted">Use your email or username.</p>

    <form class="mt-6 space-y-4" @submit.prevent="submit">
      <UFormField label="Email or username">
        <UInput
          v-model="form.identifier"
          autocomplete="username"
          autofocus
          class="w-full"
        />
      </UFormField>

      <UFormField label="Password">
        <UInput
          v-model="form.password"
          type="password"
          autocomplete="current-password"
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
        label="Sign in"
        icon="i-lucide-log-in"
        block
        :loading="pending"
        :disabled="!canSubmit"
      />
    </form>

    <div class="mt-5 border-t border-default pt-4 text-center">
      <ULink to="/forgot-password" class="text-sm text-muted hover:text-highlighted">
        Forgot your password?
      </ULink>
    </div>
  </div>
</template>
