<script setup lang="ts">
import type { SwarmCreateForm, SwarmCreatePayload, SwarmNodeRef } from '~/types/swarm'

/**
 * The one create form each resource offers.
 *
 * It is generated from the resource's field list rather than written nine
 * times: every form here is a handful of plain inputs whose only real
 * difference is which of them docker needs.
 */

const props = defineProps<{
  form: SwarmCreateForm
  /** Offered by the `node` field, for the resources created on one host */
  nodes: SwarmNodeRef[]
  loading?: boolean
}>()

const emit = defineEmits<{ confirm: [SwarmCreatePayload] }>()

const open = defineModel<boolean>('open', { default: false })

const values = ref<Record<string, string>>({})

// Reopening should be a clean form, not the last attempt.
watch(open, (isOpen) => {
  if (!isOpen) return
  values.value = Object.fromEntries(props.form.fields.map(field => [field.key, '']))

  // With one node in the swarm there is nothing to choose, so it is filled in.
  const nodeField = props.form.fields.find(field => field.type === 'node')
  if (nodeField && props.nodes.length) values.value[nodeField.key] = props.nodes[0]!.id
})

const nodeItems = computed(() =>
  props.nodes.map(node => ({ label: node.name, value: node.id })))

const complete = computed(() =>
  props.form.fields.every(field => !field.required || String(values.value[field.key] ?? '').trim()))

function submit() {
  const payload: SwarmCreatePayload = {}

  for (const field of props.form.fields) {
    const value = String(values.value[field.key] ?? '').trim()
    if (!value) continue

    if (field.key === 'replicas') {
      payload.replicas = Number(value)
      continue
    }
    // Every remaining field is a string on the payload; `replicas` above is the
    // only one that is not.
    (payload as Record<string, string>)[field.key] = value
  }

  emit('confirm', payload)
}
</script>

<template>
  <UModal v-model:open="open" :title="form.title" :ui="{ content: 'max-w-2xl' }">
    <template #body>
      <div class="space-y-4">
        <p class="text-sm text-muted">{{ form.description }}</p>

        <UFormField
          v-for="field in form.fields"
          :key="field.key"
          :label="field.label"
          :help="field.help"
          :required="field.required"
        >
          <USelect
            v-if="field.type === 'node'"
            v-model="values[field.key]"
            :items="nodeItems"
            value-key="value"
            icon="i-lucide-server"
            class="w-full"
          />
          <UTextarea
            v-else-if="field.type === 'textarea'"
            v-model="values[field.key]"
            :placeholder="field.placeholder"
            :rows="10"
            class="w-full font-mono text-xs"
          />
          <UInput
            v-else
            v-model="values[field.key]"
            :type="field.type === 'number' ? 'number' : 'text'"
            :placeholder="field.placeholder"
            class="w-full"
          />
        </UFormField>
      </div>
    </template>

    <template #footer>
      <div class="flex w-full justify-end gap-2">
        <UButton label="Cancel" color="neutral" variant="ghost" @click="open = false" />
        <UButton
          :label="form.label"
          :icon="form.icon"
          :loading="loading"
          :disabled="!complete"
          @click="submit"
        />
      </div>
    </template>
  </UModal>
</template>
