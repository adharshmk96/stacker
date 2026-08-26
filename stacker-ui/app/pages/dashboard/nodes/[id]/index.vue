<script setup lang="ts">
/** The node page always lives on a tab, so `/nodes/:id` is just a jump. */
definePageMeta({
  redirect: to => `/dashboard/nodes/${to.params.id}/overview`
})
</script>

<template>
  <div />
</template>
