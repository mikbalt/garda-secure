// Runtime configuration. In the Docker image this file is regenerated at
// container start from API_URL, KEYCLOAK_URL, KEYCLOAK_CLIENT_ID,
// KEYCLOAK_REALMS, IDLE_LOGOUT_MINUTES, DEMO and DEMO_FORCE
// (docker-entrypoint.d/40-securepatrol-config.sh). In development it stays empty and the app falls back to the
// VITE_* variables from .env (see .env.example).
window.__SP_CONFIG__ = window.__SP_CONFIG__ || {}
