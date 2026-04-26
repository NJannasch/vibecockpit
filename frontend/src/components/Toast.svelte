<script>
  /**
   * Toast notification system.
   *
   * Manages its own array of toasts with auto-dismiss.
   * Export an `addToast` function that parent components can call via bind:this.
   *
   * Each toast: { id, message, type ('success'|'error'), actionFn?, actionLabel?, fading }
   * - Auto-dismiss after 3s (or 8s if toast has an actionFn)
   * - Toasts slide in from the right, fade out before removal
   */

  let toasts = $state([]);
  let nextId = 0;

  export function addToast(message, type = "success", actionFn = null, actionLabel = "Open Settings") {
    const id = nextId++;
    const toast = { id, message, type, actionFn, actionLabel, fading: false };
    toasts.push(toast);

    const timeout = actionFn ? 8000 : 3000;
    setTimeout(() => fadeOut(id), timeout);
  }

  function fadeOut(id) {
    const toast = toasts.find((t) => t.id === id);
    if (toast) {
      toast.fading = true;
      setTimeout(() => removeToast(id), 200);
    }
  }

  function removeToast(id) {
    toasts = toasts.filter((t) => t.id !== id);
  }

  function handleAction(toast) {
    removeToast(toast.id);
    toast.actionFn?.();
  }
</script>

<div class="toast-area">
  {#each toasts as toast (toast.id)}
    <div
      class="toast {toast.type}"
      style={toast.fading ? "opacity:0;transition:opacity 200ms ease" : ""}
    >
      {toast.message}
      {#if toast.actionFn}
        <button
          class="btn btn-sm"
          style="margin-left:.6rem;padding:.2rem .5rem;font-size:.75rem"
          onclick={() => handleAction(toast)}
        >
          {toast.actionLabel}
        </button>
      {/if}
    </div>
  {/each}
</div>
