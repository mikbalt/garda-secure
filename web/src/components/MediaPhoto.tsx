import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Camera, ImageOff, ExternalLink } from 'lucide-react'
import { apiGet } from '../api/client'
import { useAuth } from '../hooks/useAuth'
import { useT } from '../i18n/context'
import { canOpenInTab, mediaUrl } from '../lib/media'
import { mediaKindOf } from '../lib/roles'
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
  // Only https:/blob:/same-origin URLs are rendered (WEB-9).
  const url = data?.uploaded ? mediaUrl(data.url) : undefined
  if (!url) {
    return (
      <div className="flex aspect-video w-full flex-col items-center justify-center gap-2 rounded-lg border border-dashed border-gray-300 bg-gray-50 text-sm text-gray-700">
        <ImageOff className="h-8 w-8" aria-hidden="true" />
        {data?.uploaded && data.url ? t('plat.photoBlocked') : t('common.pendingUpload')}
      </div>
    )
  }
  return (
    <figure>
      <img src={url} alt={title} className="max-h-[70vh] w-full rounded-lg object-contain" />
      {canOpenInTab(url) && (
        <figcaption className="mt-2 text-right text-sm">
          <a href={url} target="_blank" rel="noreferrer noopener" className="inline-flex items-center gap-1 text-primary-800 underline">
            <ExternalLink className="h-3.5 w-3.5" aria-hidden="true" />
            {t('common.openFull')}
          </a>
        </figcaption>
      )}
    </figure>
  )
}

/**
 * "View photo" button for a stored object key (defect, visitor, selfie). The
 * pre-signed URL is fetched on demand from GET /media/url.
 */
export function MediaPhotoButton({ objectKey, title }: { objectKey?: string | null; title: string }) {
  const t = useT()
  const { canSeeMedia } = useAuth()
  const [open, setOpen] = useState(false)
  if (!objectKey) return <span className="text-gray-600">—</span>
  // The API answers 403 media_forbidden for kinds the role may not see.
  if (!canSeeMedia(mediaKindOf(objectKey))) return null
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
