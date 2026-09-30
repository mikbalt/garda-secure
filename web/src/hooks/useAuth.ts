import { getUsername, getUserRoles, logout } from '../keycloak'

export interface AuthUser {
  username: string
  roles: string[]
}

/**
 * Returns the current authenticated user's info and a logout function.
 * Reads directly from the Keycloak singleton — no React state needed
 * since Keycloak is initialized before the app renders.
 */
export function useAuth() {
  const user: AuthUser = {
    username: getUsername(),
    roles: getUserRoles(),
  }

  return { user, logout }
}
