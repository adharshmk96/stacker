<script setup lang="ts">
/** The project page always lives on a tab, so `/projects/:id` is just a jump. */
definePageMeta({
  redirect: to => `/dashboard/projects/${to.params.id}/overview`
})
</script>

<template>
  <div />
</template>
