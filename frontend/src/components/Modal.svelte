<script>
  /**
   * Reusable modal wrapper with overlay backdrop.
   *
   * Props:
   *   open     - boolean controlling visibility
   *   onclose  - callback when the user clicks the backdrop or presses Escape
   *   children - slot content (Svelte 5 snippet)
   */

  let { open = false, onclose, children } = $props();

  function handleKeydown(e) {
    if (e.key === "Escape" && open) {
      onclose?.();
    }
  }

  function handleOverlayClick(e) {
    // Only close when clicking the overlay backdrop itself, not the modal content
    if (e.target === e.currentTarget) {
      onclose?.();
    }
  }
</script>

<svelte:window onkeydown={handleKeydown} />

<!-- svelte-ignore a11y_click_events_have_key_events -->
<!-- svelte-ignore a11y_no_static_element_interactions -->
<div
  class="overlay"
  class:open
  onclick={handleOverlayClick}
>
  <div class="modal">
    {#if children}
      {@render children()}
    {/if}
  </div>
</div>
