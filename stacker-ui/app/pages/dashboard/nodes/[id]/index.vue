<script setup lang="ts">
const route = useRoute()
await navigateTo(`/dashboard/nodes/${String(route.params.id)}/monitoring`, { replace: true })
</script>
