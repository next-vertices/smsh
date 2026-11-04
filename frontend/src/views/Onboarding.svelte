<script lang="ts">
  import AccountForm from '../components/AccountForm.svelte'
  import { refreshAll } from '../lib/actions'
  import { ui } from '../lib/state.svelte'

  async function saved() {
    await refreshAll()
    ui.view = 'send'
  }
</script>

<div class="wrap">
  <div class="card">
    <h1>Bienvenue dans Smsh</h1>
    <p class="muted">
      Configurez votre premier compte OVH. Vos clés sont stockées dans le coffre de votre système
      et ne quittent jamais cet ordinateur.
    </p>
    <AccountForm onsaved={saved} />
  </div>
</div>

<style>
  .wrap { max-width: 640px; margin: 0 auto; padding: 40px 24px; }
  @media (max-width: 640px) { .wrap { padding: 14px 10px; } }
</style>
