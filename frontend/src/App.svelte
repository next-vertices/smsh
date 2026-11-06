<script lang="ts">
  import { onMount } from 'svelte'
  import UnlockModal from './components/UnlockModal.svelte'
  import Onboarding from './views/Onboarding.svelte'
  import Campaigns from './views/Campaigns.svelte'
  import History from './views/History.svelte'
  import Send from './views/Send.svelte'
  import Settings from './views/Settings.svelte'
  import { api } from './lib/api'
  import { refreshAll } from './lib/actions'
  import { setTheme, ui, type View } from './lib/state.svelte'
  import { initLanguage, t } from './lib/i18n.svelte'

  const nav: { id: View; label: () => string }[] = [
    { id: 'send', label: () => t('nav.send') },
    { id: 'campaigns', label: () => t('nav.campaigns') },
    { id: 'history', label: () => t('nav.history') },
    { id: 'settings', label: () => t('nav.settings') },
  ]

  let fatal = $state('')

  onMount(async () => {
    setTheme(ui.theme)
    await initLanguage(api.systemLanguage, api.setLanguage)
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
    // e.code (position physique) : sur AZERTY, la touche « 1 » non shiftée donne « & ».
    const m = /^(?:Digit|Numpad)([1-9])$/.exec(e.code)
    const i = m ? Number(m[1]) - 1 : Number(e.key) - 1
    if (i >= 0 && i < nav.length && ui.accounts.length) {
      e.preventDefault()
      ui.view = nav[i].id
    }
  }

  const icons: Record<string, string> = {
    send: '<path d="M22 2 11 13"/><path d="M22 2 15 22l-4-9-9-4z"/>',
    campaigns: '<path d="M3 11v2a1 1 0 0 0 1 1h2l5 4V6L6 10H4a1 1 0 0 0-1 1z"/><path d="M15.5 8.5a5 5 0 0 1 0 7"/>',
    history: '<circle cx="12" cy="12" r="9"/><path d="M12 7v5l3 2"/>',
    settings: '<circle cx="12" cy="12" r="3"/><path d="M19.4 15a1.7 1.7 0 0 0 .3 1.8l.1.1a2 2 0 1 1-2.8 2.8l-.1-.1a1.7 1.7 0 0 0-1.8-.3 1.7 1.7 0 0 0-1 1.5V21a2 2 0 1 1-4 0v-.1a1.7 1.7 0 0 0-1.1-1.5 1.7 1.7 0 0 0-1.8.3l-.1.1a2 2 0 1 1-2.8-2.8l.1-.1a1.7 1.7 0 0 0 .3-1.8 1.7 1.7 0 0 0-1.5-1H3a2 2 0 1 1 0-4h.1a1.7 1.7 0 0 0 1.5-1.1 1.7 1.7 0 0 0-.3-1.8l-.1-.1a2 2 0 1 1 2.8-2.8l.1.1a1.7 1.7 0 0 0 1.8.3H9a1.7 1.7 0 0 0 1-1.5V3a2 2 0 1 1 4 0v.1a1.7 1.7 0 0 0 1 1.5 1.7 1.7 0 0 0 1.8-.3l.1-.1a2 2 0 1 1 2.8 2.8l-.1.1a1.7 1.7 0 0 0-.3 1.8V9a1.7 1.7 0 0 0 1.5 1H21a2 2 0 1 1 0 4h-.1a1.7 1.7 0 0 0-1.5 1z"/>',
  }

  const active = $derived(ui.accounts.find((a) => a.active))
</script>

<svelte:window {onkeydown} />

{#if !ui.ready}
  <div class="center muted">{t('app.loading')}</div>
{:else if fatal}
  <div class="center"><div class="alert error">{fatal}</div></div>
{:else if ui.accounts.length === 0 && !ui.unlock}
  <Onboarding />
{:else}
  <div class="shell">
    <nav aria-label={t('nav.aria')}>
      <div class="brand"><span class="logo">S</span>Smsh</div>
      {#each nav as n, i}
        <button class="item" class:current={ui.view === n.id} onclick={() => (ui.view = n.id)} aria-current={ui.view === n.id ? 'page' : undefined}>
          <span class="lab"><svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">{@html icons[n.id] ?? ''}</svg>{n.label()}</span><kbd>Ctrl+{i + 1}</kbd>
        </button>
      {/each}
      <div class="grow"></div>
      {#if active}
        <div class="acct">
          <div class="small muted lbl">{t('app.active_account')}</div>
          <strong><span class="dot" class:bad={!!ui.overviewError}></span>{active.name}</strong>
          {#if ui.overview}
            <div class="small lbl2">{t('app.credits_left', { count: ui.overview.creditsLeft })}</div>
          {:else if ui.overviewError}
            <div class="small err" title={ui.overviewError}>{t('app.connection_failed')}</div>
          {/if}
        </div>
      {/if}
    </nav>
    <main>
      {#if ui.overviewError}
        <div class="alert error" role="alert"><strong>{t('app.account_unreachable', { name: active?.name })}</strong>{ui.overviewError}</div>
      {/if}
      {#if ui.view === 'send'}
        <Send />
      {:else if ui.view === 'campaigns'}
        <Campaigns />
      {:else if ui.view === 'history'}
        <History />
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
  .brand { display: flex; align-items: center; gap: 10px; font-weight: 700; font-size: 18px; padding: 4px 8px 16px; letter-spacing: -0.01em; }
  .logo { width: 28px; height: 28px; border-radius: 9px; display: grid; place-items: center; color: var(--accent-text); font-size: 15px;
    background: var(--accent); }
  .lab { display: flex; align-items: center; gap: 10px; }
  .lab svg { opacity: 0.75; flex-shrink: 0; }
  .item.current .lab svg { opacity: 1; color: var(--accent); }
  .dot { display: inline-block; width: 8px; height: 8px; border-radius: 50%; background: var(--ok); margin-right: 8px; }
  .dot.bad { background: var(--danger); }
  .item {
    display: flex; justify-content: space-between; align-items: center;
    width: 100%; text-align: left; background: none; border: 1px solid transparent; padding: 8px 10px;
  }
  .item:hover:not(:disabled) { background: var(--surface-2); border-color: transparent; }
  .item { border-radius: 10px; }
  .item.current { background: color-mix(in srgb, var(--accent) 12%, transparent); border-color: transparent; font-weight: 600; }
  .acct { border-top: 1px solid var(--border); padding: 12px 10px 0; }
  .err { color: var(--danger); }
  main { flex: 1; overflow-y: auto; padding: 28px 32px; }


  /* Fenêtre étroite : barre latérale réduite */
  @media (max-width: 900px) {
    nav { width: 168px; }
    .item kbd { display: none; }
    main { padding: 22px 20px; }
  }
  /* Très étroit : la navigation passe en barre horizontale en haut */
  @media (max-width: 700px) {
    .shell { flex-direction: column; }
    nav {
      width: auto; flex-direction: row; align-items: center; gap: 2px; overflow-x: auto;
      padding: 8px 10px; border-right: none; border-bottom: 1px solid var(--border);
    }
    .brand { padding: 4px 10px 4px 4px; }
    .item { width: auto; white-space: nowrap; }
    .acct { border-top: none; padding: 0 6px; white-space: nowrap; }
    .acct .lbl { display: none; }
    main { padding: 16px 14px; }
  }
  @media (max-width: 560px) {
    .acct .lbl2 { display: none; }
  }
</style>
