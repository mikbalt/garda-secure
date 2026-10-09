import { createContext, useContext } from 'react'
import { en, type Key } from './en'
import { id } from './id'
import type { Lang } from '../lib/time'

export type { Key }
export type Vars = Record<string, string | number>

export const dictionaries = { en, id } as const

export interface I18nValue {
  lang: Lang
  setLang: (lang: Lang) => void
  t: (key: Key, vars?: Vars) => string
}

export function translate(lang: Lang, key: Key, vars?: Vars): string {
  let s: string = dictionaries[lang][key] ?? en[key] ?? key
  if (vars) {
    for (const [k, v] of Object.entries(vars)) s = s.split(`{${k}}`).join(String(v))
  }
  return s
}

/** Translates a dynamic key (e.g. `status.${value}`), falling back to the raw value. */
export function translateMaybe(lang: Lang, key: string, fallback: string): string {
  return key in en ? translate(lang, key as Key) : fallback
}

export const I18nContext = createContext<I18nValue>({
  lang: 'id',
  setLang: () => undefined,
  t: (key, vars) => translate('id', key, vars),
})

export function useI18n(): I18nValue {
  return useContext(I18nContext)
}

/** Returns the translate function `t(key, vars)`. */
export function useT() {
  return useContext(I18nContext).t
}

/** Incident category label: translated for known categories, else humanised. */
export function incidentTypeLabel(lang: Lang, type: string): string {
  const human = type.replace(/[_-]+/g, ' ').replace(/^\w/, (c) => c.toUpperCase())
  return translateMaybe(lang, `incType.${type}`, human)
}
