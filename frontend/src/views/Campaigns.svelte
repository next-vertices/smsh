<script lang="ts">
  import CampaignDetail from './CampaignDetail.svelte'
  import CampaignWizard from './CampaignWizard.svelte'
  import { api, ApiError, type Campaign } from '../lib/api'
  import { refreshOverview } from '../lib/actions'
  import { dateFr, modeLabel, statusLabel } from '../lib/format'

  type Screen = { name: 'list' } | { name: 'new' } | { name: 'detail'; id: string }
  let screen = $state<Screen>({ name: 'list' })
  let list = $state<Campaign[]>([])
  let loaded = $state(false)
  let error = $state('')

  async function load() {
    try {
      list = await api.campaigns()
      error = ''
    } catch (e) {
      error = e instanceof ApiError ? e.message : String(e)
    } finally {
      loaded = true
    }
  }

  // Liste : rafraîchie toutes les 2 s tant qu'une campagne est active.
  $effect(() => {
    if (screen.name !== 'list') return
    load()
    const t = setInterval(() => {
      if (list.some((c) => ['pending', 'running', 'paused'].includes(c.status))) load()
    }, 2000)
    return () => clearInterval(t)
  })

  function pct(c: Campaign) { return c.total ? Math.min(100, Math.round(((c.sent + c.failed) / c.total) * 100)) : 0 }

  function launched(c: Campaign) {
    refreshOverview()
    screen = { name: 'detail', id: c.id }
  }
</script>

{#if screen.name === 'new'}
  <CampaignWizard oncancel={() => (screen = { name: 'list' })} onlaunched={launched} />
{:else if screen.name === 'detail'}
  <CampaignDetail id={screen.id} onback={() => { refreshOverview(); screen = { name: 'list' } }} />
{:else}
  <div class="row between" style="margin-bottom: 4px">
    <h1>Campagnes</h1>
    <button class="primary" onclick={() => (screen = { name: 'new' })}>Nouvelle campagne</button>
  </div>
  <p class="muted">Envois en masse depuis un fichier CSV, avec simulation préalable.</p>

  {#if error}<div class="alert error" role="alert">{error}</div>{/if}

  {#if loaded && list.length === 0 && !error}
    <div class="card muted">Aucune campagne pour l'instant. Cliquez sur « Nouvelle campagne » pour importer un CSV.</div>
  {:else}
    <div class="card" style="padding: 0; overflow: hidden">
      {#each list as c (c.id)}
        <button class="item" onclick={() => (screen = { name: 'detail', id: c.id })}>
          <div class="grow">
            <div><strong>{c.name}</strong> <span class="badge">{modeLabel[c.mode]}</span></div>
            <div class="small muted">{dateFr(c.createdAt)} · {c.total} destinataire(s) · {c.credits} crédit(s)</div>
          </div>
          <div class="prog">
            <div class="bar"><div class="fill" class:bad={c.status === 'failed'} style="width: {pct(c)}%"></div></div>
            <div class="small muted">{c.sent + c.failed}/{c.total}{#if c.failed} · <span style="color: var(--danger)">{c.failed} échec(s)</span>{/if}</div>
          </div>
          <span class="badge" class:accent={c.status === 'running'}>{statusLabel[c.status] ?? c.status}</span>
        </button>
      {/each}
    </div>
  {/if}
{/if}

<style>
  .item {
    display: flex; align-items: center; gap: 16px; width: 100%; text-align: left;
    background: none; border: none; border-bottom: 1px solid var(--border); border-radius: 0; padding: 12px 16px;
  }
  .item:last-child { border-bottom: none; }
  .item:hover:not(:disabled) { background: var(--surface-2); }
  .prog { width: 160px; flex-shrink: 0; }
  .bar { height: 6px; background: var(--surface-2); border: 1px solid var(--border); border-radius: 999px; overflow: hidden; margin-bottom: 2px; }
  .fill { height: 100%; background: var(--accent); }
  .fill.bad { background: var(--danger); }
</style>
