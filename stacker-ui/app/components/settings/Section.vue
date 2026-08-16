<script setup lang="ts">
/** The card every settings panel is built from — title, blurb, body, optional footer. */
defineProps<{
  title: string
  description?: string
  danger?: boolean
}>()
</script>

<template>
  <section
    class="rounded-lg border bg-default/60 p-5 backdrop-blur"
    :class="danger ? 'border-error/40' : 'border-default'"
  >
    <header class="mb-4 flex items-start justify-between gap-3">
      <div>
        <h2 class="text-sm font-semibold text-highlighted">{{ title }}</h2>
        <p v-if="description" class="mt-1 text-sm text-muted">{{ description }}</p>
      </div>
      <slot name="header-right" />
    </header>

    <slot />

    <div v-if="$slots.footer" class="mt-5 flex items-center justify-end gap-2 border-t border-default pt-4">
      <slot name="footer" />
    </div>
  </section>
</template>
