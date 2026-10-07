import { useMemo } from 'react'
import { getRealm, getUserRoles, getUsername, logout, switchOrganisation } from '../keycloak'
import { config } from '../config'
import { can, type Capability } from '../lib/roles'

export interface AuthUser {
  username: string
  roles: string[]
  realm: string
  realmLabel: string
}

/**
 * Current user and role checks. Keycloak is initialised before the app
 * renders, so values are read from the singleton.
 */
export function useAuth() {
  const roles = getUserRoles()
  const realm = getRealm()
  const key = roles.join(',')
  return useMemo(() => {
    const user: AuthUser = {
      username: getUsername(),
      roles,
      realm,
      realmLabel: config.realms.find((r) => r.id === realm)?.label ?? realm,
    }
    return {
      user,
      can: (cap: Capability) => can(roles, cap),
      logout,
      switchOrganisation,
      multiRealm: config.realms.length > 1,
    }
    // roles array identity changes per call; key captures its content
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [key, realm])
}
