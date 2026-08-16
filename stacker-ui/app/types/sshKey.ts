/** Algorithms stacker can generate a keypair for */
export type SshKeyType = 'ed25519' | 'rsa'

export interface SshKey {
  id: string
  name: string
  type: SshKeyType
  /** OpenSSH public key line — the only half stacker ever shows */
  publicKey: string
  /** SHA256 fingerprint, as printed by `ssh-keygen -lf` */
  fingerprint: string
  /** Protected install-wide key created on first run. */
  isDefault: boolean
  createdAt: string
}

/**
 * User-created keys are create-and-delete only. The protected default key can
 * be rotated explicitly after warning about existing connections.
 */
export type SshKeyPayload = Pick<SshKey, 'name' | 'type'>
