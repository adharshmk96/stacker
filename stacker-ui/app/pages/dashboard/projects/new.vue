<script setup lang="ts">
import type { ProjectPayload } from '~/types/project'

useHead({ title: 'New project · Stacker' })

const { create } = useProjects()
const { enqueue } = useDeployments()
const toast = useToast()
const router = useRouter()

function onSubmit(payload: ProjectPayload, deploy: boolean) {
  const project = create(payload)

  if (deploy) {
    // Placeholder: no build runs, the deployment is queued so the list moves.
    const env = project.environments[0]!
    enqueue(project.id, project.name, env.name)
    toast.add({
      title: 'Project saved and queued',
      description: `${project.name} · ${env.name}`,
      icon: 'i-lucide-rocket',
      color: 'success'
    })
    return router.push('/dashboard/deployments')
  }

  toast.add({
    title: 'Project saved',
    description: project.name,
    icon: 'i-lucide-check-circle',
    color: 'success'
  })
  router.push(`/dashboard/projects/${project.id}/overview`)
}
</script>

<template>
  <UDashboardPanel id="project-new" :ui="{ body: 'relative' }">
    <template #header>
      <UDashboardNavbar title="New project">
        <template #leading>
          <UDashboardSidebarCollapse />
        </template>

        <template #right>
          <UButton
            label="All projects"
            icon="i-lucide-arrow-left"
            color="neutral"
            variant="ghost"
            to="/dashboard/projects"
          />
        </template>
      </UDashboardNavbar>
    </template>

    <template #body>
      <div class="stacker-grid pointer-events-none absolute inset-0" />

      <div class="mx-auto w-full max-w-4xl shrink-0">
        <ProjectForm @submit="onSubmit" @cancel="router.push('/dashboard/projects')" />
      </div>
    </template>
  </UDashboardPanel>
</template>
