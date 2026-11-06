// Formatage d'affichage uniquement (aucune règle métier), selon la langue courante.
import { dateTime, money, t } from './i18n.svelte'

export const eur = money

export function priceRange(min: number, max: number, cur: string): string {
  if (!(max > 0)) return '—'
  return min === max ? eur(max, cur) : t('price.range', { min: eur(min, cur), max: eur(max, cur) })
}

export function duration(seconds: number): string {
  if (seconds < 60) return `${Math.ceil(seconds)} s`
  const m = Math.round(seconds / 60)
  return m < 60 ? `${m} min` : `${Math.floor(m / 60)} h ${String(m % 60).padStart(2, '0')}`
}

/** Objet de libellés traduits : labels('status')['paused'] ; indéfini si la clé n'existe pas. */
function labels(prefix: string): Record<string, string | undefined> {
  return new Proxy({} as Record<string, string | undefined>, {
    get(_, k) {
      if (typeof k !== 'string') return undefined
      const key = `${prefix}.${k}`
      const v = t(key)
      return v === key ? undefined : v
    },
  })
}

export const statusLabel = labels('status')
export const modeLabel = labels('mode')
export const dateFr = dateTime
