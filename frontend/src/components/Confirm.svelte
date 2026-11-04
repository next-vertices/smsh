<script lang="ts">
  import type { Snippet } from 'svelte'
  import Modal from './Modal.svelte'

  let { title, confirmLabel = 'Confirmer', danger = false, busy = false, onconfirm, oncancel, children }: {
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
    <button onclick={oncancel} disabled={busy}>Annuler</button>
    <button class={danger ? 'danger solid' : 'primary'} onclick={onconfirm} disabled={busy}>
      {busy ? 'En cours…' : confirmLabel}
    </button>
  {/snippet}
</Modal>
