import { parseDocument } from 'yaml'

export interface ComposeServicePreview {
  name: string
  image: string
  builds: boolean
  ports: string[]
  replicas?: number
}

export interface ComposePreview {
  services: ComposeServicePreview[]
  error?: string
}

/**
 * Reads only the small, useful part of a Compose document for the create form.
 * The server remains the authority that validates a compose file on deploy.
 */
export function composePreview(source: string): ComposePreview {
  if (!source.trim()) return { services: [] }

  try {
    const document = parseDocument(source)
    if (document.errors.length) return { services: [], error: document.errors[0]!.message }

    const value = document.toJS()
    const services = isRecord(value) && isRecord(value.services) ? value.services : null
    if (!services) return { services: [], error: 'Add a top-level services: section.' }

    return {
      services: Object.entries(services)
        .filter((entry): entry is [string, Record<string, unknown>] => isRecord(entry[1]))
        .map(([name, service]) => ({
          name,
          image: stringValue(service.image),
          builds: service.build !== undefined && service.build !== null,
          ports: listValue(service.ports),
          replicas: replicasOf(service)
        }))
        .sort((a, b) => a.name.localeCompare(b.name))
    }
  } catch (error) {
    return { services: [], error: error instanceof Error ? error.message : 'Could not read this compose file.' }
  }
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value)
}

function stringValue(value: unknown): string {
  return typeof value === 'string' ? value : ''
}

function listValue(value: unknown): string[] {
  return Array.isArray(value) ? value.map(item => String(item)) : []
}

function replicasOf(service: Record<string, unknown>): number | undefined {
  if (!isRecord(service.deploy) || typeof service.deploy.replicas !== 'number') return undefined
  return service.deploy.replicas
}
