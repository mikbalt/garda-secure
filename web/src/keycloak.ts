import Keycloak from 'keycloak-js'

const keycloak = new Keycloak({
  url: import.meta.env.VITE_KEYCLOAK_URL as string,
  realm: import.meta.env.VITE_KEYCLOAK_REALM as string,
  clientId: import.meta.env.VITE_KEYCLOAK_CLIENT_ID as string,
})

/**
 * Initialize Keycloak with login-required flow.
 * Must be called before rendering the app.
 */
export async function initKeycloak(): Promise<boolean> {
  const authenticated = await keycloak.init({
    onLoad: 'login-required',
    checkLoginIframe: false,
    pkceMethod: 'S256',
  })

  // Auto-refresh the access token 60 seconds before it expires.
  keycloak.onTokenExpired = () => {
    keycloak.updateToken(60).catch(() => {
      console.warn('Token refresh failed — logging out')
      keycloak.logout()
    })
  }

  return authenticated
}

export function getToken(): string | undefined {
  return keycloak.token
}

export function getUserRoles(): string[] {
  return keycloak.realmAccess?.roles ?? []
}

export function getUsername(): string {
  return (keycloak.tokenParsed?.['preferred_username'] as string) ?? ''
}

export function logout(): void {
  keycloak.logout()
}

export default keycloak
