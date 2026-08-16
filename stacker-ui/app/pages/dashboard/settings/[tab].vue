<script setup lang="ts">
import type { NavigationMenuItem } from '@nuxt/ui'

/**
 * Settings. Each tab is its own route (`/dashboard/settings/git-provider`), so
 * a section is linkable and the back button walks between them — the same shape
 * as the project detail tabs, only with the menu down the left.
 *
 * Placeholder UI: no API calls, every form is local state.
 */

// Reuse the component across tabs so the vertical menu does not remount.
definePageMeta({ key: 'settings' })

const route = useRoute()

const tabs = [
  { key: 'account', label: 'Account', icon: 'i-lucide-circle-user', group: 0 },
  { key: 'git-provider', label: 'Git Provider', icon: 'i-lucide-git-branch', group: 1 },
  { key: 'registries', label: 'Registries', icon: 'i-lucide-container', group: 1 },
  { key: 'tokens', label: 'API Tokens', icon: 'i-lucide-key-square', group: 1 },
  { key: 'notifications', label: 'Notifications', icon: 'i-lucide-bell', group: 2 },
  { key: 'smtp', label: 'SMTP', icon: 'i-lucide-mail', group: 2 },
  { key: 'backup', label: 'Backup', icon: 'i-lucide-database-backup', group: 3 },
  { key: 'appearance', label: 'Appearance', icon: 'i-lucide-palette', group: 3 }
] as const

const tab = computed(() => String(route.params.tab))

const current = computed(() => tabs.find(item => item.key === tab.value))

// An unknown segment is a 404 rather than a silently blank panel.
watchEffect(() => {
  if (!current.value) {
    throw createError({ statusCode: 404, statusMessage: 'Unknown settings tab', fatal: true })
  }
})

const toItem = (item: (typeof tabs)[number]): NavigationMenuItem => ({
  label: item.label,
  icon: item.icon,
  to: `/dashboard/settings/${item.key}`
})

const groups = computed<NavigationMenuItem[][]>(() =>
  [0, 1, 2, 3].map(group => tabs.filter(item => item.group === group).map(toItem)))

useHead(() => ({ title: `${current.value?.label ?? 'Settings'} · Stacker` }))
</script>

<template>
  <UDashboardPanel id="settings" :ui="{ body: 'relative' }">
    <template #header>
      <UDashboardNavbar title="Settings">
        <template #leading>
          <UDashboardSidebarCollapse />
        </template>

        <template #trailing>
          <UBadge
            :label="current?.label ?? ''"
            color="neutral"
            variant="subtle"
            class="font-mono text-[11px]"
          />
        </template>
      </UDashboardNavbar>
    </template>

    <template #body>
      <div class="stacker-grid pointer-events-none absolute inset-0" />

      <div class="flex w-full shrink-0 flex-col gap-6 lg:flex-row lg:items-start">
        <!-- tab rail -->
        <nav class="lg:sticky lg:top-0 lg:w-56 lg:shrink-0">
          <UNavigationMenu
            v-for="(items, index) in groups"
            :key="index"
            orientation="vertical"
            :items="items"
            class="stacker-nav"
            :class="index ? 'mt-4' : undefined"
            :ui="{ list: 'space-y-1', link: 'px-2.5 py-2 gap-3 text-[13px]', linkLeadingIcon: 'size-4.5' }"
          />
        </nav>

        <!-- panel -->
        <div class="min-w-0 flex-1 space-y-6">
          <SettingsAccountSection v-if="tab === 'account'" />
          <SettingsGitProviderSection v-else-if="tab === 'git-provider'" />
          <SettingsRegistriesSection v-else-if="tab === 'registries'" />
          <SettingsTokensSection v-else-if="tab === 'tokens'" />
          <SettingsNotificationsSection v-else-if="tab === 'notifications'" />
          <SettingsSmtpSection v-else-if="tab === 'smtp'" />
          <SettingsBackupSection v-else-if="tab === 'backup'" />
          <SettingsAppearanceSection v-else-if="tab === 'appearance'" />
        </div>
      </div>
    </template>
  </UDashboardPanel>
</template>
