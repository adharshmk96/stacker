<script setup lang="ts">
import type { Environment } from '~/types/project'

/**
 * The environment picker shared by every per-environment tab. The selection
 * lives on the page, so switching tabs keeps you on the same environment.
 */
const props = defineProps<{
  environments: Environment[]
  /** Hide the add/remove controls where the list is not editable */
  editable?: boolean
}>()

const emit = defineEmits<{ add: [name: string], remove: [id: string] }>()

const selected = defineModel<string>({ required: true })

const draftName = ref('')

function onAdd() {
  const name = draftName.value.trim()
  if (!name) return
  if (props.environments.some(env => env.name.toLowerCase() === name.toLowerCase())) return

  emit('add', name)
  draftName.value = ''
}
</script>

<template>
  <div class="flex flex-wrap items-center gap-2">
    <div class="flex flex-wrap gap-1.5">
      <button
        v-for="env in environments"
        :key="env.id"
        type="button"
        class="group flex items-center gap-1.5 rounded-md border px-2.5 py-1.5 font-mono text-[13px] transition-colors"
        :class="env.id === selected
          ? 'border-accented bg-elevated text-highlighted'
          : 'border-default text-muted hover:bg-elevated/40'"
        @click="selected = env.id"
      >
        <UIcon
          name="i-lucide-layers"
          class="size-3.5"
          :class="env.id === selected ? 'text-primary' : 'text-dimmed'"
        />
        {{ env.name || 'unnamed' }}
        <UIcon
          v-if="editable && environments.length > 1"
          name="i-lucide-x"
          class="size-3.5 text-dimmed opacity-0 transition-opacity hover:text-error group-hover:opacity-100"
          @click.stop="emit('remove', env.id)"
        />
      </button>
    </div>

    <div v-if="editable" class="flex gap-1.5">
      <UInput
        v-model="draftName"
        size="sm"
        placeholder="staging"
        class="w-32"
        @keydown.enter.prevent="onAdd"
      />
      <UButton
        icon="i-lucide-plus"
        size="sm"
        color="neutral"
        variant="subtle"
        aria-label="Add environment"
        :disabled="!draftName.trim()"
        @click="onAdd"
      />
    </div>
  </div>
</template>
