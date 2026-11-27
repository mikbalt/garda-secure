import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { I18nProvider } from '../../i18n/I18nProvider'
import { ProofPackReport } from './ProofPackReport'
import type { Site } from '../../types'

const downloadFile = vi.fn()
vi.mock('../../api/client', () => ({ downloadFile: (...a: unknown[]) => downloadFile(...a) }))
vi.mock('../../config', () => ({ config: { apiUrl: '/api/v1', demo: false, realms: [] } }))
const onError = vi.fn()
vi.mock('../../components/toast/context', () => ({ useDownloadErrorToast: () => onError }))
vi.mock('../../lib/time', async (orig) => ({ ...(await orig<typeof import('../../lib/time')>()), monthIn: () => '2026-11' }))

const site = { id: 's1', name: 'Gate A', timezone: 'Asia/Jakarta', active: true, created_at: '', config_json: { language: 'id' } } as Site

function setup() {
  render(
    <I18nProvider>
      <ProofPackReport site={site} />
    </I18nProvider>,
  )
}

describe('ProofPackReport', () => {
  beforeEach(() => {
    downloadFile.mockReset().mockResolvedValue(undefined)
    onError.mockReset()
  })

  it('downloads the previous month as PDF in the chosen language', async () => {
    setup()
    fireEvent.change(screen.getByRole('combobox'), { target: { value: 'en' } })
    fireEvent.click(screen.getByRole('button', { name: /proof pack \(PDF\)|paket bukti \(PDF\)/i }))
    await waitFor(() => expect(downloadFile).toHaveBeenCalledTimes(1))
    expect(downloadFile).toHaveBeenCalledWith('/sites/s1/proof-pack', { month: '2026-10', lang: 'en', stored: undefined }, 'proof-pack-2026-10.pdf')
  })

  it('asks for the kept copy and for the export bundle of the whole month', async () => {
    setup()
    fireEvent.click(screen.getByRole('button', { name: /kept copy|salinan tersimpan/i }))
    await waitFor(() => expect(downloadFile).toHaveBeenCalledTimes(1))
    expect(downloadFile.mock.calls[0][1]).toMatchObject({ month: '2026-10', stored: 'true' })
    fireEvent.click(screen.getByRole('button', { name: /export bundle|berkas ekspor/i }))
    await waitFor(() => expect(downloadFile).toHaveBeenCalledTimes(2))
    expect(downloadFile.mock.calls[1][0]).toBe('/sites/s1/chain/export')
    expect(downloadFile.mock.calls[1][1]).toEqual({ from: '2026-10-01', to: '2026-10-31' })
    // The saved name is the one the PDF's command uses: garda-chain-<first 8 of the site id>-<from>_<to>.zip
    expect(downloadFile.mock.calls[1][2]).toBe('garda-chain-s1-2026-10-01_2026-10-31.zip')
  })

  it('shows a failed download as an error toast', async () => {
    downloadFile.mockRejectedValueOnce(new Error('boom'))
    setup()
    fireEvent.click(screen.getByRole('button', { name: /proof pack \(PDF\)|paket bukti \(PDF\)/i }))
    await waitFor(() => expect(onError).toHaveBeenCalled())
  })
})
