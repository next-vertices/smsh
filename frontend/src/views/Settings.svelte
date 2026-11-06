<script lang="ts">
  import AccountForm from '../components/AccountForm.svelte'
  import Confirm from '../components/Confirm.svelte'
  import { api, ApiError, type AccountView } from '../lib/api'
  import { refreshAll } from '../lib/actions'
  import { setTheme, ui } from '../lib/state.svelte'
  import { i18n, setLangPref, t, type LangPref } from '../lib/i18n.svelte'

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

  const backendLabel = (b: string) => (['keyring', 'age', 'memory'].includes(b) ? t(`settings.backend_${b}`) : '—')

  function changeLang(e: Event) {
    setLangPref((e.currentTarget as HTMLSelectElement).value as LangPref, api.systemLanguage, api.setLanguage)
  }
</script>

<h1>{t('settings.title')}</h1>
<p class="muted">{t('settings.subtitle')}</p>

<div class="settings">
<div class="col-main">
{#if error}<div class="alert error" role="alert">{error}</div>{/if}

{#if editing}
  <div class="card">
    <h2>{editing === 'new' ? t('settings.new_account') : t('settings.edit_account', { name: editing.name })}</h2>
    <AccountForm account={editing === 'new' ? undefined : editing} oncancel={() => (editing = null)} onsaved={saved} />
  </div>
{:else}
  <div class="card">
    <div class="row between" style="margin-bottom: 12px">
      <h2 style="margin: 0">{t('settings.accounts')}</h2>
      <button class="primary" onclick={() => (editing = 'new')}>{t('settings.add_account')}</button>
    </div>
    {#each ui.accounts as a (a.id)}
      <div class="acc">
        <div class="grow">
          <strong>{a.name}</strong>
          {#if a.active}<span class="badge accent">{t('settings.active')}</span>{/if}
          <div class="muted small mono">{a.serviceName} · {a.endpoint} · {a.appKey}</div>
        </div>
        {#if !a.active}
          <button onclick={() => run(() => api.setActive(a.id))} disabled={busy}>{t('settings.activate')}</button>
        {/if}
        <button onclick={() => (editing = a)} disabled={busy}>{t('settings.edit')}</button>
        <button class="danger" onclick={() => (deleting = a)} disabled={busy}>{t('settings.delete')}</button>
      </div>
    {:else}
      <p class="muted">{t('settings.no_account')}</p>
    {/each}
  </div>
{/if}

</div>
<div class="col-side">
<div class="card">
  <h2>{t('settings.appearance')}</h2>
  <div class="row">
    <button class:primary={ui.theme === 'dark'} onclick={() => setTheme('dark')}>{t('settings.dark')}</button>
    <button class:primary={ui.theme === 'light'} onclick={() => setTheme('light')}>{t('settings.light')}</button>
  </div>
  <div class="field" style="margin: 14px 0 0">
    <label for="lang">{t('settings.language')}</label>
    <select id="lang" value={i18n.pref} onchange={changeLang}>
      <option value="auto">{t('settings.lang_auto')}</option>
      <option value="fr-FR">{t('settings.lang_fr')}</option>
      <option value="en-GB">{t('settings.lang_en')}</option>
    </select>
  </div>
</div>

<div class="card">
  <h2>{t('settings.key_storage')}</h2>
  <p class="muted" style="margin: 0">
    {backendLabel(ui.status?.backend ?? '')}<br />
    {t('settings.storage_note')}
  </p>
</div>
</div>
</div>

{#if deleting}
  <Confirm title={t('settings.delete_title')} confirmLabel={t('settings.delete')} danger {busy} onconfirm={confirmDelete} oncancel={() => (deleting = null)}>
    <p>{t('settings.delete_pre')} <strong>{deleting.name}</strong> {t('settings.delete_post')}</p>
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
