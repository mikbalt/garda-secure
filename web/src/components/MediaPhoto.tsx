import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Camera, ImageOff, ExternalLink } from 'lucide-react'
import { apiGet } from '../api/client'
import { useT } from '../i18n/context'
import { Button } from './ui/Button'
import { Modal } from './ui/Modal'
import { ErrorState } from './ui/States'
import type { Photo } from '../types'

/** Pre-signed URLs are valid for 15 minutes; refetch well before that. */
const URL_STALE_MS = 10 * 60 * 1000

function PhotoViewer({ objectKey, title }: { objectKey: string; title: string }) {
  const t = useT()
  const { data, isLoading, error, refetch } = useQuery({
    queryKey: ['media-url', objectKey],
    queryFn: () => apiGet<Photo>('/media/url', { key: objectKey }),
    staleTime: URL_STALE_MS,
    gcTime: URL_STALE_MS,
  })
  if (isLoading) return <div className="aspect-square w-full animate-pulse rounded-lg bg-gray-100" role="status" aria-label={t('app.loading')} />
  if (error) return <ErrorState error={error} onRetry={() => refetch()} />
  if (!data?.uploaded || !data.url) {
    return (
      <div className="flex aspect-video w-full flex-col items-center justify-center gap-2 rounded-lg border border-dashed border-gray-300 bg-gray-50 text-sm text-gray-700">
        <ImageOff className="h-8 w-8" aria-hidden="true" />
        {t('common.pendingUpload')}
      </div>
    )
  }
  return (
    <figure>
      <img src={data.url} alt={title} className="max-h-[70vh] w-full rounded-lg object-contain" />
      <figcaption className="mt-2 text-right text-sm">
        <a href={data.url} target="_blank" rel="noreferrer" className="inline-flex items-center gap-1 text-primary-800 underline">
          <ExternalLink className="h-3.5 w-3.5" aria-hidden="true" />
          {t('common.openFull')}
        </a>
      </figcaption>
    </figure>
  )
}

/**
 * "View photo" button for a stored object key (defect, visitor, selfie). The
 * pre-signed URL is fetched on demand from GET /media/url.
 */
export function MediaPhotoButton({ objectKey, title }: { objectKey?: string | null; title: string }) {
  const t = useT()
  const [open, setOpen] = useState(false)
  if (!objectKey) return <span className="text-gray-600">—</span>
  return (
    <>
      <Button size="sm" variant="ghost" onClick={() => setOpen(true)} aria-label={`${t('common.viewPhoto')} — ${title}`}>
        <Camera className="h-4 w-4" aria-hidden="true" />
        <span className="sr-only sm:not-sr-only">{t('common.viewPhoto')}</span>
      </Button>
      {open && (
        <Modal open onClose={() => setOpen(false)} title={title} size="lg">
          <PhotoViewer objectKey={objectKey} title={title} />
        </Modal>
      )}
    </>
  )
}
