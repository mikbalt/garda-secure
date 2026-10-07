import { useCallback, useEffect, useMemo, useState, type ReactNode } from 'react'
import { I18nContext, translate, type Key, type Vars } from './context'
import { readStored, STORAGE_KEYS, writeStored } from '../lib/storage'
import type { Lang } from '../lib/time'

function initialLang(): Lang {
  const stored = readStored(STORAGE_KEYS.lang)
  return stored === 'en' || stored === 'id' ? stored : 'id'
}

export function I18nProvider({ children }: { children: ReactNode }) {
  const [lang, setLangState] = useState<Lang>(initialLang)

  const setLang = useCallback((l: Lang) => {
    setLangState(l)
    writeStored(STORAGE_KEYS.lang, l)
  }, [])

  useEffect(() => {
    document.documentElement.lang = lang
  }, [lang])

  const value = useMemo(
    () => ({ lang, setLang, t: (key: Key, vars?: Vars) => translate(lang, key, vars) }),
    [lang, setLang]
  )
  return <I18nContext.Provider value={value}>{children}</I18nContext.Provider>
}
