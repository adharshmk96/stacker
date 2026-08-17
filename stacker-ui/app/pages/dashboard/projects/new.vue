<script setup lang="ts">
import type { ProjectPayload } from '~/types/project'

useHead({ title: 'New project · Stacker' })

const { create, deploy } = useProjects()
const { track } = useDeployments()
const toast = useToast()
const router = useRouter()

const saving = ref(false)

async function onSubmit(payload: ProjectPayload, withDeploy: boolean) {
  saving.value = true
  try {
    const project = await create(payload)

    if (!withDeploy) {
      toast.add({
        title: 'Project saved',
        description: project.name,
        icon: 'i-lucide-check-circle',
        color: 'success'
      })
      return await router.push(`/dashboard/projects/${project.id}/overview`)
    }

    // The project exists either way. A deploy that will not start is reported
    // on its own rather than as a failed save, and the user lands on the project
    // so they can read the reason and try again.
    const environment = project.environments[0]!
    try {
      const deployment = await deploy(project.id, environment.id, 'Created from the new project form')
      track(deployment)
      toast.add({
        title: 'Deploying',
        description: `${project.name} · ${environment.name}`,
        icon: 'i-lucide-rocket',
        color: 'success'
      })
      await router.push('/dashboard/deployments')
    } catch (err: any) {
      toast.add({
        title: 'Project saved, but the deploy did not start',
        description: err.message,
        color: 'error'
      })
      await router.push(`/dashboard/projects/${project.id}/overview`)
    }
  } catch (err: any) {
    toast.add({ title: 'Could not save the project', description: err.message, color: 'error' })
  } finally {
    saving.value = false
  }
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
        <ProjectForm :saving="saving" @submit="onSubmit" @cancel="router.push('/dashboard/projects')" />
      </div>
    </template>
  </UDashboardPanel>
</template>
