import type { Key } from '../../i18n/context'
import { errorText } from '../../i18n/errors'
import { PushError } from '../../lib/push'
import type { Lang } from '../../lib/time'

/** Text of a failed enable/disable: browser-side reasons first, then API error codes. */
export function pushErrorText(err: unknown, lang: Lang, t: (k: Key) => string): string {
  return err instanceof PushError ? t(`push.err.${err.code}` as Key) : errorText(err, lang)
}
