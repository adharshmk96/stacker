<script setup lang="ts">
/**
 * The "erase everything" confirmation. It takes a typed word rather than a
 * plain Confirm because the button does not just sign the operator out — it
 * drops the database, nodes and keys included, and there is no undo.
 */
const open = defineModel<boolean>('open', { default: false })

const { resetAllData } = useAuth()
const toast = useToast()

const CONFIRM_WORD = 'RESET'

const typed = ref('')
const resetting = ref(false)

// Clear the field on every open so a previous confirmation cannot be reused.
watch(open, (value) => {
  if (value) typed.value = ''
})

const canConfirm = computed(() => typed.value.trim().toUpperCase() === CONFIRM_WORD)

async function onConfirm() {
  if (!canConfirm.value) return

  resetting.value = true

  try {
    await resetAllData()
    open.value = false
    toast.add({
      title: 'All data reset',
      description: 'Stacker is back at first run. Create an account to continue.',
      icon: 'i-lucide-rotate-ccw'
    })
    // The account is gone with everything else, so registration is open again.
    await navigateTo('/register')
  } catch (error) {
    toast.add({
      title: 'Could not reset data',
      description: error instanceof Error ? error.message : undefined,
      icon: 'i-lucide-circle-alert',
      color: 'error'
    })
  } finally {
    resetting.value = false
  }
}
</script>

<template>
  <UModal v-model:open="open" title="Reset all data">
    <template #body>
      <p class="text-sm text-muted">
        This empties the stacker database. Every node, SSH key and session is
        deleted, along with your account — the install goes back to first run
        and asks you to register again. The servers themselves are untouched,
        but stacker will no longer know about them. This cannot be undone.
      </p>

      <UFormField
        class="mt-4"
        :label="`Type ${CONFIRM_WORD} to confirm`"
      >
        <UInput v-model="typed" autocomplete="off" class="w-full" />
      </UFormField>
    </template>

    <template #footer>
      <div class="flex w-full justify-end gap-2">
        <UButton label="Cancel" color="neutral" variant="ghost" @click="open = false" />
        <UButton
          label="Reset all data"
          icon="i-lucide-rotate-ccw"
          color="error"
          :loading="resetting"
          :disabled="!canConfirm"
          @click="onConfirm"
        />
      </div>
    </template>
  </UModal>
</template>
