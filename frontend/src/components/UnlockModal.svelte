<script lang="ts">
  import Modal from './Modal.svelte'
  import { api, ApiError } from '../lib/api'
  import { ui } from '../lib/state.svelte'

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
      error = 'Les deux passphrases ne correspondent pas.'
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

<Modal title={initialized ? 'Déverrouiller le coffre' : 'Créer une passphrase'} onclose={() => ui.unlock?.reject()}>
  <form onsubmit={submit}>
    {#if initialized}
      <p class="muted">Aucun coffre sécurisé n'est disponible sur cet ordinateur : vos clés sont stockées dans un fichier chiffré. Saisissez la passphrase pour y accéder.</p>
    {:else}
      <p class="muted">Aucun coffre sécurisé n'est disponible sur cet ordinateur. Vos clés seront stockées dans un fichier chiffré par une passphrase. <strong>Elle ne peut pas être récupérée en cas d'oubli.</strong></p>
    {/if}
    <div class="field">
      <label for="pp">Passphrase</label>
      <input id="pp" type="password" bind:value={pass} autocomplete="off" />
    </div>
    {#if !initialized}
      <div class="field">
        <label for="pp2">Confirmation</label>
        <input id="pp2" type="password" bind:value={confirm} autocomplete="off" />
        <div class="hint">8 caractères minimum.</div>
      </div>
    {/if}
    {#if error}<div class="alert error">{error}</div>{/if}
    <div class="end">
      <button type="button" onclick={() => ui.unlock?.reject()}>Annuler</button>
      <button type="submit" class="primary" disabled={busy || !pass}>{initialized ? 'Déverrouiller' : 'Créer'}</button>
    </div>
  </form>
</Modal>

<style>
  .end { display: flex; justify-content: flex-end; gap: 8px; margin-top: 8px; }
</style>
