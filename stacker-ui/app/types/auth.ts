export interface User {
  id: string
  name: string
  username: string
  email: string
  createdAt: string
  updatedAt: string
}

/** `/api/auth/status` — false means the install has no account yet. */
export interface AuthStatus {
  registered: boolean
}

/** What register and login answer with. */
export interface LoginResult {
  token: string
  user: User
  sessionId: string
  expiresAt: string
}

/** One signed-in device. `current` marks the one making the request. */
export interface Session {
  id: string
  userId: string
  userAgent: string
  ip: string
  createdAt: string
  lastSeenAt: string
  expiresAt: string
  current: boolean
}

export interface RegisterPayload {
  name: string
  username: string
  email: string
  password: string
}

export interface LoginPayload {
  /** Email or username — the server accepts either. */
  identifier: string
  password: string
}

export interface ProfilePayload {
  name: string
  username: string
  email: string
}

export interface ChangePasswordPayload {
  currentPassword: string
  newPassword: string
}
