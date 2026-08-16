<script setup lang="ts">
/** Theme is wired to the real color mode; the rest is placeholder. */
const colorMode = useColorMode()

const modes = [
  { value: 'light', label: 'Light', icon: 'i-lucide-sun' },
  { value: 'dark', label: 'Dark', icon: 'i-lucide-moon' },
  { value: 'system', label: 'System', icon: 'i-lucide-monitor' }
]

const density = ref('comfortable')
const timeFormat = ref('24h')
</script>

<template>
  <SettingsSection title="Theme" description="Applies to this browser only.">
    <div class="grid gap-3 sm:grid-cols-3">
      <button
        v-for="mode in modes"
        :key="mode.value"
        type="button"
        class="flex items-center gap-2.5 rounded-md border p-3 text-sm transition-colors"
        :class="colorMode.preference === mode.value
          ? 'border-primary bg-primary/5 text-highlighted'
          : 'border-default text-toned hover:bg-elevated/40'"
        @click="colorMode.preference = mode.value"
      >
        <UIcon :name="mode.icon" class="size-4" />
        {{ mode.label }}
        <UIcon
          v-if="colorMode.preference === mode.value"
          name="i-lucide-check"
          class="ms-auto size-4 text-primary"
        />
      </button>
    </div>
  </SettingsSection>

  <SettingsSection title="Display" description="How dense the tables and lists are.">
    <div class="grid max-w-md gap-4">
      <UFormField label="Density">
        <URadioGroup
          v-model="density"
          orientation="horizontal"
          :items="[
            { label: 'Comfortable', value: 'comfortable' },
            { label: 'Compact', value: 'compact' }
          ]"
        />
      </UFormField>

      <UFormField label="Time format">
        <URadioGroup
          v-model="timeFormat"
          orientation="horizontal"
          :items="[
            { label: '24-hour', value: '24h' },
            { label: '12-hour', value: '12h' }
          ]"
        />
      </UFormField>
    </div>
  </SettingsSection>
</template>
