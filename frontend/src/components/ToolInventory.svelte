<script>
  import { onMount } from "svelte";
  import { fetchInventory, fetchInventoryFile } from "../lib/api.js";
  import { providerColors, shortModel, relativeTime } from "../lib/utils.js";

  let inventory = $state(null);
  let loading = $state(true);
  let error = $state(null);
  let tab = $state("overview");
  let expandedTools = $state(new Set());
  let auditOpen = $state(false);
  let auditFilter = $state("");
  let fileViewer = $state({ open: false, path: "", content: "", loading: false, error: null });

  onMount(async () => {
    try {
      inventory = await fetchInventory();
    } catch (e) {
      error = e.message;
    } finally {
      loading = false;
    }
  });

  async function refresh() {
    loading = true;
    try {
      inventory = await fetchInventory(true);
      error = null;
    } catch (e) {
      error = e.message;
    } finally {
      loading = false;
    }
  }

  function toggleTool(id) {
    let next = new Set(expandedTools);
    if (next.has(id)) next.delete(id);
    else next.add(id);
    expandedTools = next;
  }

  async function openFile(path) {
    fileViewer = { open: true, path, content: "", loading: true, error: null };
    try {
      const data = await fetchInventoryFile(path);
      fileViewer = { open: true, path, content: data.content, loading: false, error: null };
    } catch (e) {
      fileViewer = { open: true, path, content: "", loading: false, error: e.message };
    }
  }

  function closeFile() {
    fileViewer = { open: false, path: "", content: "", loading: false, error: null };
  }

  function homePath(p) {
    const home = "/home/";
    if (p?.startsWith(home)) {
      const rest = p.slice(home.length);
      const i = rest.indexOf("/");
      if (i >= 0) return "~" + rest.slice(i);
    }
    return p || "";
  }

  let installedTools = $derived(inventory?.tools?.filter(t => t.installed) ?? []);
  let notInstalledTools = $derived(inventory?.tools?.filter(t => !t.installed) ?? []);
  let mcpCount = $derived(inventory?.mcpServers?.length ?? 0);
  let instrCount = $derived(inventory?.instructionFiles?.length ?? 0);
  let skillCount = $derived(inventory?.skills?.length ?? 0);
  let memoryCount = $derived(inventory?.memories?.length ?? 0);
  let sensitiveCount = $derived(inventory?.sensitiveFiles?.length ?? 0);
  let extCount = $derived(inventory?.ideExtensions?.length ?? 0);

  let filteredScanLog = $derived.by(() => {
    const log = inventory?.scanLog ?? [];
    if (auditFilter === "found") return log.filter(e => e.found);
    if (auditFilter === "missed") return log.filter(e => !e.found);
    return log;
  });

  let instructionsByProject = $derived.by(() => {
    if (!inventory?.instructionFiles?.length) return {};
    const groups = {};
    for (const f of inventory.instructionFiles) {
      const key = f.projectName || "unknown";
      if (!groups[key]) groups[key] = [];
      groups[key].push(f);
    }
    return groups;
  });

  let memoriesByProject = $derived.by(() => {
    if (!inventory?.memories?.length) return {};
    const groups = {};
    for (const m of inventory.memories) {
      const key = m.projectName || "unknown";
      if (!groups[key]) groups[key] = [];
      groups[key].push(m);
    }
    return groups;
  });

  let extByIDE = $derived.by(() => {
    if (!inventory?.ideExtensions?.length) return {};
    const groups = {};
    for (const e of inventory.ideExtensions) {
      const key = e.ide || "unknown";
      if (!groups[key]) groups[key] = [];
      groups[key].push(e);
    }
    return groups;
  });

  const memoryTypeColors = {
    "user": "#7c3aed",
    "feedback": "#f97316",
    "project": "#06b6d4",
    "reference": "#10b981",
  };

  const sourceColors = {
    "claude-global": "#7c3aed",
    "claude-project": "#a78bfa",
    "claude-desktop": "#8b5cf6",
    "claude-plugin": "#c084fc",
    "cursor": "#f97316",
    "windsurf": "#06b6d4",
    "cline": "#22d3ee",
    "roo-code": "#10b981",
    "zed": "#f59e0b",
    "junie": "#ec4899",
    "copilot": "#1f883d",
    "gemini": "#d97706",
    "continue": "#e11d48",
    "amazonq": "#ff9900",
    "tabnine": "#6b21a8",
    "amp": "#0ea5e9",
    "devin": "#7c3aed",
    "kilo": "#14b8a6",
    "qwen": "#3b82f6",
    "goose": "#84cc16",
    "project": "#6366f1",
  };

  function sourceLabel(source) {
    const map = {
      "claude-global": "Claude Code",
      "claude-global (permissions)": "Claude Code",
      "claude-project": "Claude Project",
      "claude-project (permissions)": "Claude Project",
      "claude-desktop": "Claude Desktop",
      "claude-plugin": "Plugin",
      "cursor": "Cursor",
      "windsurf": "Windsurf",
      "cline": "Cline",
      "roo-code": "Roo Code",
      "zed": "Zed",
      "junie": "Junie",
      "copilot": "Copilot",
      "gemini": "Gemini",
      "continue": "Continue",
      "amazonq": "Amazon Q",
      "tabnine": "Tabnine",
      "amp": "Amp",
      "devin": "Devin",
      "kilo": "Kilo Code",
      "qwen": "Qwen",
      "goose": "Goose",
      "project": "Project",
      "project (permissions)": "Project",
    };
    return map[source] || source;
  }

  function sourceColor(source) {
    const base = source.replace(/ \(permissions\)$/, "");
    return sourceColors[base] || "var(--text-secondary)";
  }

  function fileTypeIcon(type) {
    const map = {
      "CLAUDE.md": "C", "claude-agents": "C",
      ".cursorrules": "Cu", "cursor-rules": "Cu",
      "copilot-instructions.md": "Cp",
      "GEMINI.md": "G",
      "agents.md": "A", "AGENTS.md": "A",
      "CODEX.md": "Cx", "codex.md": "Cx", ".codexrc": "Cx",
      ".windsurfrules": "Ws", "windsurf-rules": "Ws",
      ".aiignore": "Ai", ".aider.conf.yml": "Ad",
      ".rooignore": "Ro", "roo-rules": "Ro",
    };
    return map[type] || "?";
  }

  function fileTypeColor(type) {
    const map = {
      "CLAUDE.md": "#7c3aed", "claude-agents": "#a78bfa",
      ".cursorrules": "#f97316", "cursor-rules": "#f97316",
      "copilot-instructions.md": "#059669",
      "GEMINI.md": "#d97706",
      "agents.md": "#6366f1", "AGENTS.md": "#6366f1",
      ".windsurfrules": "#06b6d4", "windsurf-rules": "#06b6d4",
      ".aiignore": "#94a3b8", ".aider.conf.yml": "#94a3b8",
      ".rooignore": "#10b981", "roo-rules": "#10b981",
      "CODEX.md": "#e879f9", "codex.md": "#e879f9", ".codexrc": "#e879f9",
    };
    return map[type] || "var(--text-secondary)";
  }

  function formatBytes(bytes) {
    if (bytes < 1024) return bytes + " B";
    return (bytes / 1024).toFixed(1) + " KB";
  }

  function toolColor(tool) {
    return providerColors[tool.id]
      || providerColors[tool.id.replace('-cli', '')]
      || providerColors[tool.id.replace('-ide', '')]
      || 'var(--primary)';
  }

  function fileBasename(path) {
    return path?.split("/").pop() || path;
  }

  let tabs = $derived([
    { id: "overview", label: "Overview", count: installedTools.length },
    { id: "extensions", label: "IDE Extensions", count: extCount },
    { id: "mcp", label: "MCP Servers", count: mcpCount },
    { id: "instructions", label: "Instructions", count: instrCount },
    { id: "skills", label: "Skills", count: skillCount },
    { id: "memories", label: "Memories", count: memoryCount },
  ]);
</script>

<div class="inventory">
  <!-- Header -->
  <div class="inv-header">
    <div>
      <h2>Tool Inventory</h2>
      <p class="inv-subtitle">Your vibe coding setup at a glance</p>
    </div>
    <div class="inv-header-right">
      {#if inventory?.scanDurationMs}
        <span class="scan-time">{inventory.scanDurationMs}ms</span>
      {/if}
      <button class="btn btn-sm" onclick={refresh} disabled={loading}>
        {loading ? "Scanning..." : "Refresh"}
      </button>
    </div>
  </div>

  {#if error}
    <div class="inv-error">{error}</div>
  {:else if loading && !inventory}
    <div class="inv-loading">Scanning your setup...</div>
  {:else if inventory}

  <!-- Summary strip -->
  <div class="summary-strip">
    <div class="summary-stat">
      <span class="stat-value">{installedTools.length}</span>
      <span class="stat-label">tools</span>
    </div>
    <div class="summary-stat">
      <span class="stat-value">{inventory.models?.length ?? 0}</span>
      <span class="stat-label">models</span>
    </div>
    <div class="summary-stat">
      <span class="stat-value">{extCount}</span>
      <span class="stat-label">AI extensions</span>
    </div>
    <div class="summary-stat">
      <span class="stat-value">{mcpCount}</span>
      <span class="stat-label">MCP servers</span>
    </div>
    <div class="summary-stat">
      <span class="stat-value">{instrCount}</span>
      <span class="stat-label">instruction files</span>
    </div>
    <div class="summary-stat">
      <span class="stat-value">{memoryCount}</span>
      <span class="stat-label">memories</span>
    </div>
    <div class="summary-stat">
      <span class="stat-value">{inventory.projectPaths?.length ?? 0}</span>
      <span class="stat-label">projects scanned</span>
    </div>
  </div>

  <!-- Sensitive files warning -->
  {#if sensitiveCount > 0}
    <details class="sensitive-banner">
      <summary class="sensitive-header">
        <span class="sensitive-icon">!</span>
        <span>{sensitiveCount} sensitive {sensitiveCount === 1 ? "file" : "files"} detected in your projects</span>
        <span class="sensitive-hint">These files may contain secrets — ensure they are in <code>.gitignore</code></span>
      </summary>
      <div class="sensitive-list">
        {#each inventory.sensitiveFiles as f}
          <div class="sensitive-row">
            <code class="sensitive-name">{f.name}</code>
            <span class="sensitive-project">{f.projectName}</span>
            <span class="sensitive-risk">{f.risk}</span>
          </div>
        {/each}
      </div>
    </details>
  {/if}

  <!-- Tabs -->
  <div class="inv-tabs">
    {#each tabs as t}
      <button class="inv-tab" class:active={tab === t.id} onclick={() => tab = t.id}>
        {t.label}
        {#if t.count > 0}<span class="tab-count">{t.count}</span>{/if}
      </button>
    {/each}
  </div>

  <!-- Tab content -->
  <div class="tab-content">

    <!-- OVERVIEW -->
    {#if tab === "overview"}
      <div class="tool-grid">
        {#each installedTools as tool}
          <button class="tool-card" class:expanded={expandedTools.has(tool.id)} onclick={() => toggleTool(tool.id)}>
            <div class="tool-top">
              <span class="tool-dot" style="background:{toolColor(tool)}"></span>
              <span class="tool-name">{tool.name}</span>
              {#if tool.version}
                <span class="tool-ver">{tool.version.split(" ")[0]}</span>
              {/if}
            </div>
            {#if expandedTools.has(tool.id)}
              <div class="tool-details">
                {#if tool.version}<div class="tool-row"><span class="row-label">Version</span><span>{tool.version}</span></div>{/if}
                {#if tool.binaryPath}<div class="tool-row"><span class="row-label">Binary</span><code>{homePath(tool.binaryPath)}</code></div>{/if}
                {#if tool.configDir}<div class="tool-row"><span class="row-label">Config</span><code>{homePath(tool.configDir)}</code></div>{/if}
                {#if tool.dataDir}<div class="tool-row"><span class="row-label">Data</span><code>{homePath(tool.dataDir)}</code></div>{/if}
              </div>
            {/if}
          </button>
        {/each}
        {#each notInstalledTools as tool}
          <div class="tool-card dimmed">
            <div class="tool-top">
              <span class="tool-dot" style="background:var(--text-tertiary, var(--text-secondary))"></span>
              <span class="tool-name">{tool.name}</span>
              <span class="tool-absent">Not found</span>
            </div>
          </div>
        {/each}
      </div>

      <!-- Models table -->
      {#if inventory.models?.length}
        <h3 class="sub-heading">Models Used</h3>
        <div class="table-wrap">
          <table class="inv-table">
            <thead><tr><th>Model</th><th>Provider</th><th>Sessions</th><th>Last used</th></tr></thead>
            <tbody>
              {#each inventory.models as m}
                <tr>
                  <td><code>{shortModel(m.model)}</code></td>
                  <td><span class="chip" style="--c:{providerColors[m.provider] || 'var(--primary)'}">{m.provider}</span></td>
                  <td class="num">{m.sessionCount}</td>
                  <td class="muted">{relativeTime(m.lastUsed)}</td>
                </tr>
              {/each}
            </tbody>
          </table>
        </div>
      {/if}

    <!-- IDE EXTENSIONS -->
    {:else if tab === "extensions"}
      {#if extCount === 0}
        <div class="empty">No AI-related IDE extensions detected. Install extensions like GitHub Copilot, Continue, or Cline in VS Code or Cursor.</div>
      {:else}
        {#each Object.entries(extByIDE) as [ide, exts]}
          <div class="ext-group">
            <div class="ext-ide">
              <span class="ext-ide-dot" style="background:{sourceColors[exts[0]?.ideId] || 'var(--primary)'}"></span>
              {ide}
              <span class="ext-ide-count">{exts.length}</span>
            </div>
            <div class="ext-list">
              {#each exts as ext}
                <div class="ext-row">
                  <span class="ext-name">{ext.name}</span>
                  <code class="ext-id">{ext.id}</code>
                  <span class="ext-ver">{ext.version}</span>
                </div>
                {#if ext.description}
                  <div class="ext-desc">{ext.description}</div>
                {/if}
              {/each}
            </div>
          </div>
        {/each}
      {/if}

    <!-- MCP SERVERS -->
    {:else if tab === "mcp"}
      {#if mcpCount === 0}
        <div class="empty">No MCP servers detected. Configure servers in your tool settings or project <code>.mcp.json</code>.</div>
      {:else}
        <div class="mcp-list">
          {#each inventory.mcpServers as s}
            <div class="mcp-card">
              <div class="mcp-top">
                <span class="mcp-name">{s.name}</span>
                <span class="chip" style="--c:{sourceColor(s.source)}">{sourceLabel(s.source)}</span>
              </div>
              <div class="mcp-meta">
                {#if s.command}
                  <code class="mcp-cmd">{s.command}{s.args?.length ? " " + s.args.join(" ") : ""}</code>
                {:else if s.url}
                  <code class="mcp-cmd">{s.url}</code>
                {/if}
                <span class="mcp-scope">{s.scope === "global" ? "Global" : homePath(s.scope)}</span>
              </div>
              {#if s.sourcePath}
                <button class="link-btn" onclick={() => openFile(s.sourcePath)}>View config</button>
              {/if}
            </div>
          {/each}
        </div>
      {/if}

    <!-- INSTRUCTIONS -->
    {:else if tab === "instructions"}
      {#if instrCount === 0}
        <div class="empty">No instruction files detected. Add a <code>CLAUDE.md</code>, <code>.cursorrules</code>, or other instruction files to your projects.</div>
      {:else}
        {#each Object.entries(instructionsByProject) as [project, files]}
          <div class="instr-group">
            <div class="instr-project">{project}</div>
            <div class="instr-list">
              {#each files as f}
                <button class="instr-row" onclick={() => openFile(f.path)}>
                  <span class="instr-icon" style="background:{fileTypeColor(f.type)}">{fileTypeIcon(f.type)}</span>
                  <span class="instr-name">{fileBasename(f.path)}</span>
                  <span class="instr-type">{f.type}</span>
                  <span class="instr-size">{formatBytes(f.sizeBytes)}</span>
                  <span class="instr-time">{relativeTime(f.modified)}</span>
                  <span class="instr-view">View</span>
                </button>
              {/each}
            </div>
          </div>
        {/each}
      {/if}

    <!-- SKILLS -->
    {:else if tab === "skills"}
      {#if skillCount === 0}
        <div class="empty">No custom skills or commands detected. Add files to <code>~/.claude/commands/</code> or project <code>.claude/commands/</code>.</div>
      {:else}
        <div class="table-wrap">
          <table class="inv-table">
            <thead><tr><th>Name</th><th>Type</th><th>Source</th><th></th></tr></thead>
            <tbody>
              {#each inventory.skills as s}
                <tr>
                  <td><code>/{s.name}</code></td>
                  <td><span class="type-badge {s.type}">{s.type}</span></td>
                  <td class="muted">{s.source}</td>
                  <td>{#if s.path}<button class="link-btn" onclick={() => openFile(s.path)}>View</button>{/if}</td>
                </tr>
              {/each}
            </tbody>
          </table>
        </div>
      {/if}

    <!-- MEMORIES -->
    {:else if tab === "memories"}
      {#if memoryCount === 0}
        <div class="empty">No memory files detected. Claude Code creates these automatically in <code>~/.claude/projects/*/memory/</code>.</div>
      {:else}
        {#each Object.entries(memoriesByProject) as [project, mems]}
          <div class="mem-group">
            <div class="mem-project">{project}</div>
            <div class="mem-list">
              {#each mems as m}
                <button class="mem-row" onclick={() => openFile(m.path)}>
                  <span class="mem-type-dot" style="background:{memoryTypeColors[m.memoryType] || 'var(--text-secondary)'}"></span>
                  <span class="mem-name">{m.name}</span>
                  {#if m.memoryType}
                    <span class="chip" style="--c:{memoryTypeColors[m.memoryType] || 'var(--text-secondary)'}">{m.memoryType}</span>
                  {/if}
                  <span class="mem-desc">{m.description || ""}</span>
                  <span class="mem-size">{formatBytes(m.sizeBytes)}</span>
                  <span class="mem-view">View</span>
                </button>
              {/each}
            </div>
          </div>
        {/each}
      {/if}
    {/if}
  </div>

  <!-- Audit trail footer -->
  <div class="audit-footer">
    <button class="audit-toggle" onclick={() => auditOpen = !auditOpen}>
      <span class="audit-arrow">{auditOpen ? "▾" : "▸"}</span>
      Scan Audit Trail
      <span class="audit-summary">{inventory.scanLog?.length ?? 0} paths checked, {inventory.scanLog?.filter(e => e.found).length ?? 0} found</span>
    </button>
    {#if auditOpen}
      {#if inventory.projectPaths?.length}
        <div class="audit-block">
          <div class="audit-label">Project paths ({inventory.projectPaths.length})</div>
          <div class="audit-paths">
            {#each inventory.projectPaths as p}
              <div class="audit-path"><code>{homePath(p)}</code></div>
            {/each}
          </div>
        </div>
      {/if}
      {#if inventory.scanLog?.length}
        <div class="audit-block">
          <div class="audit-label">Files checked</div>
          <div class="audit-filters">
            <button class="audit-pill" class:active={!auditFilter} onclick={() => auditFilter = ""}>All ({inventory.scanLog.length})</button>
            <button class="audit-pill" class:active={auditFilter === "found"} onclick={() => auditFilter = "found"}>Found ({inventory.scanLog.filter(e => e.found).length})</button>
            <button class="audit-pill" class:active={auditFilter === "missed"} onclick={() => auditFilter = "missed"}>Not found ({inventory.scanLog.filter(e => !e.found).length})</button>
          </div>
          <div class="audit-log">
            {#each filteredScanLog as entry}
              <div class="audit-entry" class:found={entry.found}>
                <span class="audit-mark">{entry.found ? "Y" : "."}</span>
                <span class="audit-type">{entry.type}</span>
                <code class="audit-file">{homePath(entry.path)}</code>
              </div>
            {/each}
          </div>
        </div>
      {/if}
    {/if}
  </div>

  {/if}
</div>

<!-- File Viewer Modal -->
{#if fileViewer.open}
<div class="fv-overlay" onclick={closeFile} role="dialog" aria-modal="true" tabindex="-1" onkeydown={(e) => { if (e.key === 'Escape') { closeFile(); } }}>
  <div class="fv-modal" onclick={(e) => e.stopPropagation()} role="presentation">
    <div class="fv-header">
      <div class="fv-title">
        <span class="fv-icon">&#9776;</span>
        <code>{homePath(fileViewer.path)}</code>
      </div>
      <button class="fv-close" onclick={closeFile}>&times;</button>
    </div>
    <div class="fv-body">
      {#if fileViewer.loading}
        <div class="fv-status">Loading...</div>
      {:else if fileViewer.error}
        <div class="fv-status error">{fileViewer.error}</div>
      {:else}
        <pre class="fv-content">{fileViewer.content}</pre>
      {/if}
    </div>
  </div>
</div>
{/if}

<style>
  /* Layout */
  .inventory { max-width: 920px; margin: 1.5rem auto; padding: 0 1rem; }
  .inv-header { display: flex; align-items: flex-start; justify-content: space-between; margin-bottom: 1.2rem; }
  .inv-header h2 { margin: 0 0 .2rem; }
  .inv-subtitle { font-size: .82rem; color: var(--text-secondary); margin: 0; }
  .inv-header-right { display: flex; align-items: center; gap: .7rem; }
  .scan-time { font-size: .72rem; color: var(--text-tertiary, var(--text-secondary)); font-variant-numeric: tabular-nums; }
  .inv-error { color: var(--danger, #ef4444); padding: 1rem; background: var(--surface); border-radius: 8px; }
  .inv-loading { text-align: center; padding: 3rem; color: var(--text-secondary); }

  /* Sensitive files banner */
  .sensitive-banner {
    margin-bottom: 1rem; border: 1px solid var(--danger, #ef4444); border-radius: 8px;
    background: var(--danger-dim, rgba(248,113,113,.08));
  }
  .sensitive-header {
    display: flex; align-items: center; gap: .5rem; padding: .55rem .8rem;
    font-size: .82rem; cursor: pointer; list-style: none; flex-wrap: wrap;
  }
  .sensitive-header::-webkit-details-marker { display: none; }
  .sensitive-icon {
    width: 1.2rem; height: 1.2rem; border-radius: 50%;
    background: var(--danger, #ef4444); color: white;
    display: flex; align-items: center; justify-content: center;
    font-size: .7rem; font-weight: 700; flex-shrink: 0;
  }
  .sensitive-hint { margin-left: auto; font-size: .72rem; color: var(--text-secondary); }
  .sensitive-hint code { font-size: .7rem; }
  .sensitive-list { padding: 0 .8rem .6rem; }
  .sensitive-row {
    display: flex; align-items: baseline; gap: .6rem; padding: .25rem 0;
    font-size: .78rem; border-bottom: 1px solid var(--border-light, var(--border));
  }
  .sensitive-row:last-child { border-bottom: none; }
  .sensitive-name { font-size: .76rem; min-width: 10rem; color: var(--danger, #ef4444); font-weight: 500; }
  .sensitive-project { font-size: .72rem; color: var(--text-secondary); min-width: 8rem; }
  .sensitive-risk { font-size: .72rem; color: var(--text-secondary); flex: 1; }

  /* Summary strip */
  .summary-strip {
    display: flex; gap: .5rem; margin-bottom: 1.2rem; flex-wrap: wrap;
  }
  .summary-stat {
    flex: 1; min-width: 100px;
    background: var(--surface); border: 1px solid var(--border); border-radius: 8px;
    padding: .6rem .8rem; text-align: center;
  }
  .stat-value { display: block; font-size: 1.3rem; font-weight: 700; font-variant-numeric: tabular-nums; color: var(--primary); }
  .stat-label { font-size: .7rem; color: var(--text-secondary); text-transform: uppercase; letter-spacing: .03em; }

  /* Tabs */
  .inv-tabs {
    display: flex; gap: .15rem; border-bottom: 1px solid var(--border);
    margin-bottom: 1rem; overflow-x: auto;
  }
  .inv-tab {
    padding: .5rem .9rem; border: none; background: none; cursor: pointer;
    font-size: .82rem; font-weight: 500; color: var(--text-secondary);
    border-bottom: 2px solid transparent; transition: all .15s;
    font-family: inherit; white-space: nowrap;
    display: flex; align-items: center; gap: .35rem;
  }
  .inv-tab:hover { color: var(--text-primary, var(--text)); }
  .inv-tab.active { color: var(--primary); border-bottom-color: var(--primary); font-weight: 600; }
  .tab-count {
    font-size: .68rem; background: var(--surface); border-radius: 8px;
    padding: .05rem .4rem; font-variant-numeric: tabular-nums;
    color: var(--text-secondary);
  }
  .inv-tab.active .tab-count { background: var(--primary-glow, color-mix(in srgb, var(--primary) 12%, transparent)); color: var(--primary); }
  .tab-content { min-height: 200px; }
  .sub-heading { font-size: .92rem; margin: 1.5rem 0 .6rem; }

  /* Tool cards */
  .tool-grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(210px, 1fr)); gap: .5rem; }
  .tool-card {
    background: var(--surface); border: 1px solid var(--border); border-radius: 8px;
    padding: .6rem .75rem; cursor: pointer; transition: border-color .15s;
    text-align: left; font-family: inherit; font-size: inherit; color: inherit;
  }
  .tool-card:hover { border-color: var(--primary); }
  .tool-card.dimmed { opacity: .4; cursor: default; }
  .tool-card.dimmed:hover { border-color: var(--border); }
  .tool-top { display: flex; align-items: center; gap: .4rem; }
  .tool-dot { width: 8px; height: 8px; border-radius: 50%; flex-shrink: 0; }
  .tool-name { font-weight: 600; font-size: .84rem; flex: 1; }
  .tool-ver { font-size: .68rem; color: var(--text-secondary); font-family: monospace; }
  .tool-absent { font-size: .68rem; color: var(--text-secondary); }
  .tool-details {
    margin-top: .45rem; padding-top: .4rem; border-top: 1px solid var(--border);
    display: flex; flex-direction: column; gap: .15rem;
  }
  .tool-row { font-size: .74rem; color: var(--text-secondary); display: flex; gap: .35rem; align-items: baseline; }
  .tool-row code { font-size: .7rem; color: var(--text-primary, var(--text)); word-break: break-all; }
  .row-label { color: var(--text-tertiary, var(--text-secondary)); min-width: 3rem; flex-shrink: 0; }

  /* Tables */
  .table-wrap { overflow-x: auto; }
  .inv-table { width: 100%; border-collapse: collapse; font-size: .82rem; }
  .inv-table th { text-align: left; color: var(--text-secondary); font-weight: 500; padding: .4rem .6rem; border-bottom: 1px solid var(--border); }
  .inv-table td { padding: .45rem .6rem; border-bottom: 1px solid var(--border-light, var(--border)); }
  .inv-table code { font-size: .78rem; }
  .num { text-align: right; font-variant-numeric: tabular-nums; }
  .muted { color: var(--text-secondary); font-size: .78rem; }

  /* Chip (provider + source) */
  .chip {
    display: inline-block; font-size: .7rem; padding: .1rem .5rem; border-radius: 10px;
    background: color-mix(in srgb, var(--c) 14%, transparent);
    color: var(--c); font-weight: 500; white-space: nowrap;
  }

  /* MCP cards */
  .mcp-list { display: flex; flex-direction: column; gap: .5rem; }
  .mcp-card {
    background: var(--surface); border: 1px solid var(--border); border-radius: 8px;
    padding: .65rem .85rem;
  }
  .mcp-top { display: flex; align-items: center; gap: .5rem; margin-bottom: .25rem; }
  .mcp-name { font-weight: 600; font-size: .85rem; flex: 1; }
  .mcp-meta { display: flex; align-items: center; gap: .7rem; flex-wrap: wrap; }
  .mcp-cmd { font-size: .72rem; color: var(--text-secondary); word-break: break-all; }
  .mcp-scope { font-size: .72rem; color: var(--text-tertiary, var(--text-secondary)); }

  .link-btn {
    margin-top: .35rem; font-size: .7rem; color: var(--primary);
    background: none; border: none; cursor: pointer; padding: 0;
    font-family: inherit; text-decoration: underline;
    text-decoration-style: dotted; text-underline-offset: 2px;
  }
  .link-btn:hover { text-decoration-style: solid; }

  /* Instruction files */
  .instr-group { margin-bottom: .9rem; }
  .instr-project { font-size: .82rem; font-weight: 600; margin-bottom: .3rem; padding-left: .2rem; }
  .instr-list { display: flex; flex-direction: column; gap: .2rem; }
  .instr-row {
    display: flex; align-items: center; gap: .55rem; font-size: .8rem;
    padding: .35rem .5rem; border-radius: 6px;
    background: none; border: 1px solid transparent; cursor: pointer;
    text-align: left; font-family: inherit; color: inherit; width: 100%;
    transition: background .12s, border-color .12s;
  }
  .instr-row:hover { background: var(--surface); border-color: var(--border); }
  .instr-icon {
    width: 1.4rem; height: 1.4rem; border-radius: 4px;
    display: flex; align-items: center; justify-content: center;
    font-size: .58rem; font-weight: 700; color: white; flex-shrink: 0;
  }
  .instr-name { font-weight: 500; flex: 1; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .instr-type { font-size: .7rem; color: var(--text-secondary); min-width: 5.5rem; }
  .instr-size { font-size: .72rem; font-variant-numeric: tabular-nums; color: var(--text-secondary); min-width: 3.5rem; text-align: right; }
  .instr-time { font-size: .72rem; color: var(--text-tertiary, var(--text-secondary)); min-width: 4rem; }
  .instr-view { font-size: .7rem; color: var(--primary); }

  /* Skills */
  .type-badge {
    font-size: .7rem; padding: .1rem .5rem; border-radius: 10px;
    background: var(--surface); color: var(--text-secondary);
  }
  .type-badge.command { color: var(--primary); background: color-mix(in srgb, var(--primary) 10%, transparent); }
  .type-badge.plugin { color: #d97706; background: color-mix(in srgb, #d97706 10%, transparent); }

  /* Memory files */
  .mem-group { margin-bottom: .9rem; }
  .mem-project { font-size: .82rem; font-weight: 600; margin-bottom: .3rem; padding-left: .2rem; }
  .mem-list { display: flex; flex-direction: column; gap: .2rem; }
  .mem-row {
    display: flex; align-items: center; gap: .55rem; font-size: .8rem;
    padding: .4rem .5rem; border-radius: 6px;
    background: none; border: 1px solid transparent; cursor: pointer;
    text-align: left; font-family: inherit; color: inherit; width: 100%;
    transition: background .12s, border-color .12s;
  }
  .mem-row:hover { background: var(--surface); border-color: var(--border); }
  .mem-type-dot { width: 7px; height: 7px; border-radius: 50%; flex-shrink: 0; }
  .mem-name { font-weight: 500; min-width: 8rem; max-width: 12rem; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .mem-desc { flex: 1; font-size: .74rem; color: var(--text-secondary); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .mem-size { font-size: .72rem; font-variant-numeric: tabular-nums; color: var(--text-secondary); min-width: 3.5rem; text-align: right; }
  .mem-view { font-size: .7rem; color: var(--primary); }
  .type-badge.skill { color: #06b6d4; background: color-mix(in srgb, #06b6d4 10%, transparent); }

  /* IDE Extensions */
  .ext-group { margin-bottom: 1rem; }
  .ext-ide {
    display: flex; align-items: center; gap: .4rem;
    font-size: .82rem; font-weight: 600; margin-bottom: .35rem; padding-left: .2rem;
  }
  .ext-ide-dot { width: 8px; height: 8px; border-radius: 50%; flex-shrink: 0; }
  .ext-ide-count {
    font-size: .68rem; font-weight: 500; color: var(--text-secondary);
    background: var(--surface); border-radius: 8px; padding: .05rem .4rem;
    margin-left: .2rem;
  }
  .ext-list { display: flex; flex-direction: column; gap: .15rem; }
  .ext-row {
    display: flex; align-items: center; gap: .55rem; font-size: .8rem;
    padding: .35rem .5rem .1rem; border-radius: 6px;
  }
  .ext-row:hover { background: var(--surface); }
  .ext-name { font-weight: 500; min-width: 10rem; }
  .ext-id { font-size: .7rem; color: var(--text-secondary); flex: 1; }
  .ext-ver { font-size: .72rem; font-variant-numeric: tabular-nums; color: var(--text-secondary); min-width: 4rem; text-align: right; }
  .ext-desc {
    font-size: .72rem; color: var(--text-tertiary, var(--text-secondary));
    padding: 0 .5rem .35rem; margin-top: -.05rem;
  }

  .empty { text-align: center; padding: 2.5rem 1rem; color: var(--text-secondary); font-size: .85rem; }
  .empty code { font-size: .78rem; }

  /* Audit footer */
  .audit-footer {
    border-top: 1px solid var(--border); margin-top: 1.5rem; padding-top: .3rem;
  }
  .audit-toggle {
    display: flex; align-items: center; gap: .4rem; width: 100%;
    background: none; border: none; cursor: pointer;
    padding: .5rem 0; color: var(--text-secondary); font-size: .8rem;
    font-family: inherit; text-align: left;
  }
  .audit-toggle:hover { color: var(--text-primary, var(--text)); }
  .audit-arrow { font-size: .75rem; width: .8rem; }
  .audit-summary { margin-left: auto; font-size: .72rem; color: var(--text-tertiary, var(--text-secondary)); }
  .audit-block { margin-bottom: .8rem; }
  .audit-label { font-size: .76rem; font-weight: 600; color: var(--text-secondary); margin-bottom: .3rem; margin-top: .3rem; }
  .audit-paths { max-height: 180px; overflow-y: auto; }
  .audit-path { font-size: .73rem; padding: .12rem .4rem; }
  .audit-path code { color: var(--text-secondary); font-size: .7rem; }
  .audit-filters { display: flex; gap: .3rem; margin-bottom: .4rem; }
  .audit-pill {
    font-size: .7rem; padding: .18rem .55rem; border-radius: 10px;
    border: 1px solid var(--border); background: var(--bg); cursor: pointer;
    color: var(--text-secondary); font-family: inherit;
  }
  .audit-pill.active { background: var(--primary); color: white; border-color: var(--primary); }
  .audit-log { max-height: 350px; overflow-y: auto; font-size: .73rem; }
  .audit-entry { display: flex; align-items: center; gap: .4rem; padding: .12rem .4rem; }
  .audit-entry.found { color: var(--success, #22c55e); }
  .audit-entry:not(.found) { color: var(--text-tertiary, var(--text-secondary)); }
  .audit-mark { font-family: monospace; width: .8rem; text-align: center; }
  .audit-type { min-width: 7rem; color: var(--text-secondary); }
  .audit-file { font-size: .68rem; word-break: break-all; }

  /* File viewer modal */
  .fv-overlay {
    position: fixed; inset: 0; z-index: 1000;
    background: rgba(0, 0, 0, .55); backdrop-filter: blur(4px);
    display: flex; align-items: center; justify-content: center;
    padding: 2rem;
  }
  .fv-modal {
    background: var(--bg); border: 1px solid var(--border); border-radius: 10px;
    width: 100%; max-width: 780px; max-height: 80vh;
    display: flex; flex-direction: column;
    box-shadow: 0 20px 60px rgba(0,0,0,.4);
  }
  .fv-header {
    display: flex; align-items: center; justify-content: space-between;
    padding: .65rem 1rem; border-bottom: 1px solid var(--border); flex-shrink: 0;
  }
  .fv-title { display: flex; align-items: center; gap: .5rem; min-width: 0; }
  .fv-title code { font-size: .78rem; color: var(--text-secondary); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .fv-icon { color: var(--text-tertiary, var(--text-secondary)); font-size: .9rem; }
  .fv-close {
    background: none; border: none; color: var(--text-secondary); font-size: 1.3rem;
    cursor: pointer; padding: 0 .3rem; line-height: 1;
  }
  .fv-close:hover { color: var(--text-primary, var(--text)); }
  .fv-body { overflow-y: auto; flex: 1; }
  .fv-status { padding: 2rem; text-align: center; color: var(--text-secondary); }
  .fv-status.error { color: var(--danger, #ef4444); }
  .fv-content {
    margin: 0; padding: 1rem; font-size: .78rem; line-height: 1.55;
    white-space: pre-wrap; word-break: break-word;
    color: var(--text-primary, var(--text)); background: var(--surface);
  }
</style>
