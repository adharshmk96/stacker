<script setup lang="ts">
import type { DropdownMenuItem } from '@nuxt/ui'

defineProps<{ collapsed?: boolean }>()

const colorMode = useColorMode()

const modes = [
  { value: 'light', label: 'Light', icon: 'i-lucide-sun' },
  { value: 'dark', label: 'Dark', icon: 'i-lucide-moon' },
  { value: 'system', label: 'System', icon: 'i-lucide-monitor' }
] as const

// The menu only renders once opened, so reading the resolved preference here
// cannot cause a hydration mismatch.
const items = computed<DropdownMenuItem[][]>(() => [
  [
    {
      label: 'adharsh',
      avatar: { icon: 'i-lucide-circle-user' },
      type: 'label'
    }
  ],
  [
    {
      label: 'Theme',
      icon: 'i-lucide-palette',
      children: modes.map(mode => ({
        label: mode.label,
        icon: mode.icon,
        type: 'checkbox' as const,
        checked: colorMode.preference === mode.value,
        // Re-selecting the active mode would otherwise uncheck it.
        onUpdateChecked: (checked: boolean) => {
          if (checked) colorMode.preference = mode.value
        },
        onSelect: (event: Event) => event.preventDefault()
      }))
    },
    { label: 'Settings', icon: 'i-lucide-settings', disabled: true }
  ],
  [
    { label: 'Sign out', icon: 'i-lucide-log-out', disabled: true }
  ]
])
</script>

<template>
  <UDropdownMenu
    :items="items"
    :content="{ align: 'start', side: 'top' }"
    :ui="{ content: 'w-48' }"
  >
    <UButton
      icon="i-lucide-circle-user"
      color="neutral"
      variant="ghost"
      :label="collapsed ? undefined : 'Account'"
      :trailing-icon="collapsed ? undefined : 'i-lucide-chevrons-up-down'"
      class="w-full"
      :class="collapsed ? undefined : 'justify-start'"
      :ui="{ trailingIcon: 'ms-auto text-dimmed' }"
      :block="collapsed"
      aria-label="Account"
    />
  </UDropdownMenu>
</template>
