// Fine couche au-dessus des bindings Wails : décode les erreurs structurées
// et gère le déverrouillage du coffre. Aucune logique métier ici.
import * as Go from '../../wailsjs/go/app/App'
import type { creds, service, sms, store } from '../../wailsjs/go/models'
import { requestUnlock } from './state.svelte'

export type Account = creds.Account
export type AccountView = creds.View
export type AccountInput = creds.Input
export type VaultStatus = creds.Status
export type ServiceInfo = service.ServiceInfo
export type SendInput = service.SendInput
export type SendOutcome = service.SendOutcome
export type CostEstimate = service.CostEstimate
export type Campaign = store.Campaign
export type CampaignRecipient = store.Recipient
export type ImportInfo = service.ImportInfo
export type PlanInput = service.PlanInput
export type PlanReport = service.PlanReport
export type LaunchInput = service.LaunchInput
export type RecipientsPage = service.RecipientsPage
export type Analysis = sms.Analysis
export type Receivers = sms.Receivers

export type ErrorKind =
  | 'locked' | 'bad_passphrase' | 'no_account' | 'validation'
  | 'credentials' | 'rate_limit' | 'insufficient_credits' | 'invalid_receiver'
  | 'invalid_request' | 'not_found' | 'network' | 'unknown' | 'internal'

export class ApiError extends Error {
  constructor(public kind: ErrorKind, message: string) {
    super(message)
  }
}

function decode(e: unknown): ApiError {
  const raw = typeof e === 'string' ? e : (e as Error)?.message ?? String(e)
  try {
    const p = JSON.parse(raw)
    if (p && typeof p.kind === 'string') return new ApiError(p.kind, p.message)
  } catch { /* pas du JSON : erreur brute */ }
  return new ApiError('internal', raw)
}

/** Appelle un binding ; si le coffre est verrouillé, demande la passphrase puis réessaie. */
async function call<T>(fn: () => Promise<T>): Promise<T> {
  for (;;) {
    try {
      return await fn()
    } catch (e) {
      const err = decode(e)
      if (err.kind !== 'locked') throw err
      await requestUnlock()
    }
  }
}

export const api = {
  status: () => call(Go.Status),
  unlock: (pass: string) => call(() => Go.Unlock(pass)),
  endpoints: () => call(Go.Endpoints),
  accounts: () => call(Go.ListAccounts),
  saveAccount: (i: AccountInput) => call(() => Go.SaveAccount(i)),
  deleteAccount: (id: string) => call(() => Go.DeleteAccount(id)),
  setActive: (id: string) => call(() => Go.SetActiveAccount(id)),
  testConnection: (i: AccountInput) => call(() => Go.TestConnection(i)),
  overview: () => call(Go.Overview),
  estimateCost: (i: service.CostInput) => call(() => Go.EstimateCost(i)),
  send: (i: SendInput) => call(() => Go.Send(i)),
  maxRecipients: () => call(Go.MaxRecipients),
  pickCsv: () => call(Go.PickCSV),
  simulate: (i: PlanInput) => call(() => Go.SimulateCampaign(i)),
  launch: (i: LaunchInput) => call(() => Go.LaunchCampaign(i)),
  campaigns: () => call(Go.ListCampaigns),
  campaign: (id: string) => call(() => Go.GetCampaign(id)),
  campaignRecipients: (id: string, status: string, offset: number, limit: number) =>
    call(() => Go.CampaignRecipients(id, status, offset, limit)),
  pauseCampaign: (id: string) => call(() => Go.PauseCampaign(id)),
  resumeCampaign: (id: string) => call(() => Go.ResumeCampaign(id)),
  cancelCampaign: (id: string) => call(() => Go.CancelCampaign(id)),
  retryFailed: (id: string) => call(() => Go.RetryFailed(id)),
  deleteCampaign: (id: string) => call(() => Go.DeleteCampaign(id)),
  analyze: (m: string) => call(() => Go.AnalyzeMessage(m)),
  parseRecipients: (t: string) => call(() => Go.ParseRecipients(t)),
}

export function emptyInput(): AccountInput {
  return { id: '', name: '', serviceName: '', endpoint: 'ovh-eu', appKey: '', appSecret: '', consumerKey: '' }
}
