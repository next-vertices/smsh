<script lang="ts">
  import CampaignDetail from './CampaignDetail.svelte'
  import CampaignWizard from './CampaignWizard.svelte'
  import CsvTemplate from '../components/CsvTemplate.svelte'
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

  const stats = $derived({
    count: list.length,
    active: list.filter((c) => ['pending', 'running', 'paused'].includes(c.status)).length,
    sent: list.reduce((n, c) => n + c.sent, 0),
    failed: list.reduce((n, c) => n + c.failed, 0),
    credits: list.reduce((n, c) => n + c.credits, 0),
  })

  function pct(c: Campaign) { return c.total ? Math.min(100, Math.round(((c.sent + c.failed) / c.total) * 100)) : 0 }

  function launched(c: Campaign) {
    refreshOverview()
    screen = { name: 'detail', id: c.id }
  }
</script>

{#if screen.name === 'new'}
  <div class="narrow"><CampaignWizard oncancel={() => (screen = { name: 'list' })} onlaunched={launched} /></div>
{:else if screen.name === 'detail'}
  <CampaignDetail id={screen.id} onback={() => { refreshOverview(); screen = { name: 'list' } }} />
{:else}
  <div class="row between" style="margin-bottom: 4px">
    <h1>Campagnes</h1>
    <button class="primary" onclick={() => (screen = { name: 'new' })}>Nouvelle campagne</button>
  </div>
  <p class="muted">Envois en masse depuis un fichier CSV, avec simulation préalable.</p>

  {#if error}<div class="alert error" role="alert">{error}</div>{/if}

  <div class="cpage">
  <div class="cmain">
  {#if list.length > 0}
    <div class="stats">
      <div class="stat"><div class="n">{stats.count}</div><div class="small muted">campagne(s)</div></div>
      <div class="stat"><div class="n">{stats.active}</div><div class="small muted">en cours</div></div>
      <div class="stat"><div class="n">{stats.sent}</div><div class="small muted">SMS envoyés</div></div>
      <div class="stat"><div class="n" style:color={stats.failed ? 'var(--danger)' : undefined}>{stats.failed}</div><div class="small muted">échecs</div></div>
      <div class="stat"><div class="n">{stats.credits}</div><div class="small muted">crédits consommés</div></div>
    </div>
  {/if}
  {#if loaded && list.length === 0 && !error}
    <div class="card muted">Aucune campagne pour l'instant. Cliquez sur « Nouvelle campagne » pour importer un CSV, ou téléchargez d'abord le modèle.</div>
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
  </div>

  <aside class="chelp">
    <div class="card">
      <h2>Fichier CSV</h2>
      <p class="muted small">Première ligne = en-têtes. Une colonne contient le numéro (06…, +33…). Séparateur <code>;</code> <code>,</code> ou tabulation, UTF-8 ou Excel.</p>
      <pre class="ex">telephone;prenom;ville
0612345678;Jean;Lyon
+33698765432;Léa;Paris</pre>
      <div class="row wrap" style="gap: 8px"><CsvTemplate /></div>
    </div>
    <div class="card">
      <h2>Variables</h2>
      <p class="muted small" style="margin: 0">Dans le message, <code>{'{prenom}'}</code> est remplacé par la valeur de la colonne <code>prenom</code> de chaque ligne. Le nom doit correspondre à l'en-tête du CSV.</p>
    </div>
    <div class="card">
      <h2>Deux modes d'envoi</h2>
      <p class="small" style="margin: 0 0 8px"><strong>Lot (batch)</strong> <span class="muted">: même message pour tous, traité côté OVH, rapide. Pas de pause possible.</span></p>
      <p class="small" style="margin: 0"><strong>Personnalisé</strong> <span class="muted">: un SMS par destinataire (variables), débit réglable, pause / reprise / annulation. Imposé dès qu'il y a des variables.</span></p>
    </div>
    <div class="card">
      <h2>Avant d'envoyer</h2>
      <p class="muted small" style="margin: 0">Une simulation (sans envoi) affiche automatiquement les numéros invalides, les doublons, le nombre de SMS, le coût estimé et la durée.</p>
    </div>
  </aside>
  </div>
{/if}

<style>
  .cpage { display: block; }
  @media (min-width: 1280px) {
    .cpage { display: grid; grid-template-columns: minmax(0, 1fr) 380px; gap: 0 20px; align-items: start; }
    .chelp { position: sticky; top: 0; }
  }
  .stats { display: grid; grid-template-columns: repeat(auto-fit, minmax(110px, 1fr)); gap: 12px; margin-bottom: 16px; }
  .stat { background: var(--surface); border: 1px solid var(--border); border-radius: 14px; padding: 12px 16px; box-shadow: var(--card-shadow); }
  .stat .n { font-size: 22px; font-weight: 700; letter-spacing: -0.02em; }
  .ex { margin: 0 0 12px; padding: 10px 12px; background: var(--surface-2); border: 1px solid var(--border); border-radius: 10px; font: 12px Consolas, monospace; overflow-x: auto; }
  code { font: 12px Consolas, monospace; background: var(--surface-2); padding: 0 4px; border-radius: 4px; }
  .narrow { max-width: 920px; margin-inline: auto; }
  @media (min-width: 1280px) { .narrow { max-width: 1500px; } }
  @media (min-width: 2400px) { .narrow { max-width: none; margin-inline: 0; } }
  .item {
    display: flex; align-items: center; gap: 16px; width: 100%; text-align: left;
    background: none; border: none; border-bottom: 1px solid var(--border); border-radius: 0; padding: 12px 16px;
  }
  .item:last-child { border-bottom: none; }
  .item:hover:not(:disabled) { background: var(--surface-2); }
  .prog { width: 160px; flex-shrink: 0; }
  .item > .badge { min-width: 84px; text-align: center; flex-shrink: 0; }
  @media (max-width: 640px) {
    .item { flex-wrap: wrap; gap: 8px 12px; }
    .prog { order: 3; width: 100%; }
  }
  .bar { height: 6px; background: var(--surface-2); border: 1px solid var(--border); border-radius: 999px; overflow: hidden; margin-bottom: 2px; }
  .fill { height: 100%; background: var(--accent); }
  .fill.bad { background: var(--danger); }
</style>
