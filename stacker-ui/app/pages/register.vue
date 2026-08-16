<script setup lang="ts">
/**
 * First-run setup. The guard only routes here while the install has no account,
 * and the server closes the endpoint the moment one exists — stacker is a
 * single-operator tool, so this page is seen exactly once.
 */
definePageMeta({ layout: 'auth' })
useHead({ title: 'Create your account · Stacker' })

const { register } = useAuth()

const form = reactive({ name: '', username: '', email: '', password: '', confirm: '' })
const pending = ref(false)
const error = ref<string | null>(null)

const tooShort = computed(() => !!form.password && form.password.length < 8)
const mismatch = computed(() => !!form.confirm && form.password !== form.confirm)

const canSubmit = computed(() =>
  !!form.name.trim()
  && !!form.username.trim()
  && !!form.email.trim()
  && form.password.length >= 8
  && !mismatch.value)

async function submit() {
  if (!canSubmit.value || pending.value) return

  pending.value = true
  error.value = null

  try {
    await register({
      name: form.name.trim(),
      username: form.username.trim(),
      email: form.email.trim(),
      password: form.password
    })
    await navigateTo('/dashboard/nodes')
  } catch (err: any) {
    error.value = err.message
  } finally {
    pending.value = false
  }
}
</script>

<template>
  <div>
    <h1 class="text-base font-semibold text-highlighted">Create your account</h1>
    <p class="mt-1 text-sm text-muted">
      This is a fresh install. The account you make here is the only one.
    </p>

    <form class="mt-6 space-y-4" @submit.prevent="submit">
      <UFormField label="Name">
        <UInput v-model="form.name" autocomplete="name" autofocus class="w-full" />
      </UFormField>

      <UFormField label="Username" hint="Letters, digits, dot, dash, underscore">
        <UInput v-model="form.username" autocomplete="username" class="w-full" />
      </UFormField>

      <UFormField label="Email">
        <UInput v-model="form.email" type="email" autocomplete="email" class="w-full" />
      </UFormField>

      <UFormField
        label="Password"
        :error="tooShort ? 'Use at least 8 characters.' : undefined"
      >
        <UInput
          v-model="form.password"
          type="password"
          autocomplete="new-password"
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
        label="Create account"
        icon="i-lucide-user-plus"
        block
        :loading="pending"
        :disabled="!canSubmit"
      />
    </form>
  </div>
</template>
