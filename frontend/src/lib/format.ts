// Formatage d'affichage uniquement (aucune règle métier).
export const eur = (v: number, cur = 'EUR') =>
  new Intl.NumberFormat('fr-FR', { style: 'currency', currency: cur || 'EUR', minimumFractionDigits: 2, maximumFractionDigits: 3 }).format(v)

export function priceRange(min: number, max: number, cur: string): string {
  if (!(max > 0)) return '—'
  return min === max ? eur(max, cur) : `${eur(min, cur)} à ${eur(max, cur)}`
}

export function duration(seconds: number): string {
  if (seconds < 60) return `${Math.ceil(seconds)} s`
  const m = Math.round(seconds / 60)
  return m < 60 ? `${m} min` : `${Math.floor(m / 60)} h ${String(m % 60).padStart(2, '0')}`
}

export const statusLabel: Record<string, string> = {
  pending: 'En attente', running: 'En cours', paused: 'En pause',
  completed: 'Terminée', canceled: 'Annulée', failed: 'Échec',
}

export const modeLabel: Record<string, string> = { batch: 'Batch OVH', local: 'Personnalisé' }

export const dateFr = (d: unknown) => (d ? new Date(d as string).toLocaleString('fr-FR') : '')
