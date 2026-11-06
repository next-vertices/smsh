<script lang="ts">
  import { api, ApiError, emptyInput, type AccountView, type ServiceInfo } from '../lib/api'
  import { BrowserOpenURL } from '../../wailsjs/runtime/runtime'
  import { t } from '../lib/i18n.svelte'

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
      <label for="acc-name">{t('account.name')}</label>
      <input id="acc-name" type="text" bind:value={form.name} placeholder={t('account.name_ph')} />
    </div>
    <div class="field">
      <label for="acc-ep">{t('account.region')}</label>
      <select id="acc-ep" bind:value={form.endpoint}>
        {#each endpoints as ep}<option value={ep}>{ep}</option>{/each}
      </select>
    </div>
  </div>

  <div class="field">
    <label for="acc-svc">serviceName</label>
    <input id="acc-svc" type="text" bind:value={form.serviceName} placeholder="sms-ab123456-1" autocomplete="off" />
    <div class="hint">{t('account.svc_hint')}</div>
  </div>

  <div class="grid2">
    <div class="field">
      <label for="acc-ak">{t('account.app_key')}</label>
      <input id="acc-ak" type="password" bind:value={form.appKey} autocomplete="off"
        placeholder={editing ? account?.appKey : ''} />
    </div>
    <div class="field">
      <label for="acc-as">{t('account.app_secret')}</label>
      <input id="acc-as" type="password" bind:value={form.appSecret} autocomplete="off"
        placeholder={editing ? account?.appSecret : ''} />
    </div>
  </div>
  <div class="field">
    <label for="acc-ck">{t('account.consumer_key')}</label>
    <input id="acc-ck" type="password" bind:value={form.consumerKey} autocomplete="off"
      placeholder={editing ? account?.consumerKey : ''} />
    <div class="hint">
      {#if editing}{t('account.keep_hint')} {/if}
      {t('account.generate_pre')}
      <button type="button" class="link" onclick={() => BrowserOpenURL(tokenUrls[form.endpoint] ?? tokenUrls['ovh-eu'])}>{t('account.token_page')}</button>
      {t('account.with_rights')} <span class="mono">GET/POST/DELETE /sms/*</span>.
    </div>
  </div>

  {#if error}
    <div class="alert error" role="alert"><strong>{t('account.failure')}</strong>{error}</div>
  {/if}
  {#if testResult}
    <div class="alert ok" role="status">
      <strong>{t('account.test_ok')}</strong>
      {t('account.test_summary', { service: testResult.serviceName, credits: testResult.creditsLeft, senders: testResult.senders.length })}
    </div>
  {/if}

  <div class="row between">
    <button type="button" onclick={test} disabled={testing || saving}>
      {testing ? t('account.testing') : t('account.test')}
    </button>
    <div class="row">
      {#if oncancel}<button type="button" onclick={oncancel} disabled={saving}>{t('common.cancel')}</button>{/if}
      <button type="submit" class="primary" disabled={saving || testing}>
        {saving ? t('account.saving') : t('account.save')}
      </button>
    </div>
  </div>
</form>
