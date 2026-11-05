<script lang="ts">
  import { api, ApiError } from '../lib/api'

  let { label = 'Télécharger le modèle CSV', primary = false }: { label?: string; primary?: boolean } = $props()
  let msg = $state('')
  let err = $state(false)

  async function save() {
    msg = ''
    err = false
    try {
      const p = await api.saveCsvTemplate()
      if (p) msg = `Modèle enregistré : ${p}`
    } catch (e) {
      err = true
      msg = e instanceof ApiError ? e.message : String(e)
    }
  }
</script>

<button class:primary onclick={save}>{label}</button>
{#if msg}<span class="small" class:muted={!err} style:color={err ? 'var(--danger)' : undefined}>{msg}</span>{/if}
