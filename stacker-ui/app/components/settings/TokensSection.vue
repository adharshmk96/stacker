<script setup lang="ts">
/** API tokens for the stacker CLI and CI. Placeholder data only. */
const toast = useToast()

const tokens = ref([
  { id: 't1', name: 'CLI · macbook', prefix: 'stk_live_9f21', lastUsed: '2026-08-15T08:12:00Z', scope: 'Full access' },
  { id: 't2', name: 'GitHub Actions', prefix: 'stk_live_4c07', lastUsed: '2026-08-12T19:40:00Z', scope: 'Deploy only' }
])

function revoke(id: string) {
  tokens.value = tokens.value.filter(token => token.id !== id)
  toast.add({ title: 'Token revoked', icon: 'i-lucide-shield-off', color: 'warning' })
}

const formatTime = (value: string) =>
  new Date(value).toLocaleString(undefined, {
    day: 'numeric', month: 'short', hour: '2-digit', minute: '2-digit'
  })
</script>

<template>
  <SettingsSection
    title="API tokens"
    description="Used by the stacker CLI and CI pipelines. The secret is shown once at creation."
  >
    <template #header-right>
      <UButton label="New token" icon="i-lucide-plus" size="sm" disabled />
    </template>

    <p v-if="!tokens.length" class="text-sm text-dimmed">No active tokens.</p>

    <div
      v-for="token in tokens"
      :key="token.id"
      class="flex flex-wrap items-center gap-3 border-t border-default py-3 first:border-t-0"
    >
      <div class="leading-tight">
        <p class="text-sm font-medium text-highlighted">{{ token.name }}</p>
        <p class="font-mono text-xs text-dimmed">{{ token.prefix }}••••••••</p>
      </div>
      <UBadge :label="token.scope" color="neutral" variant="subtle" size="sm" />
      <span class="ms-auto text-xs text-dimmed">Last used {{ formatTime(token.lastUsed) }}</span>
      <UButton
        label="Revoke"
        color="error"
        variant="ghost"
        size="sm"
        @click="revoke(token.id)"
      />
    </div>
  </SettingsSection>
</template>
