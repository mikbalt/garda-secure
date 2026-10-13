import { act, fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { I18nProvider } from '../i18n/I18nProvider'
import { SecretOnce } from './SecretOnce'

describe('SecretOnce', () => {
  it('warns that the secret is shown once and copies it', async () => {
    const writeText = vi.fn().mockResolvedValue(undefined)
    Object.defineProperty(navigator, 'clipboard', { value: { writeText }, configurable: true })
    render(
      <I18nProvider>
        <SecretOnce label="Kata sandi sementara" value="S3cret-Value-42" />
      </I18nProvider>
    )
    expect(screen.getByText('Hanya ditampilkan sekali')).toBeInTheDocument()
    const field = screen.getByLabelText('Kata sandi sementara')
    expect(field).toHaveValue('S3cret-Value-42')
    expect(field).toHaveAttribute('readonly')
    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: /Salin — Kata sandi sementara/ }))
    })
    expect(writeText).toHaveBeenCalledWith('S3cret-Value-42')
    expect(screen.getByRole('status')).toHaveTextContent('Tersalin')
  })
})
