import type { SiteConfig } from '../types'

export type SiteConfigNumber = Exclude<keyof SiteConfig, 'language' | 'scan_radius_m'>

/**
 * The config_json to save for a site. The API replaces the whole config
 * (anything missing falls back to its default), so keys the form does not
 * edit, such as `scan_radius_m`, are carried over from the stored config.
 */
export function siteConfigForSave(
  stored: Partial<SiteConfig> | null | undefined,
  language: SiteConfig['language'],
  numbers: Record<SiteConfigNumber, string>
): SiteConfig {
  const out = { ...(stored ?? {}), language } as SiteConfig
  for (const key of Object.keys(numbers) as SiteConfigNumber[]) out[key] = Number(numbers[key])
  return out
}