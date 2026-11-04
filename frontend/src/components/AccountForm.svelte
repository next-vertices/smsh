<script lang="ts">
  import { api, ApiError, emptyInput, type AccountView, type ServiceInfo } from '../lib/api'
  import { BrowserOpenURL } from '../../wailsjs/runtime/runtime'

  let { account, oncancel, onsaved }: {
    account?: AccountView
    oncancel?: () => void
    onsaved: () => void
  } = $props()

  // svelte-ignore state_referenced_locally
  const editing = !!account
  // svelte-ignore state_referenced_locally
  let form = $state(
    account
      ? { ...emptyInput(), id: account.id, name: account.name, serviceName: account.serviceName, endpoint: account.endpoint }
      : emptyInput(),
  )
  let endpoints = $state<string[]>(['ovh-eu'])
  let testing = $state(false)
  let saving = $state(false)
  let testResult = $state<ServiceInfo | null>(null)
  let error = $state('')

  api.endpoints().then((e) => (endpoints = e)).catch(() => {})

  const tokenUrls: Record<string, string> = {
    'ovh-eu': 'https://eu.api.ovh.com/createToken/',
    'ovh-ca': 'https://ca.api.ovh.com/createToken/',
    'ovh-us': 'https://api.us.ovhcloud.com/createToken/',
  }

  function message(e: unknown) {
    return e instanceof ApiError ? e.message : String(e)
  }

  async function test() {
    error = ''
    testResult = null
    testing = true
    try {
      testResult = await api.testConnection($state.snapshot(form))
    } catch (e) {
      error = message(e)
    } finally {
      testing = false
    }
  }

  async function save(e: Event) {
    e.preventDefault()
    error = ''
    saving = true
    try {
      await api.saveAccount($state.snapshot(form))
      onsaved()
    } catch (err) {
      error = message(err)
    } finally {
      saving = false
    }
  }
</script>

<form onsubmit={save}>
  <div class="grid2">
    <div class="field">
      <label for="acc-name">Nom du compte</label>
      <input id="acc-name" type="text" bind:value={form.name} placeholder="Perso, Boulot…" />
    </div>
    <div class="field">
      <label for="acc-ep">Région OVH</label>
      <select id="acc-ep" bind:value={form.endpoint}>
        {#each endpoints as ep}<option value={ep}>{ep}</option>{/each}
      </select>
    </div>
  </div>

  <div class="field">
    <label for="acc-svc">serviceName</label>
    <input id="acc-svc" type="text" bind:value={form.serviceName} placeholder="sms-ab123456-1" autocomplete="off" />
    <div class="hint">Visible dans l'espace client OVH, rubrique Télécom → SMS.</div>
  </div>

  <div class="grid2">
    <div class="field">
      <label for="acc-ak">Application key</label>
      <input id="acc-ak" type="password" bind:value={form.appKey} autocomplete="off"
        placeholder={editing ? account?.appKey : ''} />
    </div>
    <div class="field">
      <label for="acc-as">Application secret</label>
      <input id="acc-as" type="password" bind:value={form.appSecret} autocomplete="off"
        placeholder={editing ? account?.appSecret : ''} />
    </div>
  </div>
  <div class="field">
    <label for="acc-ck">Consumer key</label>
    <input id="acc-ck" type="password" bind:value={form.consumerKey} autocomplete="off"
      placeholder={editing ? account?.consumerKey : ''} />
    <div class="hint">
      {#if editing}Laissez vide un champ pour conserver sa valeur actuelle. {/if}
      Générez les clés sur
      <button type="button" class="link" onclick={() => BrowserOpenURL(tokenUrls[form.endpoint] ?? tokenUrls['ovh-eu'])}>la page de création de token OVH</button>
      avec les droits <span class="mono">GET/POST/DELETE /sms/*</span>.
    </div>
  </div>

  {#if error}
    <div class="alert error" role="alert"><strong>Échec</strong>{error}</div>
  {/if}
  {#if testResult}
    <div class="alert ok" role="status">
      <strong>Connexion réussie</strong>
      Service {testResult.serviceName} · {testResult.creditsLeft} crédit(s) · {testResult.senders.length} expéditeur(s)
    </div>
  {/if}

  <div class="row between">
    <button type="button" onclick={test} disabled={testing || saving}>
      {testing ? 'Test en cours…' : 'Tester la connexion'}
    </button>
    <div class="row">
      {#if oncancel}<button type="button" onclick={oncancel} disabled={saving}>Annuler</button>{/if}
      <button type="submit" class="primary" disabled={saving || testing}>
        {saving ? 'Enregistrement…' : 'Enregistrer'}
      </button>
    </div>
  </div>
</form>
