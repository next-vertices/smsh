// État global de l'UI (navigation, comptes, thème, modale de passphrase).
import type { AccountView, ServiceInfo, VaultStatus } from './api'

export type View = 'send' | 'campaigns' | 'history' | 'settings'
export type Theme = 'dark' | 'light'

function loadTheme(): Theme {
  try {
    const t = localStorage.getItem('smsh.theme')
    if (t === 'light' || t === 'dark') return t
  } catch { /* stockage indisponible */ }
  return window.matchMedia?.('(prefers-color-scheme: light)').matches ? 'light' : 'dark'
}

export const ui = $state({
  view: 'send' as View,
  ready: false,
  theme: loadTheme(),
  status: null as VaultStatus | null,
  accounts: [] as AccountView[],
  overview: null as ServiceInfo | null,
  overviewError: '',
  historySyncedAt: 0,
  unlock: null as null | { initialized: boolean; resolve: () => void; reject: () => void },
})

export function setTheme(t: Theme) {
  ui.theme = t
  document.documentElement.dataset.theme = t
  try { localStorage.setItem('smsh.theme', t) } catch { /* ignoré */ }
}

/** Ouvre la modale de passphrase ; résout quand le coffre est déverrouillé. */
export function requestUnlock(): Promise<void> {
  if (ui.unlock) {
    // Une modale est déjà ouverte : attendre la même issue.
    const prev = ui.unlock
    return new Promise((resolve, reject) => {
      const { resolve: r, reject: j } = prev
      prev.resolve = () => { r(); resolve() }
      prev.reject = () => { j(); reject(new Error('Coffre verrouillé : opération annulée.')) }
    })
  }
  return new Promise((resolve, reject) => {
    ui.unlock = {
      initialized: ui.status?.initialized ?? true,
      resolve: () => { ui.unlock = null; resolve() },
      reject: () => { ui.unlock = null; reject(new Error('Coffre verrouillé : opération annulée.')) },
    }
  })
}
