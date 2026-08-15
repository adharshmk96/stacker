<script setup lang="ts">
import type { Environment } from '~/types/project'

/** Hostnames the proxy routes to this environment. Edits in place. */
const props = defineProps<{ environment: Environment }>()

const tlsItems = [
  { label: 'Let\'s Encrypt', value: 'auto' },
  { label: 'Custom certificate', value: 'custom' },
  { label: 'No TLS', value: 'none' }
]

function addDomain() {
  props.environment.domains.push(blankDomain())
}

function removeDomain(id: string) {
  props.environment.domains = props.environment.domains.filter(domain => domain.id !== id)
}
</script>

<template>
  <div class="space-y-4">
    <div class="flex items-start justify-between gap-3">
      <p class="text-sm text-muted">
        Each host is published on the swarm's ingress and routed to a service in the compose file.
      </p>
      <UButton
        label="Add domain"
        icon="i-lucide-plus"
        size="sm"
        color="neutral"
        variant="subtle"
        @click="addDomain"
      />
    </div>

    <div
      v-if="!environment.domains.length"
      class="flex flex-col items-center gap-3 rounded-lg border border-dashed border-default py-10"
    >
      <UIcon name="i-lucide-globe" class="size-8 text-dimmed" />
      <p class="text-sm text-muted">
        No domain yet — <span class="font-mono">{{ environment.name }}</span> is only reachable
        inside the swarm.
      </p>
      <UButton label="Add a domain" icon="i-lucide-plus" size="sm" @click="addDomain" />
    </div>

    <div
      v-for="domain in environment.domains"
      :key="domain.id"
      class="rounded-lg border border-default p-4"
    >
      <div class="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
        <UFormField label="Host" class="lg:col-span-2">
          <UInput
            v-model="domain.host"
            icon="i-lucide-globe"
            placeholder="shop.acme.dev"
            class="w-full font-mono"
          />
        </UFormField>

        <UFormField label="Service">
          <UInput v-model="domain.service" placeholder="web" class="w-full font-mono" />
        </UFormField>

        <UFormField label="Port">
          <UInput v-model.number="domain.port" type="number" class="w-full font-mono" />
        </UFormField>

        <UFormField label="Certificate">
          <USelectMenu v-model="domain.tls" :items="tlsItems" value-key="value" class="w-full" />
        </UFormField>

        <div class="flex items-end justify-between gap-3 sm:col-span-2 lg:col-span-3">
          <USwitch v-model="domain.redirectWww" label="Redirect www to the bare host" />
          <UButton
            icon="i-lucide-trash-2"
            color="error"
            variant="ghost"
            size="sm"
            aria-label="Remove domain"
            @click="removeDomain(domain.id)"
          />
        </div>
      </div>

      <p v-if="domain.host" class="mt-3 border-t border-default pt-3 font-mono text-xs text-dimmed">
        {{ domain.tls === 'none' ? 'http' : 'https' }}://{{ domain.host }}
        → {{ domain.service || 'service' }}:{{ domain.port }}
      </p>
    </div>
  </div>
</template>
