<script setup lang="ts">
import type { FormError, FormSubmitEvent } from '@nuxt/ui'
import type { SshKey, SshKeyType } from '~/types/sshKey'

const emit = defineEmits<{ created: [SshKey] }>()

const open = defineModel<boolean>('open', { default: false })

const { items, create } = useSshKeys()
const toast = useToast()

interface State {
  name: string
  type: SshKeyType
}

const blank = (): State => ({ name: '', type: 'ed25519' })

const state = reactive<State>(blank())

/** Set once generation succeeds — the modal then shows the public key instead of the form. */
const generated = ref<SshKey | null>(null)

// Reset on open so a cancelled run never leaks into the next one.
watch(open, (value) => {
  if (!value) return
  Object.assign(state, blank())
  generated.value = null
})

const typeItems = [
  { label: 'ED25519 — recommended', value: 'ed25519' },
  { label: 'RSA 4096', value: 'rsa' }
]

// Keys are addressed by name in the Nodes menu, so duplicates would be ambiguous.
const NAME_RE = /^[a-z0-9][a-z0-9_-]*$/i

function validate(state: State): FormError[] {
  const errors: FormError[] = []
  const name = state.name.trim()

  if (!name) {
    errors.push({ name: 'name', message: 'Name is required' })
  } else if (!NAME_RE.test(name)) {
    errors.push({ name: 'name', message: 'Use letters, numbers, dashes and underscores only' })
  } else if (items.value.some(key => key.name.toLowerCase() === name.toLowerCase())) {
    errors.push({ name: 'name', message: 'A key with this name already exists' })
  }

  return errors
}

const submitting = ref(false)

async function onSubmit(event: FormSubmitEvent<State>) {
  submitting.value = true

  try {
    const key = await create({ name: event.data.name.trim(), type: event.data.type })
    generated.value = key

    toast.add({
      title: 'SSH key created',
      description: key.name,
      icon: 'i-lucide-check-circle',
      color: 'success'
    })

    emit('created', key)
  } catch (error) {
    toast.add({
      title: 'Could not create SSH key',
      description: error instanceof Error ? error.message : undefined,
      icon: 'i-lucide-circle-alert',
      color: 'error'
    })
  } finally {
    submitting.value = false
  }
}

async function copyPublicKey() {
  if (!generated.value) return
  await navigator.clipboard.writeText(generated.value.publicKey)
  toast.add({ title: 'Public key copied', icon: 'i-lucide-clipboard-check' })
}
</script>

<template>
  <UModal
    v-model:open="open"
    :title="generated ? 'SSH key created' : 'New SSH key'"
    :description="generated
      ? 'The private half stays on the stacker server and is never shown.'
      : 'Stacker generates the keypair and keeps the private half.'"
  >
    <template #body>
      <div v-if="generated" class="space-y-4">
        <div class="flex items-center gap-3">
          <div class="flex size-9 items-center justify-center rounded-md bg-success/10 ring-1 ring-success/25">
            <UIcon name="i-lucide-key-round" class="size-4.5 text-success" />
          </div>
          <div class="leading-tight">
            <p class="font-medium text-highlighted">{{ generated.name }}</p>
            <p class="font-mono text-xs text-dimmed">{{ generated.fingerprint }}</p>
          </div>
        </div>

        <div>
          <p class="mb-1.5 text-sm font-medium text-highlighted">Public key</p>
          <pre class="max-h-32 overflow-auto rounded-md border border-default bg-elevated/50 p-3 font-mono text-xs break-all whitespace-pre-wrap text-toned">{{ generated.publicKey }}</pre>
        </div>

        <p class="text-sm text-muted">
          Install it on a node from the Nodes menu, or add it to
          <code class="font-mono text-xs">~/.ssh/authorized_keys</code> yourself.
        </p>
      </div>

      <UForm
        v-else
        id="ssh-key-form"
        :state="state"
        :validate="validate"
        class="space-y-4"
        @submit="onSubmit"
      >
        <UFormField
          label="Name"
          name="name"
          required
          description="How the key is labelled across stacker. Cannot be changed later."
        >
          <UInput v-model="state.name" placeholder="stacker-deploy" class="w-full" autofocus />
        </UFormField>

        <UFormField label="Type" name="type">
          <USelect v-model="state.type" :items="typeItems" value-key="value" class="w-full" />
        </UFormField>
      </UForm>
    </template>

    <template #footer>
      <div class="flex justify-end gap-2 w-full">
        <template v-if="generated">
          <UButton
            label="Copy public key"
            icon="i-lucide-copy"
            color="neutral"
            variant="subtle"
            @click="copyPublicKey"
          />
          <UButton label="Done" @click="open = false" />
        </template>
        <template v-else>
          <UButton label="Cancel" color="neutral" variant="ghost" @click="open = false" />
          <UButton
            type="submit"
            form="ssh-key-form"
            label="Generate key"
            :loading="submitting"
          />
        </template>
      </div>
    </template>
  </UModal>
</template>
