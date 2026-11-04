<script lang="ts">
  import { onMount } from 'svelte'
  import UnlockModal from './components/UnlockModal.svelte'
  import Onboarding from './views/Onboarding.svelte'
  import Placeholder from './views/Placeholder.svelte'
  import Send from './views/Send.svelte'
  import Settings from './views/Settings.svelte'
  import { api } from './lib/api'
  import { refreshAll } from './lib/actions'
  import { setTheme, ui, type View } from './lib/state.svelte'

  const nav: { id: View; label: string }[] = [
    { id: 'send', label: 'Envoyer' },
    { id: 'campaigns', label: 'Campagnes' },
    { id: 'history', label: 'Historique' },
    { id: 'settings', label: 'Paramètres' },
  ]

  let fatal = $state('')

  onMount(async () => {
    setTheme(ui.theme)
    try {
      ui.status = await api.status()
      // Coffre à passphrase : déverrouillage via la modale (déclenché par l'appel suivant).
      await refreshAll()
    } catch (e) {
      fatal = e instanceof Error ? e.message : String(e)
    } finally {
      ui.ready = true
    }
  })

  function onkeydown(e: KeyboardEvent) {
    if (!e.ctrlKey || e.shiftKey || e.altKey) return
    const i = Number(e.key) - 1
    if (i >= 0 && i < nav.length && ui.accounts.length) {
      e.preventDefault()
      ui.view = nav[i].id
    }
  }

  const active = $derived(ui.accounts.find((a) => a.active))
</script>

<svelte:window {onkeydown} />

{#if !ui.ready}
  <div class="center muted">Chargement…</div>
{:else if fatal}
  <div class="center"><div class="alert error">{fatal}</div></div>
{:else if ui.accounts.length === 0 && !ui.unlock}
  <Onboarding />
{:else}
  <div class="shell">
    <nav aria-label="Navigation principale">
      <div class="brand">Smsh</div>
      {#each nav as n, i}
        <button class="item" class:current={ui.view === n.id} onclick={() => (ui.view = n.id)} aria-current={ui.view === n.id ? 'page' : undefined}>
          <span>{n.label}</span><kbd>Ctrl+{i + 1}</kbd>
        </button>
      {/each}
      <div class="grow"></div>
      {#if active}
        <div class="acct">
          <div class="small muted">Compte actif</div>
          <strong>{active.name}</strong>
          {#if ui.overview}
            <div class="small">{ui.overview.creditsLeft} crédit(s)</div>
          {:else if ui.overviewError}
            <div class="small err" title={ui.overviewError}>Connexion impossible</div>
          {/if}
        </div>
      {/if}
    </nav>
    <main>
      {#if ui.overviewError}
        <div class="alert error" role="alert"><strong>Compte « {active?.name} » inaccessible</strong>{ui.overviewError}</div>
      {/if}
      {#if ui.view === 'send'}
        <Send />
      {:else if ui.view === 'campaigns'}
        <Placeholder title="Campagnes" text="Import CSV, templates, dry-run et envoi avec progression : à venir." />
      {:else if ui.view === 'history'}
        <Placeholder title="Historique" text="Liste, recherche et suppression des SMS envoyés : à venir." />
      {:else}
        <Settings />
      {/if}
    </main>
  </div>
{/if}

{#if ui.unlock}
  <UnlockModal initialized={ui.unlock.initialized} />
{/if}

<style>
  .center { height: 100%; display: flex; align-items: center; justify-content: center; }
  .shell { display: flex; height: 100%; }
  nav {
    width: 210px; flex-shrink: 0; padding: 16px 12px;
    background: var(--surface); border-right: 1px solid var(--border);
    display: flex; flex-direction: column; gap: 4px;
  }
  .brand { font-weight: 700; font-size: 18px; padding: 4px 10px 14px; letter-spacing: 0.5px; }
  .item {
    display: flex; justify-content: space-between; align-items: center;
    width: 100%; text-align: left; background: none; border: 1px solid transparent; padding: 8px 10px;
  }
  .item:hover:not(:disabled) { background: var(--surface-2); border-color: transparent; }
  .item.current { background: var(--surface-2); border-color: var(--border); font-weight: 600; }
  .acct { border-top: 1px solid var(--border); padding: 12px 10px 0; }
  .err { color: var(--danger); }
  main { flex: 1; overflow-y: auto; padding: 28px 32px; }
  main > :global(*) { max-width: 760px; }
</style>
