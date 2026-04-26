<script>
  /**
   * New project modal content.
   *
   * Props:
   *   config   - app config (uses newProjectDir for the default value)
   *   oncreate - callback({dir}) when the user clicks "Create & Launch"
   *   onclose  - callback to close the modal
   */
  import Modal from "./Modal.svelte";

  let { config = {}, oncreate, onclose } = $props();

  let dir = $state("");
  let inputEl = $state(null);

  // Initialize directory from config
  $effect(() => {
    dir = config.newProjectDir ? config.newProjectDir + "/" : "";
  });

  // Auto-focus input when mounted and place cursor at end
  $effect(() => {
    if (inputEl) {
      inputEl.focus();
      inputEl.setSelectionRange(inputEl.value.length, inputEl.value.length);
    }
  });

  function handleSubmit(e) {
    e.preventDefault();
    const trimmed = dir.trim();
    if (!trimmed) return;
    oncreate?.({ dir: trimmed });
  }
</script>

<Modal open={true} {onclose}>
  <form onsubmit={handleSubmit}>
    <h2><span style="color:var(--primary)">&#9670;</span> New Project</h2>

    <div class="field">
      <label>Project directory</label>
      <input
        type="text"
        bind:this={inputEl}
        bind:value={dir}
        placeholder="/home/user/projects/my-new-app"
      />
      <div class="field-hint">
        The directory will be created if it doesn't exist. Claude Code will launch inside it.
      </div>
    </div>

    <div class="actions">
      <button type="button" class="btn" onclick={onclose}>Cancel</button>
      <button type="submit" class="btn btn-primary">Create &amp; Launch</button>
    </div>
  </form>
</Modal>
