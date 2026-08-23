<script setup lang="ts">
import type { EnvVar, Environment } from '~/types/project'

/** Name, branch, variables and secrets of one environment. Edits in place. */
const props = defineProps<{
  environment: Environment
  /** Shown as the placeholder on the branch override */
  defaultBranch?: string
  /** Hide the branch field when the source is a pasted compose file */
  showBranch?: boolean
  /**
   * Show the first host inline. The create form does, so a new project can be
   * reachable without a second stop; the Domains tab owns the full list.
   */
  showHost?: boolean
  /** The create form requires one routable host for every environment. */
  hostRequired?: boolean
  /** Services parsed from the pasted compose file for the host target picker. */
  serviceItems?: { label: string, value: string }[]
}>()

/* ---- inline host ---- */

/**
 * The one domain the create form edits. It is only appended once something is
 * typed, so an environment that needs no host keeps an empty list.
 */
function ensureDomain() {
  if (!props.environment.domains.length) props.environment.domains.push(blankDomain())
  return props.environment.domains[0]!
}

/** Drop the row again when it is left completely blank. */
function pruneDomain() {
  const domain = props.environment.domains[0]
  if (domain && !domain.host.trim() && !domain.service.trim()) {
    props.environment.domains.shift()
  }
}

function field<K extends 'host' | 'service' | 'port'>(key: K) {
  return computed({
    get: () => props.environment.domains[0]?.[key] ?? blankDomain()[key],
    set: (value) => {
      ensureDomain()[key] = value as never
      pruneDomain()
    }
  })
}

const host = field('host')
const service = field('service')
const port = field('port')

const extraDomains = computed(() => Math.max(0, props.environment.domains.length - 1))

/**
 * The fields are opt-in: an environment with no host shows a single button, so
 * nothing about the create form suggests a domain is needed to deploy.
 */
const hostOpen = ref(props.hostRequired || props.environment.domains.length > 0)

if (props.hostRequired) ensureDomain()

function openHost() {
  ensureDomain()
  hostOpen.value = true
}

function closeHost() {
  props.environment.domains.shift()
  hostOpen.value = false
}

function addPair(list: EnvVar[]) {
  list.push({ key: '', value: '' })
}

function removePair(list: EnvVar[], index: number) {
  list.splice(index, 1)
}

/**
 * Bulk paste of a `.env` file — by far the fastest way to fill a new
 * environment, and the shape people already have on their clipboard.
 */
const bulkOpen = ref(false)
const bulkText = ref('')
const bulkTarget = ref<'variables' | 'secrets'>('variables')

function openBulk(target: 'variables' | 'secrets') {
  bulkTarget.value = target
  bulkText.value = ''
  bulkOpen.value = true
}

function applyBulk() {
  const parsed = bulkText.value
    .split('\n')
    .map(line => line.trim())
    .filter(line => line && !line.startsWith('#'))
    .map((line): EnvVar => {
      const separator = line.indexOf('=')
      if (separator === -1) return { key: line, value: '' }
      return {
        key: line.slice(0, separator).trim(),
        // Strip one layer of quotes, the way docker compose does.
        value: line.slice(separator + 1).trim().replace(/^["']|["']$/g, '')
      }
    })
    .filter(pair => pair.key)

  const list = props.environment[bulkTarget.value]
  for (const pair of parsed) {
    const existing = list.find(item => item.key === pair.key)
    if (existing) existing.value = pair.value
    else list.push(pair)
  }

  bulkOpen.value = false
}
</script>

<template>
  <div class="space-y-6">
    <div class="grid gap-4 sm:grid-cols-2">
      <UFormField label="Environment name" name="env.name">
        <UInput v-model="environment.name" class="w-full font-mono" />
      </UFormField>

      <UFormField
        v-if="showBranch"
        label="Branch"
        :hint="`Defaults to ${defaultBranch || 'the project branch'}`"
      >
        <UInput
          v-model="environment.branch"
          icon="i-lucide-git-branch"
          :placeholder="defaultBranch || 'main'"
          class="w-full font-mono"
        />
      </UFormField>
    </div>

    <!-- host -->
    <div v-if="showHost">
      <div class="mb-2 flex items-center justify-between gap-2">
        <h3 class="text-xs font-semibold uppercase tracking-[0.08em] text-dimmed">
          Host <span v-if="!hostRequired" class="font-normal normal-case tracking-normal">· optional</span>
        </h3>
        <UButton
          v-if="hostOpen && !hostRequired"
          label="Remove"
          icon="i-lucide-x"
          size="xs"
          color="neutral"
          variant="ghost"
          @click="closeHost"
        />
      </div>

      <UButton
        v-if="!hostOpen"
        label="Add a host"
        icon="i-lucide-globe"
        size="sm"
        color="neutral"
        variant="subtle"
        @click="openHost"
      />

      <div v-else class="grid gap-4 sm:grid-cols-4">
        <UFormField label="Domain" name="env.host" class="sm:col-span-2" :required="hostRequired">
          <UInput
            v-model="host"
            icon="i-lucide-globe"
            :placeholder="`${environment.name || 'app'}.acme.dev`"
            class="w-full font-mono"
          />
        </UFormField>

        <UFormField label="Service" name="env.service">
          <USelectMenu
            v-if="serviceItems?.length"
            v-model="service"
            :items="serviceItems"
            value-key="value"
            searchable
            placeholder="Search services"
            class="w-full font-mono"
          />
          <UInput v-else v-model="service" placeholder="web" class="w-full font-mono" />
        </UFormField>

        <UFormField label="Port">
          <UInput v-model.number="port" type="number" class="w-full font-mono" />
        </UFormField>
      </div>

      <p class="mt-2 text-xs text-dimmed">
        <template v-if="hostOpen && host">
          https://{{ host }} → {{ service || 'service' }}:{{ port }} · certificate from Let's
          Encrypt.
        </template>
        <template v-else-if="!hostRequired">
          Without one, <span class="font-mono">{{ environment.name || 'this environment' }}</span>
          still deploys — it is just reachable only inside the swarm.
        </template>
        More hosts and certificate options live on the project's Domains tab<template v-if="extraDomains">
          — {{ extraDomains }} more {{ extraDomains === 1 ? 'host is' : 'hosts are' }} configured there</template>.
      </p>
    </div>

    <!-- variables -->
    <div>
      <div class="mb-2 flex items-center justify-between gap-2">
        <h3 class="text-xs font-semibold uppercase tracking-[0.08em] text-dimmed">
          Environment variables
        </h3>
        <div class="flex gap-1">
          <UButton
            label="Paste .env"
            icon="i-lucide-clipboard-paste"
            size="xs"
            color="neutral"
            variant="ghost"
            @click="openBulk('variables')"
          />
          <UButton
            label="Add"
            icon="i-lucide-plus"
            size="xs"
            color="neutral"
            variant="subtle"
            @click="addPair(environment.variables)"
          />
        </div>
      </div>

      <p v-if="!environment.variables.length" class="text-sm text-dimmed">No variables yet.</p>

      <div
        v-for="(pair, index) in environment.variables"
        :key="`var-${index}`"
        class="mb-1.5 flex gap-1.5"
      >
        <UInput v-model="pair.key" size="sm" placeholder="KEY" class="w-1/3 font-mono" />
        <UInput v-model="pair.value" size="sm" placeholder="value" class="flex-1 font-mono" />
        <UButton
          icon="i-lucide-trash-2"
          size="sm"
          color="neutral"
          variant="ghost"
          aria-label="Remove variable"
          @click="removePair(environment.variables, index)"
        />
      </div>
    </div>

    <!-- secrets -->
    <div>
      <div class="mb-2 flex items-center justify-between gap-2">
        <h3 class="text-xs font-semibold uppercase tracking-[0.08em] text-dimmed">Secrets</h3>
        <div class="flex gap-1">
          <UButton
            label="Paste .env"
            icon="i-lucide-clipboard-paste"
            size="xs"
            color="neutral"
            variant="ghost"
            @click="openBulk('secrets')"
          />
          <UButton
            label="Add"
            icon="i-lucide-plus"
            size="xs"
            color="neutral"
            variant="subtle"
            @click="addPair(environment.secrets)"
          />
        </div>
      </div>

      <p class="mb-2 text-xs text-dimmed">
        Passed to the services like variables, and kept out of deployment logs. The value is
        never shown again after saving — leave it blank to keep the stored one, or remove the
        row to clear it.
      </p>

      <p v-if="!environment.secrets.length" class="text-sm text-dimmed">No secrets yet.</p>

      <div
        v-for="(pair, index) in environment.secrets"
        :key="`secret-${index}`"
        class="mb-1.5 flex gap-1.5"
      >
        <UInput v-model="pair.key" size="sm" placeholder="KEY" class="w-1/3 font-mono" />
        <UInput
          v-model="pair.value"
          size="sm"
          type="password"
          placeholder="value"
          class="flex-1 font-mono"
        />
        <UButton
          icon="i-lucide-trash-2"
          size="sm"
          color="neutral"
          variant="ghost"
          aria-label="Remove secret"
          @click="removePair(environment.secrets, index)"
        />
      </div>
    </div>
  </div>

  <UModal
    v-model:open="bulkOpen"
    :title="bulkTarget === 'secrets' ? 'Paste secrets' : 'Paste variables'"
    description="One KEY=value per line. Existing keys are overwritten, the rest are appended."
  >
    <template #body>
      <UTextarea
        v-model="bulkText"
        :rows="10"
        placeholder="NODE_ENV=production&#10;API_URL=https://api.acme.dev"
        class="w-full font-mono"
        :ui="{ base: 'text-xs' }"
      />
    </template>

    <template #footer>
      <div class="flex w-full justify-end gap-2">
        <UButton label="Cancel" color="neutral" variant="ghost" @click="bulkOpen = false" />
        <UButton label="Add" :disabled="!bulkText.trim()" @click="applyBulk" />
      </div>
    </template>
  </UModal>
</template>
