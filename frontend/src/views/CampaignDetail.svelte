<script lang="ts">
  import { untrack } from 'svelte'
  import Confirm from '../components/Confirm.svelte'
  import { api, ApiError, type Campaign, type CampaignRecipient } from '../lib/api'
  import { dateFr, modeLabel, statusLabel } from '../lib/format'
  import { t } from '../lib/i18n.svelte'

  let { id, onback }: { id: string; onback: () => void } = $props()

  const POLL_MS = 1500
  const PAGE = 100

  let c = $state<Campaign | null>(null)
  let items = $state<CampaignRecipient[]>([])
  let total = $state(0)
  let filter = $state('')
  let error = $state('')
  let busy = $state(false)
  let confirm = $state<'cancel' | 'delete' | null>(null)
  let loadedFor = ''

  const active = $derived(!!c && ['pending', 'running', 'paused'].includes(c.status))
  const isLocal = $derived(c?.mode === 'local')
  const done = $derived(c ? c.sent + c.failed : 0)
  const pct = $derived(c && c.total ? Math.min(100, Math.round((done / c.total) * 100)) : 0)
  const pending = $derived(c ? Math.max(c.total - done, 0) : 0)

  function fail(e: unknown) { error = e instanceof ApiError ? e.message : String(e) }

  async function refresh() {
    try {
      c = await api.campaign(id)
      if (c.mode === 'local') {
        // Recharge les lignes déjà affichées (au moins la première page).
        const p = await api.campaignRecipients(id, filter, 0, Math.max(PAGE, items.length))
        items = p.items
        total = p.total
      }
    } catch (e) { fail(e) }
  }

  // Suivi temps réel : rafraîchissement tant que la campagne est active.
  $effect(() => {
    void id; void filter
    untrack(() => {
      items = []
      refresh()
    })
    const timer = setInterval(() => { if (!c || active) refresh() }, POLL_MS)
    return () => clearInterval(timer)
  })

  async function more() {
    try {
      const p = await api.campaignRecipients(id, filter, items.length, PAGE)
      items = [...items, ...p.items]
      total = p.total
    } catch (e) { fail(e) }
  }

  async function act(fn: () => Promise<unknown>) {
    busy = true
    error = ''
    try { await fn(); await refresh() } catch (e) { fail(e) } finally { busy = false }
  }

  async function confirmed() {
    const what = confirm
    if (what === 'cancel') await act(() => api.cancelCampaign(id))
    if (what === 'delete') {
      busy = true
      try { await api.deleteCampaign(id); onback() } catch (e) { fail(e) } finally { busy = false }
    }
    confirm = null
  }

  const filters = [
    { v: '', l: () => t('detail.f_all') }, { v: 'sent', l: () => t('detail.f_sent') },
    { v: 'failed', l: () => t('detail.f_failed') }, { v: 'pending', l: () => t('detail.f_pending') },
  ]
  const recLabel = (s: string) => (['sent', 'failed', 'pending'].includes(s) ? t(`rec.${s}`) : s)
</script>

<div class="row between" style="margin-bottom: 4px">
  <div>
    <button class="link" onclick={onback}>{t('detail.back')}</button>
    <h1 style="margin-top: 6px">{c?.name ?? '…'}</h1>
  </div>
  {#if c}
    <div class="row" style="gap: 6px">
      <span class="badge">{modeLabel[c.mode]}</span>
      <span class="badge accent">{statusLabel[c.status] ?? c.status}</span>
    </div>
  {/if}
</div>

{#if error}<div class="alert error" role="alert">{error}</div>{/if}

{#if c}
  {#if c.error}
    <div class="alert" class:warn={c.status === 'paused'} class:error={c.status !== 'paused'} role="alert">{c.error}</div>
  {/if}

  <div class="dgrid" class:solo={!isLocal}>
  <div class="card side">
    <div class="bar" role="progressbar" aria-valuenow={pct} aria-valuemin="0" aria-valuemax="100">
      <div class="fill" style="width: {pct}%"></div>
    </div>
    <div class="row between" style="margin-top: 8px">
      <span><strong>{done}</strong> {t('detail.processed', { total: c.total, pct })}</span>
      <span class="muted small">{t('detail.created', { date: dateFr(c.createdAt) })}{#if c.sendAt} · {t('detail.scheduled', { date: dateFr(c.sendAt) })}{/if}</span>
    </div>

    <div class="stats">
      <div><div class="n ok">{c.sent}</div><div class="label2">{t('detail.s_sent')}</div></div>
      <div><div class="n bad">{c.failed}</div><div class="label2">{t('detail.s_failed')}</div></div>
      <div><div class="n">{pending}</div><div class="label2">{t('detail.s_remaining')}</div></div>
      <div><div class="n">{c.credits}</div><div class="label2">{t('detail.s_credits')}</div></div>
    </div>

    <div class="row wrap" style="margin-top: 14px">
      {#if isLocal && c.status === 'running'}
        <button disabled={busy} onclick={() => act(() => api.pauseCampaign(id))}>{t('detail.pause')}</button>
      {/if}
      {#if isLocal && (c.status === 'paused' || c.status === 'pending')}
        <button class="primary" disabled={busy} onclick={() => act(() => api.resumeCampaign(id))}>{t('detail.resume')}</button>
      {/if}
      {#if isLocal && c.failed > 0 && c.status !== 'running' && c.status !== 'canceled'}
        <button disabled={busy} onclick={() => act(() => api.retryFailed(id))}>{t('detail.retry', { count: c.failed })}</button>
      {/if}
      {#if active}
        <button class="danger" disabled={busy} onclick={() => (confirm = 'cancel')}>{t('detail.cancel')}</button>
      {:else}
        <button class="danger" disabled={busy} onclick={() => (confirm = 'delete')}>{t('detail.delete')}</button>
      {/if}
    </div>
    {#if !isLocal && active}
      <div class="hint">{t('detail.ovh_hint')}</div>
    {/if}
  </div>

  {#if isLocal}
    <div class="card">
      <div class="row between" style="margin-bottom: 10px">
        <h2 style="margin: 0">{t('detail.recipients')}</h2>
        <div class="row" style="gap: 4px">
          {#each filters as f}
            <button class:primary={filter === f.v} onclick={() => (filter = f.v)}>{f.l()}</button>
          {/each}
        </div>
      </div>
      <div class="tablewrap">
        <table class="data">
          <thead><tr><th>{t('detail.th_number')}</th><th>{t('detail.th_status')}</th><th>{t('detail.th_message')}</th><th>{t('detail.th_detail')}</th></tr></thead>
          <tbody>
            {#each items as r (r.idx)}
              <tr>
                <td class="mono">{r.phone}</td>
                <td><span class="st {r.status}">{recLabel(r.status)}</span></td>
                <td class="msg" title={r.message}>{r.message}</td>
                <td class="small muted">{r.error || (r.ovhId ? t('detail.ovh_id', { id: r.ovhId }) : '')}</td>
              </tr>
            {:else}
              <tr><td colspan="4" class="muted">{t('detail.none')}</td></tr>
            {/each}
          </tbody>
        </table>
      </div>
      {#if items.length < total}
        <div class="row" style="justify-content: center; margin-top: 10px">
          <button onclick={more}>{t('detail.show_more', { count: total - items.length })}</button>
        </div>
      {/if}
    </div>
  {/if}
  </div>
{/if}

{#if confirm && c}
  <Confirm
    title={confirm === 'cancel' ? t('detail.cancel_title') : t('detail.delete_title')}
    confirmLabel={confirm === 'cancel' ? t('detail.cancel') : t('detail.delete')}
    danger {busy} onconfirm={confirmed} oncancel={() => (confirm = null)}>
    {#if confirm === 'cancel'}
      <p>{t('detail.cancel_pre')} <strong>{c.name}</strong>{t('detail.cancel_post', { pending })}</p>
    {:else}
      <p>{t('detail.delete_pre')} <strong>{c.name}</strong> {t('detail.delete_post')}</p>
    {/if}
  </Confirm>
{/if}

<style>
  .dgrid { display: block; }
  @media (min-width: 1400px) {
    .dgrid:not(.solo) { display: grid; grid-template-columns: 420px minmax(0, 1fr); gap: 0 20px; align-items: start; }
    .dgrid .side { position: sticky; top: 0; }
    .tablewrap { max-height: calc(100vh - 250px) !important; }
  }
  @media (min-width: 1400px) { .solo { max-width: 720px; } }
  .bar { height: 10px; background: var(--surface-2); border-radius: 999px; overflow: hidden; border: 1px solid var(--border); }
  .fill { height: 100%; background: var(--accent); transition: width 0.4s ease; }
  .stats { display: grid; grid-template-columns: repeat(4, 1fr); gap: 12px; margin-top: 16px; }
  @media (max-width: 560px) { .stats { grid-template-columns: repeat(2, 1fr); } }
  .n { font-size: 22px; font-weight: 700; line-height: 1.2; }
  .n.ok { color: var(--ok); }
  .n.bad { color: var(--danger); }
  .label2 { font-size: 11px; text-transform: uppercase; letter-spacing: 0.6px; color: var(--muted); }
  .tablewrap { overflow-x: auto; border: 1px solid var(--border); border-radius: 8px; max-height: 60vh; overflow-y: auto; }
  table.data { border-collapse: collapse; width: 100%; min-width: 560px; font-size: 12px; }
  table.data th, table.data td { text-align: left; padding: 5px 10px; border-bottom: 1px solid var(--border); }
  table.data th { background: var(--surface-2); position: sticky; top: 0; }
  .msg { max-width: 0; width: 60%; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .st { font-size: 11px; padding: 1px 8px; border-radius: 999px; border: 1px solid var(--border); }
  .st.sent { color: var(--ok); border-color: var(--ok); }
  .st.failed { color: var(--danger); border-color: var(--danger); }
</style>
