<script lang="ts">
  import { api, ApiError } from '../lib/api'
  import { t } from '../lib/i18n.svelte'

  let { label = undefined, primary = false }: { label?: string; primary?: boolean } = $props()
  let msg = $state('')
  let err = $state(false)

  async function save() {
    msg = ''
    err = false
    try {
      const p = await api.saveCsvTemplate()
      if (p) msg = t('csvtpl.saved', { path: p })
    } catch (e) {
      err = true
      msg = e instanceof ApiError ? e.message : String(e)
    }
  }
</script>

<button class:primary onclick={save}>{label ?? t('csvtpl.download')}</button>
{#if msg}<span class="small" class:muted={!err} style:color={err ? 'var(--danger)' : undefined}>{msg}</span>{/if}
