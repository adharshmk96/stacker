<script setup lang="ts">
/** The signed-out shell: no sidebar, one centred card on the grid backdrop. */
</script>

<template>
  <div class="relative flex min-h-svh items-center justify-center overflow-hidden p-4">
    <div class="stacker-grid pointer-events-none absolute inset-0" />

    <div class="relative w-full max-w-sm">
      <div class="mb-8 flex items-center justify-center gap-2.5">
        <div class="flex size-9 shrink-0 items-center justify-center rounded-md bg-primary/10 ring-1 ring-primary/25">
          <UIcon name="i-lucide-container" class="size-5 text-primary" />
        </div>
        <div class="flex items-baseline gap-2">
          <span class="font-mono text-lg font-semibold tracking-tight text-highlighted">stacker</span>
          <span class="font-mono text-[10px] text-dimmed">v0.1</span>
        </div>
      </div>

      <div class="rounded-lg border border-default bg-default/60 p-6 backdrop-blur">
        <slot />
      </div>
    </div>
  </div>
</template>
