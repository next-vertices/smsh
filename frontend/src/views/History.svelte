<script lang="ts">
  import { untrack } from 'svelte'
  import Confirm from '../components/Confirm.svelte'
  import Modal from '../components/Modal.svelte'
  import {
    api, ApiError, type DeleteResult, type HistoryFacets, type HistoryItem, type HistoryQuery, type SyncResult,
  } from '../lib/api'
  import { refreshOverview } from '../lib/actions'
  import { dateFr } from '../lib/format'
  import { ui } from '../lib/state.svelte'

  const PAGE_SIZES = [25, 50, 100, 200]
  const AUTO_SYNC_MS = 5 * 60 * 1000

  let q = $state<HistoryQuery>({ query: '', receiver: '', status: '', sender: '', tag: '', dateFrom: '', dateTo: '' })
  let items = $state<HistoryItem[]>([])
  let page = $state(0) // page courante (0 = première)
  let pageSize = $state(50)
  let total = $state(0)
  let credits = $state(0)
  let facets = $state<HistoryFacets>({ senders: [], tags: [] })
  let campaignNames = $state<Record<string, string>>({})
  let loading = $state(false)
  let error = $state('')

  let syncing = $state(false)
  let syncResult = $state<SyncResult | null>(null)
  let days = $state(7)
  let syncedDays = $state(7)
  let progress = $state<{ done: number; total: number } | null>(null)
  let stopRequested = false

  let sel = $state<Record<number, boolean>>({})
  let selectedAll = $state(false) // « tout sélectionner » : sélection étendue à tous les résultats
  let confirmIds = $state<number[] | null>(null)
  let deleting = $state(false)
  let deleteResult = $state<DeleteResult | null>(null)

  let detail = $state<HistoryItem | null>(null)
  let searchEl: HTMLInputElement

  // Grand écran : le détail s'affiche dans un panneau à droite (liste toujours visible).
  let docked = $state(false)
  $effect(() => {
    const mq = window.matchMedia('(min-width: 2200px)')
    docked = mq.matches
    const on = (e: MediaQueryListEvent) => (docked = e.matches)
    mq.addEventListener('change', on)
    return () => mq.removeEventListener('change', on)
  })

  const selCount = $derived(Object.keys(sel).length)
  const pageAllSelected = $derived(items.length > 0 && items.every((i) => sel[i.id]))
  const statusLabel: Record<string, string> = { delivered: 'Délivré', sent: 'Envoyé', scheduled: 'Programmé' }
  const tagLabel = (t: string) => campaignNames[t] ?? t

  function fail(e: unknown) { error = e instanceof ApiError ? e.message : String(e) }

  const pageCount = $derived(Math.max(1, Math.ceil(total / pageSize)))
  const rangeFrom = $derived(total === 0 ? 0 : page * pageSize + 1)
  const rangeTo = $derived(Math.min(total, page * pageSize + items.length))

  // Charge la page courante ; si elle n'existe plus (suppressions, filtres), recule sur la dernière.
  async function load() {
    loading = true
    try {
      let p = await api.searchHistory($state.snapshot(q), page * pageSize, pageSize)
      const last = Math.max(0, Math.ceil(p.total / pageSize) - 1)
      if (page > last) {
        page = last
        p = await api.searchHistory($state.snapshot(q), page * pageSize, pageSize)
      }
      items = p.items
      total = p.total
      credits = p.credits
      error = ''
    } catch (e) { fail(e) } finally { loading = false }
  }

  function goto(n: number) {
    const target = Math.min(Math.max(0, Math.trunc(n) || 0), pageCount - 1)
    if (target === page) return
    page = target
    load()
  }

  function changeSize() {
    page = 0
    load()
  }

  async function loadFacets() {
    try {
      facets = await api.historyFacets()
      const cs = await api.campaigns()
      campaignNames = Object.fromEntries(cs.map((c) => [c.id, c.name]))
    } catch { /* facettes facultatives */ }
  }

  // Télécharge par lots (les plus récents d'abord) jusqu'à épuisement, erreur ou arrêt demandé.
  async function sync() {
    if (syncing) return
    syncing = true
    stopRequested = false
    error = ''
    syncResult = null
    progress = null
    syncedDays = days
    let fetched = 0
    let total = -1
    try {
      for (;;) {
        const r = await api.syncHistory(days)
        fetched += r.fetched
        if (total < 0) total = r.fetched + r.remaining
        progress = { done: Math.min(fetched, total), total }
        syncResult = { ...r, fetched }
        await Promise.all([load(), loadFacets()])
        if (r.errors.length || r.remaining === 0 || r.fetched === 0 || stopRequested) break
      }
      ui.historySyncedAt = Date.now()
    } catch (e) { fail(e) } finally { syncing = false; progress = null }
  }

  // Recherche instantanée (cache local) 300 ms après la dernière saisie.
  $effect(() => {
    void [q.query, q.receiver, q.status, q.sender, q.tag, q.dateFrom, q.dateTo]
    const t = setTimeout(() => {
      sel = {}
      selectedAll = false
      page = 0
      load()
    }, 300)
    return () => clearTimeout(t)
  })

  $effect(() => {
    untrack(() => {
      loadFacets()
      if (Date.now() - ui.historySyncedAt > AUTO_SYNC_MS) sync()
    })
  })

  function toggle(id: number) {
    if (sel[id]) delete sel[id]
    else sel[id] = true
    selectedAll = false
  }

  function togglePage() {
    if (pageAllSelected) for (const i of items) delete sel[i.id]
    else for (const i of items) sel[i.id] = true
    selectedAll = false
  }

  async function selectAllResults() {
    try {
      const ids = await api.historyIds($state.snapshot(q))
      sel = Object.fromEntries(ids.map((id) => [id, true]))
      selectedAll = true
    } catch (e) { fail(e) }
  }

  function clearSel() { sel = {}; selectedAll = false }

  async function doDelete() {
    if (!confirmIds) return
    deleting = true
    try {
      deleteResult = await api.deleteHistory(confirmIds)
      confirmIds = null
      detail = null
      sel = {}
      selectedAll = false
      await Promise.all([load(), loadFacets()])
      refreshOverview()
    } catch (e) { confirmIds = null; fail(e) } finally { deleting = false }
  }

  async function open(h: HistoryItem) {
    detail = h
    try { detail = await api.historyDetail(h.id) } catch { /* on garde la version en cache */ }
  }

  function reset() {
    q = { query: '', receiver: '', status: '', sender: '', tag: '', dateFrom: '', dateTo: '' }
  }

  const filtered = $derived(!!(q.query || q.receiver || q.status || q.sender || q.tag || q.dateFrom || q.dateTo))

  function onkeydown(e: KeyboardEvent) {
    const el = e.target as HTMLElement
    if (e.key === '/' && !['INPUT', 'TEXTAREA', 'SELECT'].includes(el.tagName) && !detail && !confirmIds) {
      e.preventDefault()
      searchEl?.focus()
    }
  }
</script>

<svelte:window {onkeydown} />

<div class="row between" style="margin-bottom: 4px">
  <h1>Historique</h1>
  <div class="row">
    <select bind:value={days} disabled={syncing} aria-label="Période à synchroniser" style="width: auto">
      <option value={7}>7 derniers jours</option>
      <option value={30}>30 derniers jours</option>
      <option value={90}>90 derniers jours</option>
    </select>
    {#if syncing}
      <button onclick={() => (stopRequested = true)}>Arrêter</button>
    {:else}
      <button onclick={sync}>Synchroniser</button>
    {/if}
  </div>
</div>
<p class="muted">SMS envoyés depuis ce compte, mis en cache localement. <kbd>/</kbd> pour rechercher.</p>

{#if error}<div class="alert error" role="alert">{error}</div>{/if}

<div class="hpage">
<aside class="hside">
{#if progress && progress.total > 0}
  <div class="card" style="padding: 12px 16px">
    <div class="row between small"><span>Téléchargement des SMS (les plus récents d'abord)…</span>
      <span><strong>{progress.done}</strong> / {progress.total}</span></div>
    <div class="pbar"><div class="pfill" style="width: {Math.round((progress.done / progress.total) * 100)}%"></div></div>
  </div>
{/if}

{#if syncResult && !syncing}
  <div class="alert" class:ok={!syncResult.errors.length} class:warn={syncResult.errors.length > 0} role="status">
    <strong>Synchronisation : {syncResult.listed} SMS sur {syncedDays} jours, {syncResult.fetched} téléchargé(s){#if syncResult.removed}, {syncResult.removed} supprimé(s) chez OVH{/if}</strong>
    {#if syncResult.remaining > 0}
      Il reste {syncResult.remaining} SMS à télécharger.
      <button class="link" onclick={sync} disabled={syncing}>Continuer</button>
    {/if}
    {#each syncResult.errors as e}<div class="small">{e}</div>{/each}
  </div>
{/if}

{#if deleteResult}
  <div class="alert" class:ok={!deleteResult.failed.length} class:error={deleteResult.failed.length > 0} role="status">
    <strong>{deleteResult.deleted} SMS supprimé(s){#if deleteResult.failed.length}, {deleteResult.failed.length} échec(s){/if}</strong>
    {#each deleteResult.failed.slice(0, 3) as f}<div class="small">#{f.id} : {f.error}</div>{/each}
  </div>
{/if}

<div class="card filters">
  <div class="grid-auto">
    <div>
      <label for="hq">Recherche</label>
      <input id="hq" type="text" bind:this={searchEl} bind:value={q.query} placeholder="Message, numéro, expéditeur…" />
    </div>
    <div>
      <label for="hr">Destinataire</label>
      <input id="hr" type="text" bind:value={q.receiver} placeholder="06 12 34 56 78" />
    </div>
    <div>
      <label for="hs">Statut</label>
      <select id="hs" bind:value={q.status}>
        <option value="">Tous</option>
        <option value="delivered">Délivré</option>
        <option value="sent">Envoyé (sans accusé)</option>
        <option value="scheduled">Programmé</option>
      </select>
    </div>
    <div>
      <label for="hd1">Du</label>
      <input id="hd1" type="date" bind:value={q.dateFrom} />
    </div>
    <div>
      <label for="hd2">Au</label>
      <input id="hd2" type="date" bind:value={q.dateTo} />
    </div>
    <div>
      <label for="hx">Expéditeur</label>
      <select id="hx" bind:value={q.sender}>
        <option value="">Tous</option>
        {#each facets.senders as s}<option value={s}>{s}</option>{/each}
      </select>
    </div>
  </div>
  {#if facets.tags.length}
    <div class="row" style="margin-top: 10px">
      <label for="ht" style="margin: 0">Campagne</label>
      <select id="ht" bind:value={q.tag} style="max-width: 260px">
        <option value="">Toutes</option>
        {#each facets.tags as t}<option value={t}>{tagLabel(t)}</option>{/each}
      </select>
      {#if filtered}<button class="link" onclick={reset}>Réinitialiser les filtres</button>{/if}
    </div>
  {:else if filtered}
    <div style="margin-top: 10px"><button class="link" onclick={reset}>Réinitialiser les filtres</button></div>
  {/if}
</div>
</aside>
<div class="hmain">
<div class="row between" style="margin-bottom: 8px">
  <span class="muted small">
    {total} SMS · {credits} crédit(s){#if loading} · chargement…{/if}
  </span>
  {#if selCount > 0}
    <span class="row">
      <span class="small"><strong>{selCount}</strong> sélectionné(s)</span>
      <button class="link" onclick={clearSel}>Désélectionner</button>
      <button class="danger" onclick={() => (confirmIds = Object.keys(sel).map(Number))}>Supprimer la sélection…</button>
    </span>
  {/if}
</div>

{#if pageAllSelected && !selectedAll && total > items.length}
  <div class="alert warn small" style="margin-top: 0">
    Les {items.length} SMS affichés sont sélectionnés.
    <button class="link" onclick={selectAllResults}>Sélectionner les {total} résultats</button>
  </div>
{/if}

<div class="hlayout" class:open={docked && detail}>
<div class="hlist">
{@render pager()}
<div class="card" style="padding: 0; overflow: hidden">
  <div class="tablewrap">
    <table class="hist">
      <thead>
        <tr>
          <th class="chk"><input type="checkbox" checked={pageAllSelected} onchange={togglePage} aria-label="Tout sélectionner" /></th>
          <th>Date</th><th>Destinataire</th><th>Message</th><th class="c-l">Campagne</th><th>Exp.</th><th class="c-xl">Délivré le</th><th class="r">Seg.</th><th class="r">Cr.</th><th>Statut</th>
        </tr>
      </thead>
      <tbody>
        {#each items as h (h.id)}
          <tr class:picked={sel[h.id]} class:current={detail?.id === h.id} onclick={() => open(h)}>
            <td class="chk" onclick={(e) => e.stopPropagation()}>
              <input type="checkbox" checked={!!sel[h.id]} onchange={() => toggle(h.id)} aria-label="Sélectionner le SMS {h.id}" />
            </td>
            <td class="nowrap">{dateFr(h.createdAt)}</td>
            <td class="mono nowrap">{h.receiver}</td>
            <td class="msg" title={h.message}>{h.message}</td>
            <td class="c-l nowrap">{h.tag ? tagLabel(h.tag) : ''}</td>
            <td class="nowrap">{h.sender}</td>
            <td class="c-xl nowrap">{h.deliveredAt ? dateFr(h.deliveredAt) : ''}</td>
            <td class="r">{h.parts}</td>
            <td class="r">{h.credits}</td>
            <td><span class="st {h.status}">{statusLabel[h.status] ?? h.status}</span></td>
          </tr>
        {:else}
          <tr><td colspan="10" class="empty muted">
            {#if loading}Chargement…{:else if filtered}Aucun SMS ne correspond à ces filtres.{:else}Aucun SMS en cache. Cliquez sur « Synchroniser ».{/if}
          </td></tr>
        {/each}
      </tbody>
    </table>
  </div>
</div>
{@render pager()}

</div>
{#if docked}{@render dockPanel()}{/if}
</div>
</div>
</div>

{#snippet pager()}
  <nav class="pager" aria-label="Pagination">
    <span class="small muted">{rangeFrom}–{rangeTo} sur {total}</span>
    <span class="grow"></span>
    <label class="small muted" for="psize" style="margin: 0">Par page</label>
    <select id="psize" bind:value={pageSize} onchange={changeSize} style="width: auto">
      {#each PAGE_SIZES as n}<option value={n}>{n}</option>{/each}
    </select>
    <button onclick={() => goto(0)} disabled={page === 0 || loading} aria-label="Première page">«</button>
    <button onclick={() => goto(page - 1)} disabled={page === 0 || loading} aria-label="Page précédente">‹</button>
    <span class="small">Page
      <input class="pnum" type="number" min="1" max={pageCount} value={page + 1}
        onchange={(e) => goto(Number(e.currentTarget.value) - 1)} aria-label="Numéro de page" />
      / {pageCount}</span>
    <button onclick={() => goto(page + 1)} disabled={page >= pageCount - 1 || loading} aria-label="Page suivante">›</button>
    <button onclick={() => goto(pageCount - 1)} disabled={page >= pageCount - 1 || loading} aria-label="Dernière page">»</button>
  </nav>
{/snippet}

{#snippet dockPanel()}
  {#if detail}
    {@const d = detail}
    <aside class="dock card">
      <div class="row between" style="margin-bottom: 10px">
        <h2 style="margin: 0">Détail du SMS</h2>
        <button onclick={() => (detail = null)} aria-label="Fermer le détail">✕</button>
      </div>
      {@render detailBody(d)}
      <div class="row" style="justify-content: flex-end; margin-top: 12px">
        <button class="danger" onclick={() => (confirmIds = [d.id])}>Supprimer…</button>
      </div>
    </aside>
  {/if}
{/snippet}

{#snippet detailBody(d: HistoryItem)}
    <dl>
      <dt>Destinataire</dt><dd class="mono">{d.receiver}</dd>
      <dt>Statut</dt><dd><span class="st {d.status}">{statusLabel[d.status] ?? d.status}</span></dd>
      <dt>Créé</dt><dd>{dateFr(d.createdAt)}</dd>
      <dt>Envoyé</dt><dd>{d.sentAt ? dateFr(d.sentAt) : (d.differed ? `programmé (+${d.differed} min)` : '—')}</dd>
      <dt>Délivré</dt><dd>{d.deliveredAt ? dateFr(d.deliveredAt) : '—'}</dd>
      <dt>Expéditeur</dt><dd>{d.sender || 'par défaut'}</dd>
      <dt>Segments</dt><dd>{d.parts} · {d.credits} crédit(s)</dd>
      {#if d.tag}<dt>Campagne</dt><dd>{tagLabel(d.tag)}</dd>{/if}
      <dt>ID OVH</dt><dd class="mono">{d.id}{#if d.batchId} · batch {d.batchId}{/if}</dd>
      <dt>Codes OVH</dt><dd class="mono small">accusé {d.receipt} · état {d.ptt}</dd>
    </dl>
    <blockquote>{d.message}</blockquote>
    {/snippet}

{#if detail && !docked}
  {@const d = detail}
  <Modal title="Détail du SMS" onclose={() => (detail = null)}>
    {@render detailBody(d)}
    {#snippet actions()}
      <button class="danger" onclick={() => (confirmIds = [d.id])}>Supprimer…</button>
      <button class="primary" onclick={() => (detail = null)}>Fermer</button>
    {/snippet}
  </Modal>
{/if}

{#if confirmIds}
  <Confirm title="Supprimer des SMS" confirmLabel="Supprimer" danger busy={deleting} onconfirm={doDelete} oncancel={() => (confirmIds = null)}>
    <p>Supprimer <strong>{confirmIds.length} SMS</strong> de l'historique OVH ? Cette action est irréversible
      et ne rappelle pas les messages déjà envoyés.</p>
  </Confirm>
{/if}

<style>
  .pager { display: flex; align-items: center; gap: 8px; flex-wrap: wrap; margin: 8px 0; }
  .pager button { padding: 4px 10px; }
  .pnum { width: 64px !important; padding: 4px 6px !important; text-align: center; }
  .c-l, .c-xl { display: none; }
  .hlayout { display: block; }
  .dock { position: sticky; top: 0; }
  tr.current { background: color-mix(in srgb, var(--accent) 18%, transparent) !important; }
  .hpage { display: block; }
  .hmain { min-width: 0; }
  @media (min-width: 1900px) { .c-l { display: table-cell; } }
  @media (min-width: 2400px) { .c-xl { display: table-cell; } }

  /* >= 1600 px : filtres et état de synchronisation à gauche, tableau à droite */
  @media (min-width: 1600px) {
    .hpage { display: grid; grid-template-columns: 340px minmax(0, 1fr); gap: 0 20px; align-items: start; }
    .hside { position: sticky; top: 0; max-height: calc(100vh - 100px); overflow-y: auto; padding-right: 2px; }
    .hside .grid-auto { grid-template-columns: 1fr 1fr; }
    .hside .grid-auto > :nth-child(1), .hside .grid-auto > :nth-child(2),
    .hside .grid-auto > :nth-child(3), .hside .grid-auto > :nth-child(6) { grid-column: 1 / -1; }
    table.hist { min-width: 0 !important; }
  }
  /* >= 2200 px : détail du SMS dans un troisième panneau */
  @media (min-width: 2200px) {
    .hpage { grid-template-columns: 360px minmax(0, 1fr); }
    .hlayout.open { display: grid; grid-template-columns: minmax(0, 1fr) 440px; gap: 0 16px; align-items: start; }
  }
  .pbar { height: 8px; margin-top: 8px; background: var(--surface-2); border: 1px solid var(--border); border-radius: 999px; overflow: hidden; }
  .pfill { height: 100%; background: var(--accent); transition: width 0.3s ease; }
  .tablewrap { overflow-x: auto; }
  table.hist { border-collapse: collapse; width: 100%; min-width: 760px; font-size: 12.5px; }
  .hist th, .hist td { text-align: left; padding: 7px 10px; border-bottom: 1px solid var(--border); }
  .hist th { background: var(--surface-2); font-weight: 600; white-space: nowrap; }
  .hist tbody tr { cursor: pointer; }
  .hist tbody tr:hover { background: var(--surface-2); }
  .hist tbody tr:last-child td { border-bottom: none; }
  .hist tr.picked { background: color-mix(in srgb, var(--accent) 12%, transparent); }
  .chk { width: 34px; }
  .chk input { accent-color: var(--accent); }
  .r { text-align: right !important; }
  .nowrap { white-space: nowrap; }
  .msg { max-width: 0; width: 100%; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .empty { text-align: center !important; padding: 28px 10px !important; cursor: default; }
  .st { font-size: 11px; padding: 1px 8px; border-radius: 999px; border: 1px solid var(--border); white-space: nowrap; }
  .st.delivered { color: var(--ok); border-color: var(--ok); }
  .st.scheduled { color: var(--warn); border-color: var(--warn); }
  dl { display: grid; grid-template-columns: 110px 1fr; gap: 6px 12px; margin: 0 0 12px; }
  @media (max-width: 480px) { dl { grid-template-columns: 1fr; gap: 0; } dd { margin-bottom: 8px; } }
  dt { color: var(--muted); }
  dd { margin: 0; }
  blockquote {
    margin: 0; padding: 8px 12px; border-left: 3px solid var(--accent);
    background: var(--surface-2); border-radius: 4px; white-space: pre-wrap; max-height: 180px; overflow: auto;
  }
</style>
