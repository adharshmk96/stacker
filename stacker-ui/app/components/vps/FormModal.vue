<script setup lang="ts">
import type { FormError, FormSubmitEvent } from '@nuxt/ui'
import type { Vps, VpsKeyStatus, VpsPayload } from '~/types/vps'

const props = defineProps<{
  /** Passing a VPS switches the modal to edit mode */
  vps?: Vps | null
}>()

const emit = defineEmits<{ saved: [Vps] }>()

const open = defineModel<boolean>('open', { default: false })

const { sshKeys, create, update, installKey } = useVps()
const toast = useToast()

const isEdit = computed(() => !!props.vps)

const DEFAULT_SSH_PORT = 22

interface State {
  name: string
  ssh: string
  port: number
  sshKeyId: string | undefined
}

const state = reactive<State>(blank())

function blank(): State {
  return { name: '', ssh: '', port: DEFAULT_SSH_PORT, sshKeyId: undefined }
}

/** Key status for the row being edited, updated live by the Install button */
const keyStatus = ref<VpsKeyStatus>('unknown')
const keyMessage = ref('')

// Reset every time the modal opens so a cancelled edit never leaks into the next one.
watch(open, (value) => {
  if (!value) return

  Object.assign(state, props.vps
    ? {
        name: props.vps.name,
        ssh: props.vps.ssh,
        port: props.vps.port,
        sshKeyId: props.vps.sshKeyId
      }
    : blank())

  keyStatus.value = props.vps?.keyStatus ?? 'unknown'
  keyMessage.value = ''
  installOpen.value = false
  password.value = ''
})

// Changing the key or the host invalidates whatever we last verified.
watch(() => [state.sshKeyId, state.ssh, state.port], ([keyId], [prevKeyId]) => {
  if (keyId === undefined && prevKeyId === undefined) return
  keyStatus.value = 'unknown'
  keyMessage.value = ''
})

const sshKeyItems = computed(() => sshKeys.value.map(key => ({ label: key.name, value: key.id })))

// `user@host`, where host is a hostname or an IP.
const SSH_RE = /^[a-z_][a-z0-9_-]*@[a-zA-Z0-9.-]+$/i

const connectionReady = computed(() =>
  SSH_RE.test(state.ssh.trim()) && !!state.sshKeyId && state.port > 0)

/* ---- ssh-copy-id ---- */

const installOpen = ref(false)
const password = ref('')
const installing = ref(false)

async function onInstall() {
  if (!connectionReady.value) return

  installing.value = true

  try {
    const result = await installKey({
      ssh: state.ssh.trim(),
      port: state.port,
      sshKeyId: state.sshKeyId!,
      password: password.value
    })

    keyStatus.value = result.ok ? 'ok' : 'failed'
    keyMessage.value = result.message

    if (result.ok) {
      installOpen.value = false
      password.value = ''
    }
  } catch (error) {
    keyStatus.value = 'failed'
    keyMessage.value = error instanceof Error ? error.message : 'Could not reach the host'
  } finally {
    installing.value = false
  }
}

/* ---- save ---- */

function validate(state: State): FormError[] {
  const errors: FormError[] = []

  if (!state.name.trim()) {
    errors.push({ name: 'name', message: 'Name is required' })
  }

  if (!state.ssh.trim()) {
    errors.push({ name: 'ssh', message: 'SSH connection is required' })
  } else if (!SSH_RE.test(state.ssh.trim())) {
    errors.push({ name: 'ssh', message: 'Use the user@host form, e.g. root@10.0.0.11' })
  }

  if (!Number.isInteger(state.port) || state.port < 1 || state.port > 65535) {
    errors.push({ name: 'port', message: 'Port must be between 1 and 65535' })
  }

  if (!state.sshKeyId) {
    errors.push({ name: 'sshKeyId', message: 'An SSH key is required' })
  }

  return errors
}

const submitting = ref(false)

async function onSubmit(event: FormSubmitEvent<State>) {
  submitting.value = true

  try {
    const payload: VpsPayload = {
      name: event.data.name.trim(),
      ssh: event.data.ssh.trim(),
      port: event.data.port,
      sshKeyId: event.data.sshKeyId!,
      keyStatus: keyStatus.value,
      keyCheckedAt: keyStatus.value === 'unknown' ? undefined : new Date().toISOString()
    }

    const saved = props.vps
      ? await update(props.vps.id, payload)
      : await create(payload)

    toast.add({
      title: isEdit.value ? 'VPS updated' : 'VPS created',
      description: saved.name,
      icon: 'i-lucide-check-circle',
      color: 'success'
    })

    emit('saved', saved)
    open.value = false
  } catch (error) {
    toast.add({
      title: 'Could not save VPS',
      description: error instanceof Error ? error.message : undefined,
      icon: 'i-lucide-circle-alert',
      color: 'error'
    })
  } finally {
    submitting.value = false
  }
}
</script>

<template>
  <UModal
    v-model:open="open"
    :title="isEdit ? 'Edit VPS' : 'Add VPS'"
    :description="isEdit
      ? 'Update the connection details for this server.'
      : 'Register a server so stacker can deploy stacks to it.'"
  >
    <template #body>
      <UForm
        id="vps-form"
        :state="state"
        :validate="validate"
        class="space-y-4"
        @submit="onSubmit"
      >
        <UFormField label="Name" name="name" required>
          <UInput v-model="state.name" placeholder="swarm-manager-1" class="w-full" autofocus />
        </UFormField>

        <div class="flex gap-3">
          <UFormField label="SSH" name="ssh" required class="flex-1">
            <UInput v-model="state.ssh" placeholder="root@10.0.0.11" class="w-full" />
          </UFormField>

          <UFormField label="Port" name="port" class="w-28">
            <UInputNumber v-model="state.port" :min="1" :max="65535" class="w-full" />
          </UFormField>
        </div>

        <USeparator label="Authentication" />

        <UFormField
          label="SSH key"
          name="sshKeyId"
          required
          description="Stacker connects with this key. Install it once and it is used for every deploy afterwards."
        >
          <div class="flex items-center gap-2">
            <USelectMenu
              v-model="state.sshKeyId"
              :items="sshKeyItems"
              value-key="value"
              placeholder="Select an SSH key"
              icon="i-lucide-key-round"
              class="flex-1"
            />

            <UPopover v-model:open="installOpen">
              <UButton
                label="Install"
                icon="i-lucide-download"
                color="neutral"
                variant="subtle"
                :disabled="!connectionReady"
                :title="connectionReady
                  ? 'Run ssh-copy-id to install this key on the host'
                  : 'Enter a valid user@host and pick a key first'"
              />

              <template #content>
                <div class="w-72 space-y-3 p-3">
                  <div>
                    <p class="text-sm font-medium text-highlighted">Install key on host</p>
                    <p class="mt-1 text-xs text-muted">
                      Stacker runs <code class="font-mono">ssh-copy-id</code> once, using this
                      login password. The password is not stored.
                    </p>
                  </div>

                  <UInput
                    v-model="password"
                    type="password"
                    placeholder="Login password"
                    autocomplete="off"
                    class="w-full"
                    @keydown.enter.prevent="onInstall"
                  />

                  <div class="flex justify-end gap-2">
                    <UButton
                      label="Cancel"
                      color="neutral"
                      variant="ghost"
                      size="sm"
                      @click="installOpen = false"
                    />
                    <UButton
                      label="Install"
                      size="sm"
                      :loading="installing"
                      :disabled="!password"
                      @click="onInstall"
                    />
                  </div>
                </div>
              </template>
            </UPopover>
          </div>
        </UFormField>

        <div
          v-if="installing || keyStatus !== 'unknown'"
          class="flex items-start gap-2 rounded-md border px-3 py-2 text-sm"
          :class="installing
            ? 'border-default text-muted'
            : keyStatus === 'ok'
              ? 'border-success/30 bg-success/5 text-success'
              : 'border-error/30 bg-error/5 text-error'"
        >
          <UIcon
            :name="installing
              ? 'i-lucide-loader-circle'
              : keyStatus === 'ok'
                ? 'i-lucide-circle-check'
                : 'i-lucide-circle-x'"
            class="mt-0.5 size-4 shrink-0"
            :class="{ 'animate-spin': installing }"
          />
          <span>
            {{ installing
              ? 'Running ssh-copy-id…'
              : keyMessage || (keyStatus === 'ok'
                ? 'Key authentication works'
                : 'Key authentication failed') }}
          </span>
        </div>
      </UForm>
    </template>

    <template #footer>
      <div class="flex justify-end gap-2 w-full">
        <UButton
          label="Cancel"
          color="neutral"
          variant="ghost"
          @click="open = false"
        />
        <UButton
          type="submit"
          form="vps-form"
          :label="isEdit ? 'Save changes' : 'Create VPS'"
          :loading="submitting"
        />
      </div>
    </template>
  </UModal>
</template>
