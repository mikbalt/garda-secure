import { useMemo } from 'react'
import { getRealm, getUserRoles, getUsername, logout, switchOrganisation } from '../keycloak'
import { config } from '../config'
import { can, canSeeMedia, type Capability, type MediaKind } from '../lib/roles'
import { useT } from '../i18n/context'
import { dropPushForAccountSwitch } from '../lib/push'

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
  const t = useT()
  const roles = getUserRoles()
  const realm = getRealm()
  const key = roles.join(',')
  const demoLabel = t('demo.orgLabel')
  return useMemo(() => {
    const user: AuthUser = {
      username: getUsername(),
      roles,
      realm,
      realmLabel: config.demo ? demoLabel : config.realms.find((r) => r.id === realm)?.label ?? realm,
    }
    return {
      user,
      can: (cap: Capability) => can(roles, cap),
      canSeeMedia: (kind: MediaKind | undefined) => canSeeMedia(roles, kind),
      logout,
      /** Another organisation = another account: this browser stops receiving the current user's push first (logout keeps it). */
      switchOrganisation: () => {
        void dropPushForAccountSwitch().finally(switchOrganisation)
      },
      multiRealm: config.realms.length > 1,
    }
    // roles array identity changes per call; key captures its content
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [key, realm, demoLabel])
}
