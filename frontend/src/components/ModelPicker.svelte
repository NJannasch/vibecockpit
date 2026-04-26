<script>
  /**
   * Resume-with-model picker modal.
   *
   * Props:
   *   session  - the session being resumed (has .id, .provider, .model)
   *   models   - array of model strings from config.models
   *   onlaunch - callback({sessionId, provider, model}) on confirm
   *   onclose  - callback to close the modal
   */
  import Modal from "./Modal.svelte";

  let { session, models = [], onlaunch, onclose } = $props();

  let selectedModel = $state("");

  // Build a combined model list: current model first, then the rest
  let currentModel = $derived(session?.model || "");

  let allModels = $derived.by(() => {
    if (currentModel) {
      return [currentModel, ...models.filter((m) => m !== currentModel)];
    }
    return models;
  });

  // Default selection to the current model
  $effect(() => {
    selectedModel = currentModel || (models.length > 0 ? models[0] : "");
  });

  function handleResume() {
    if (!session) return;
    onlaunch?.({
      sessionId: session.id,
      provider: session.provider,
      // Only pass model if it differs from current
      model: selectedModel !== currentModel ? selectedModel : undefined,
    });
  }
</script>

<Modal open={true} {onclose}>
  <h2><span style="color:var(--primary)">&#9670;</span> Resume Session</h2>

  <div class="field">
    <label>Model</label>
    <select bind:value={selectedModel}>
      {#each allModels as m}
        <option value={m}>
          {m}{m === currentModel ? " (current)" : ""}
        </option>
      {/each}
    </select>
    <div class="field-hint">
      Pick a model to resume with. The 1M context variants give extended context windows.
    </div>
  </div>

  <div class="actions">
    <button class="btn" onclick={onclose}>Cancel</button>
    <button class="btn btn-primary" onclick={handleResume}>Resume &rarr;</button>
  </div>
</Modal>
