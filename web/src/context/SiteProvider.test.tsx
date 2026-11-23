import { render, screen, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { beforeEach, describe, expect, it, vi } from 'vitest'

// TAQ-41: a platform-admin-only user has no sites of its own (D-032) and the
// API answers GET /sites with 403 for it, so the provider must not ask.
const apiGet = vi.fn()
vi.mock('../api/client', async (orig) => {
  const real = await orig<typeof import('../api/client')>()
  return { ...real, apiGet: (...a: unknown[]) => apiGet(...a) }
})
let roles: string[] = []
vi.mock('../keycloak', async (orig) => {
  const real = await orig<typeof import('../keycloak')>()
  return { ...real, getUserRoles: () => roles }
})

const { SiteProvider } = await import('./SiteProvider')
const { useSite } = await import('./site')

function Probe() {
  const { sites, isLoading, error } = useSite()
  return <p>{`sites=${sites.length} loading=${isLoading} error=${error ? 'yes' : 'no'}`}</p>
}

function renderProvider() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={qc}>
      <SiteProvider>
        <Probe />
      </SiteProvider>
    </QueryClientProvider>
  )
}

describe('SiteProvider', () => {
  beforeEach(() => apiGet.mockReset())

  it('loads /sites for a realm role', async () => {
    roles = ['supervisor']
    apiGet.mockResolvedValue([{ id: 's1', name: 'HQ' }])
    renderProvider()
    await waitFor(() => expect(screen.getByText('sites=1 loading=false error=no')).toBeInTheDocument())
    expect(apiGet).toHaveBeenCalledWith('/sites')
  })

  it('does not call /sites for a platform-admin-only user', async () => {
    roles = ['platform-admin']
    renderProvider()
    expect(await screen.findByText('sites=0 loading=false error=no')).toBeInTheDocument()
    expect(apiGet).not.toHaveBeenCalled()
  })

  it('loads /sites for an operator who also holds a realm role', async () => {
    roles = ['admin', 'platform-admin']
    apiGet.mockResolvedValue([])
    renderProvider()
    await waitFor(() => expect(apiGet).toHaveBeenCalledWith('/sites'))
  })
})
