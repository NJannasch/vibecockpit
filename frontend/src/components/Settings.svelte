<script>
  /**
   * Settings modal content.
   *
   * Props:
   *   config   - app config object (has availableTerminals, terminal, newProjectDir, providerPaths)
   *   sessions - session array (used to derive which providers exist)
   *   onsave   - callback({terminal, newProjectDir, providerPaths})
   *   onclose  - callback to close the modal
   */
  import Modal from "./Modal.svelte";

  let { config = {}, sessions = [], onsave, onclose } = $props();

  let terminal = $state("");
  let newProjectDir = $state("");
  let providerPaths = $state({});

  // Derive provider list from sessions
  let providers = $derived(
    [...new Set(sessions.map((s) => s.provider).filter(Boolean))]
  );

  let availableTerminals = $derived(config.availableTerminals || ["default"]);

  // Initialize local state from config when it changes
  $effect(() => {
    terminal = config.terminal || "default";
    newProjectDir = config.newProjectDir || "";
    // Deep-copy provider paths so we don't mutate the original
    const paths = config.providerPaths || {};
    const init = {};
    for (const p of providers) {
      init[p] = paths[p] || "";
    }
    providerPaths = init;
  });

  function terminalLabel(t) {
    if (t === "default") return "default (auto-detect)";
    if (t === "custom") return "custom (custom command)";
    return t;
  }

  function handleSave() {
    const cleanedPaths = {};
    for (const [key, val] of Object.entries(providerPaths)) {
      const trimmed = val.trim();
      if (trimmed) cleanedPaths[key] = trimmed;
    }
    onsave?.({ terminal, newProjectDir, providerPaths: cleanedPaths });
  }
</script>

<Modal open={true} {onclose}>
  <h2><span>&#9881;</span> Settings</h2>

  <div class="field">
    <label>Terminal emulator</label>
    <select bind:value={terminal}>
      {#each availableTerminals as t (t)}
        <option value={t}>{terminalLabel(t)}</option>
      {/each}
    </select>
    <div class="field-hint">
      Which terminal to open when resuming a session from the web UI.
    </div>
  </div>

  <div class="field">
    <label>Default new project directory</label>
    <input type="text" bind:value={newProjectDir} placeholder="/home/user/projects" />
  </div>

  <div class="field">
    <label>Provider binary paths</label>
    <div class="field-hint" style="margin-bottom:.4rem">
      Override if a tool isn't in your PATH (e.g. installed via nvm).
    </div>
    {#each providers as p (p)}
      <div style="display:flex;gap:.5rem;margin-bottom:.4rem;align-items:center">
        <label style="width:5rem;font-size:.8rem;font-weight:500">{p}</label>
        <input
          type="text"
          bind:value={providerPaths[p]}
          placeholder="auto-detect from PATH"
          style="flex:1;padding:.4rem .6rem;font-size:.8rem"
        />
      </div>
    {/each}
  </div>

  <div class="actions">
    <button class="btn" onclick={onclose}>Cancel</button>
    <button class="btn btn-primary" onclick={handleSave}>Save</button>
  </div>
</Modal>
