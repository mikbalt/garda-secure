/**
 * Attention helpers for SOS alerts (WEB-3): a WebAudio beep, a flashing
 * document title and desktop notifications. All of them degrade silently
 * when the browser does not support or allow them.
 */

type AudioCtor = typeof AudioContext

let ctx: AudioContext | null = null

function audioCtor(): AudioCtor | undefined {
  if (typeof window === 'undefined') return undefined
  return window.AudioContext ?? (window as unknown as { webkitAudioContext?: AudioCtor }).webkitAudioContext
}

/**
 * Creates/resumes the AudioContext. Browsers only allow audio after a user
 * gesture, so the app calls this from the first pointer/key event.
 */
export function unlockAudio(): void {
  const Ctor = audioCtor()
  if (!Ctor) return
  try {
    ctx ??= new Ctor()
    if (ctx.state === 'suspended') void ctx.resume().catch(() => undefined)
  } catch {
    ctx = null
  }
}

/** Two short high beeps (≈ 0.5 s). No-op until audio was unlocked. */
export function beep(): void {
  if (!ctx || ctx.state !== 'running') return
  try {
    const t0 = ctx.currentTime
    for (const [start, freq] of [
      [0, 880],
      [0.25, 1175],
    ] as const) {
      const osc = ctx.createOscillator()
      const gain = ctx.createGain()
      osc.type = 'square'
      osc.frequency.value = freq
      gain.gain.setValueAtTime(0.0001, t0 + start)
      gain.gain.exponentialRampToValueAtTime(0.18, t0 + start + 0.02)
      gain.gain.exponentialRampToValueAtTime(0.0001, t0 + start + 0.2)
      osc.connect(gain).connect(ctx.destination)
      osc.start(t0 + start)
      osc.stop(t0 + start + 0.22)
    }
  } catch {
    /* audio unavailable */
  }
}

/** Whether desktop notifications can be requested in this browser. */
export function notificationsSupported(): boolean {
  return typeof window !== 'undefined' && 'Notification' in window
}

export function notificationPermission(): NotificationPermission | 'unsupported' {
  return notificationsSupported() ? Notification.permission : 'unsupported'
}

/** Asks for permission — call from a click handler (user gesture). */
export async function requestNotifications(): Promise<NotificationPermission | 'unsupported'> {
  if (!notificationsSupported()) return 'unsupported'
  try {
    return await Notification.requestPermission()
  } catch {
    return Notification.permission
  }
}

/** Shows a desktop notification when permitted; clicking it focuses the app. */
export function notify(title: string, body: string, tag: string): void {
  if (notificationPermission() !== 'granted') return
  try {
    const n = new Notification(title, { body, tag, requireInteraction: true })
    n.onclick = () => {
      window.focus()
      n.close()
    }
  } catch {
    /* e.g. mobile browsers need a service worker */
  }
}

/** Text of the flashing title while `count` alerts are open. */
export function flashTitle(count: number, base: string): string {
  return `(${count}) SOS! · ${base}`
}
