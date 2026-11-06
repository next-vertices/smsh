<script lang="ts">
  import type { Snippet } from 'svelte'
  import Modal from './Modal.svelte'
  import { t } from '../lib/i18n.svelte'

  let { title, confirmLabel = undefined, danger = false, busy = false, onconfirm, oncancel, children }: {
    title: string
    confirmLabel?: string
    danger?: boolean
    busy?: boolean
    onconfirm: () => void
    oncancel: () => void
    children: Snippet
  } = $props()
</script>

<Modal {title} onclose={oncancel}>
  {@render children()}
  {#snippet actions()}
    <button onclick={oncancel} disabled={busy}>{t('common.cancel')}</button>
    <button class={danger ? 'danger solid' : 'primary'} onclick={onconfirm} disabled={busy}>
      {busy ? t('common.in_progress') : (confirmLabel ?? t('common.confirm'))}
    </button>
  {/snippet}
</Modal>
