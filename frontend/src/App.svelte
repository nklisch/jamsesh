<script lang="ts">
  import { current } from '$lib/router.svelte';
</script>

{#if current.name === 'login'}
  <div>Login screen placeholder (login-and-chrome story lands the real Login)</div>
{:else if current.name === 'sessions'}
  <div>Sessions landing placeholder</div>
{:else if current.name === 'session-view'}
  <div>Session view placeholder</div>
{:else}
  <h1>Not Found</h1>
  <a href="/login" onclick={(e) => { e.preventDefault(); window.location.assign('/login'); }}>Go to login</a>
{/if}
