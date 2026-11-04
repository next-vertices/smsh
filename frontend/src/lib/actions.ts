import { api, ApiError } from './api'
import { ui } from './state.svelte'

/** Recharge la liste des comptes et l'état du coffre. */
export async function refreshAccounts() {
  ui.status = await api.status()
  ui.accounts = await api.accounts()
}

/** Recharge crédits/expéditeurs du compte actif (erreur conservée pour l'affichage). */
export async function refreshOverview() {
  ui.overviewError = ''
  if (!ui.accounts.some((a) => a.active)) {
    ui.overview = null
    return
  }
  try {
    ui.overview = await api.overview()
  } catch (e) {
    ui.overview = null
    ui.overviewError = e instanceof ApiError ? e.message : String(e)
  }
}

export async function refreshAll() {
  await refreshAccounts()
  await refreshOverview()
}
