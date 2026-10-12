import { config } from '../config'
import { safeMediaUrl } from './url'

/** A photo URL from the API, or undefined when its scheme is not allowed (WEB-9). */
export function mediaUrl(raw: string | null | undefined): string | undefined {
  return safeMediaUrl(raw, { allowDataImages: config.demo, allowHttp: import.meta.env.DEV })
}

/** Whether the URL can be opened in a new tab (browsers block top-level data: URLs). */
export function canOpenInTab(url: string): boolean {
  return !url.startsWith('data:')
}
