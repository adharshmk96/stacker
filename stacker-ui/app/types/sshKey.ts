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
  createdAt: string
}

/**
 * Keys are create-and-delete only: renaming or re-keying an existing entry
 * would silently break every VPS already trusting it.
 */
export type SshKeyPayload = Pick<SshKey, 'name' | 'type'>
