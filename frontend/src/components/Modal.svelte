<script lang="ts">
  import type { Snippet } from 'svelte'

  let { title, onclose, children, actions }: {
    title: string
    onclose: () => void
    children: Snippet
    actions?: Snippet
  } = $props()

  let dialog: HTMLDivElement

  $effect(() => {
    // Focus sur le premier champ (ou le dialogue) à l'ouverture.
    const first = dialog.querySelector<HTMLElement>('input, textarea, select')
    ;(first ?? dialog).focus()
  })

  function onkeydown(e: KeyboardEvent) {
    if (e.key === 'Escape') { e.stopPropagation(); onclose() }
    if (e.key === 'Tab') {
      // Piège le focus dans la modale.
      const f = [...dialog.querySelectorAll<HTMLElement>('button, input, textarea, select')].filter((x) => !x.hasAttribute('disabled'))
      if (!f.length) return
      const first = f[0], last = f[f.length - 1]
      if (e.shiftKey && document.activeElement === first) { e.preventDefault(); last.focus() }
      else if (!e.shiftKey && document.activeElement === last) { e.preventDefault(); first.focus() }
    }
  }
</script>

<!-- svelte-ignore a11y_no_static_element_interactions -->
<div class="backdrop" onkeydown={onkeydown}>
  <div class="dialog" role="dialog" aria-modal="true" aria-label={title} tabindex="-1" bind:this={dialog}>
    <h2>{title}</h2>
    {@render children()}
    {#if actions}<div class="actions">{@render actions()}</div>{/if}
  </div>
</div>

<style>
  .backdrop {
    position: fixed; inset: 0; z-index: 100;
    background: rgba(0, 0, 0, 0.55);
    display: flex; align-items: center; justify-content: center;
  }
  .dialog {
    width: min(460px, calc(100vw - 32px));
    background: var(--surface);
    border: 1px solid var(--border);
    border-radius: 12px;
    box-shadow: var(--shadow);
    padding: 20px;
    outline: none;
    max-height: calc(100vh - 32px);
    overflow-y: auto;
  }
  .actions { display: flex; justify-content: flex-end; gap: 8px; margin-top: 16px; }
</style>
