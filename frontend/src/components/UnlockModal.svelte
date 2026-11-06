<script lang="ts">
  import Modal from './Modal.svelte'
  import { api, ApiError } from '../lib/api'
  import { ui } from '../lib/state.svelte'
  import { t } from '../lib/i18n.svelte'

  // `initialized` false : premier usage, la passphrase est créée.
  let { initialized }: { initialized: boolean } = $props()

  let pass = $state('')
  let confirm = $state('')
  let error = $state('')
  let busy = $state(false)

  async function submit(e: Event) {
    e.preventDefault()
    error = ''
    if (!initialized && pass !== confirm) {
      error = t('unlock.mismatch')
      return
    }
    busy = true
    try {
      // Appel direct : on ne veut pas de boucle de déverrouillage ici.
      await api.unlock(pass)
      if (ui.status) { ui.status.locked = false; ui.status.initialized = true }
      ui.unlock?.resolve()
    } catch (err) {
      error = err instanceof ApiError ? err.message : String(err)
    } finally {
      busy = false
    }
  }
</script>

<Modal title={initialized ? t('unlock.title_unlock') : t('unlock.title_create')} onclose={() => ui.unlock?.reject()}>
  <form onsubmit={submit}>
    {#if initialized}
      <p class="muted">{t('unlock.text_unlock')}</p>
    {:else}
      <p class="muted">{t('unlock.text_create')} <strong>{t('unlock.text_create_warn')}</strong></p>
    {/if}
    <div class="field">
      <label for="pp">{t('unlock.passphrase')}</label>
      <input id="pp" type="password" bind:value={pass} autocomplete="off" />
    </div>
    {#if !initialized}
      <div class="field">
        <label for="pp2">{t('unlock.confirmation')}</label>
        <input id="pp2" type="password" bind:value={confirm} autocomplete="off" />
        <div class="hint">{t('unlock.min8')}</div>
      </div>
    {/if}
    {#if error}<div class="alert error">{error}</div>{/if}
    <div class="end">
      <button type="button" onclick={() => ui.unlock?.reject()}>{t('common.cancel')}</button>
      <button type="submit" class="primary" disabled={busy || !pass}>{initialized ? t('unlock.submit_unlock') : t('unlock.submit_create')}</button>
    </div>
  </form>
</Modal>

<style>
  .end { display: flex; justify-content: flex-end; gap: 8px; margin-top: 8px; }
</style>
