<script lang="ts">
  import AccountForm from '../components/AccountForm.svelte'
  import Confirm from '../components/Confirm.svelte'
  import { api, ApiError, type AccountView } from '../lib/api'
  import { refreshAll } from '../lib/actions'
  import { setTheme, ui } from '../lib/state.svelte'

  let editing = $state<AccountView | 'new' | null>(null)
  let deleting = $state<AccountView | null>(null)
  let busy = $state(false)
  let error = $state('')

  async function run(fn: () => Promise<unknown>) {
    busy = true
    error = ''
    try {
      await fn()
      await refreshAll()
    } catch (e) {
      error = e instanceof ApiError ? e.message : String(e)
    } finally {
      busy = false
    }
  }

  async function confirmDelete() {
    const a = deleting
    if (!a) return
    await run(() => api.deleteAccount(a.id))
    deleting = null
  }

  async function saved() {
    editing = null
    await refreshAll()
  }

  const backendLabel: Record<string, string> = {
    keyring: 'Coffre du système (Credential Manager / Keychain / Secret Service)',
    age: 'Fichier chiffré par passphrase (age)',
    memory: 'Mémoire (mode démo, rien n\'est conservé)',
  }
</script>

<h1>Paramètres</h1>
<p class="muted">Comptes OVH, stockage des clés et apparence.</p>

<div class="settings">
<div class="col-main">
{#if error}<div class="alert error" role="alert">{error}</div>{/if}

{#if editing}
  <div class="card">
    <h2>{editing === 'new' ? 'Nouveau compte' : `Modifier « ${editing.name} »`}</h2>
    <AccountForm account={editing === 'new' ? undefined : editing} oncancel={() => (editing = null)} onsaved={saved} />
  </div>
{:else}
  <div class="card">
    <div class="row between" style="margin-bottom: 12px">
      <h2 style="margin: 0">Comptes OVH</h2>
      <button class="primary" onclick={() => (editing = 'new')}>Ajouter un compte</button>
    </div>
    {#each ui.accounts as a (a.id)}
      <div class="acc">
        <div class="grow">
          <strong>{a.name}</strong>
          {#if a.active}<span class="badge accent">actif</span>{/if}
          <div class="muted small mono">{a.serviceName} · {a.endpoint} · {a.appKey}</div>
        </div>
        {#if !a.active}
          <button onclick={() => run(() => api.setActive(a.id))} disabled={busy}>Activer</button>
        {/if}
        <button onclick={() => (editing = a)} disabled={busy}>Modifier</button>
        <button class="danger" onclick={() => (deleting = a)} disabled={busy}>Supprimer</button>
      </div>
    {:else}
      <p class="muted">Aucun compte.</p>
    {/each}
  </div>
{/if}

</div>
<div class="col-side">
<div class="card">
  <h2>Apparence</h2>
  <div class="row">
    <button class:primary={ui.theme === 'dark'} onclick={() => setTheme('dark')}>Sombre</button>
    <button class:primary={ui.theme === 'light'} onclick={() => setTheme('light')}>Clair</button>
  </div>
</div>

<div class="card">
  <h2>Stockage des clés</h2>
  <p class="muted" style="margin: 0">
    {backendLabel[ui.status?.backend ?? ''] ?? '—'}<br />
    Les clés ne sont jamais écrites en clair sur le disque, ni envoyées à l'interface.
  </p>
</div>
</div>
</div>

{#if deleting}
  <Confirm title="Supprimer le compte" confirmLabel="Supprimer" danger {busy} onconfirm={confirmDelete} oncancel={() => (deleting = null)}>
    <p>Supprimer le compte <strong>{deleting.name}</strong> et ses clés stockées ? Cette action est irréversible
      (le token reste valide côté OVH tant que vous ne le révoquez pas).</p>
  </Confirm>
{/if}

<style>
  .settings { display: block; }
  @media (min-width: 1280px) {
    .settings { display: grid; grid-template-columns: minmax(0, 1fr) 380px; gap: 0 20px; align-items: start; }
  }
  @media (min-width: 1920px) {
    .settings { grid-template-columns: minmax(0, 960px) 420px; }
  }
  .acc { display: flex; align-items: center; gap: 8px; padding: 10px 0; border-top: 1px solid var(--border); }
  .acc:first-of-type { border-top: none; }
</style>
