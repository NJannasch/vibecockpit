<script>
  import { onMount, onDestroy } from "svelte";
  import "./app.css";
  import {
    sessions,
    config,
    activeFilters,
    currentView,
    searchQuery,
    sortBy,
    groupBy,
    filteredSessions,
    loadSessions,
    loadConfig,
    startAutoRefresh,
    stopAutoRefresh,
  } from "./lib/stores.js";
  import { saveConfig, launchSession, deleteSession, createProject as apiCreateProject, testSSH } from "./lib/api.js";
  import { shortModel, relativeTime, providerColors, providerLabels, getProviderType, dateGroup } from "./lib/utils.js";
  import Dashboard from "./components/Dashboard.svelte";

  // ─── Reactive State (Svelte 5 runes) ───

  let page = $state("dashboard"); // "dashboard" | "sessions" | "settings"
  let openModal = $state(null); // "newProject" | "resume" | "delete" | null
  let toasts = $state([]);
  let newProjectDir = $state("");
  let newProjectTool = $state("");
  let newProjectModel = $state("");
  let lastScanned = $state(null);
  let scanning = $state(false);
  let pendingDelete = $state(null);
  let pendingResume = $state(null);
  let resumeModel = $state("");
  let collapsedGroups = $state(new Set());
  let collapsedTreeNodes = $state(new Set());

  // Store subscriptions for local reactivity
  let sessionList = $state([]);
  let configData = $state({});
  let filterData = $state({ filtered: [], activeCount: 0, fuzzyTerms: [] });
  let currentViewVal = $state("list");
  let searchVal = $state("");
  let sortByVal = $state("modified");
  let groupByVal = $state("none");
  let activeFiltersVal = $state({});

  const unsubSessions = sessions.subscribe((v) => (sessionList = v));
  const unsubConfig = config.subscribe((v) => (configData = v));
  const unsubFiltered = filteredSessions.subscribe((v) => (filterData = v));
  const unsubView = currentView.subscribe((v) => (currentViewVal = v));
  const unsubSearch = searchQuery.subscribe((v) => (searchVal = v));
  const unsubSort = sortBy.subscribe((v) => (sortByVal = v));
  const unsubGroup = groupBy.subscribe((v) => (groupByVal = v));
  const unsubActiveFilters = activeFilters.subscribe((v) => (activeFiltersVal = v));

  onDestroy(() => {
    unsubSessions();
    unsubConfig();
    unsubFiltered();
    unsubView();
    unsubSearch();
    unsubSort();
    unsubGroup();
    unsubActiveFilters();
    stopAutoRefresh();
  });

  // ─── Theme ───

  function applyTheme(theme) {
    document.documentElement.setAttribute("data-theme", theme);
  }

  let themeIcon = $derived(configData.theme === "dark" ? "☽" : "☆");

  function toggleTheme() {
    const next = configData.theme === "dark" ? "light" : "dark";
    const updated = { ...configData, theme: next };
    config.set(updated);
    applyTheme(next);
    saveConfig(updated);
  }

  // ─── Init + Refresh ───

  async function refresh(force = true) {
    scanning = true;
    await loadSessions(force);
    lastScanned = new Date();
    scanning = false;
  }

  onMount(async () => {
    await loadConfig();
    applyTheme(configData.theme === "dark" ? "dark" : "light");
    await refresh(false);
    startAutoRefresh();
  });

  // ─── Keyboard Shortcuts ───

  function handleKeydown(e) {
    if (e.key === "/" && !["INPUT", "SELECT", "TEXTAREA"].includes(document.activeElement?.tagName)) {
      e.preventDefault();
      document.querySelector(".search-box input")?.focus();
    }
    if (e.key === "Escape") {
      if (openModal) {
        openModal = null;
      } else {
        const input = document.querySelector(".search-box input");
        if (document.activeElement === input) {
          searchQuery.set("");
          input.blur();
        }
      }
    }
  }

  // ─── Navigation ───

  function navigateTo(p) {
    page = p;
  }

  function filterByProvider(providerName) {
    activeFilters.set({ provider: providerName });
    page = "sessions";
  }

  // ─── Search ───

  function onSearchInput(e) {
    searchQuery.set(e.target.value);
  }

  // ─── View Toggle ───

  function setView(view) {
    currentView.set(view);
  }

  // ─── Sort / Group ───

  function onSortChange(e) {
    const val = e.target.value;
    sortBy.set(val);
    const updated = { ...configData, sortBy: val };
    config.set(updated);
    saveConfig(updated);
  }

  function onGroupChange(e) {
    const val = e.target.value;
    groupBy.set(val);
    const updated = { ...configData, groupBy: val };
    config.set(updated);
    saveConfig(updated);
  }

  // ─── Filter Chips ───

  let chipData = $derived.by(() => {
    const providers = [...new Set(sessionList.map((s) => s.provider).filter(Boolean))];
    const models = [
      ...new Set(
        sessionList
          .map((s) => {
            const m = (s.model || "").replace(/^claude-/, "").split("-")[0].split("/").pop();
            return m || null;
          })
          .filter(Boolean),
      ),
    ];
    const activeCount = sessionList.filter((s) => s.isActive).length;
    return { providers, models, activeCount };
  });

  function toggleChip(key, val) {
    let filters = { ...activeFiltersVal };
    if (key === "all") {
      filters = {};
    } else if (key === "active") {
      if (filters.active) {
        delete filters.active;
      } else {
        filters.active = true;
      }
    } else {
      // For provider/model: toggle off if same value, otherwise set
      if (filters[key] === val) {
        delete filters[key];
      } else {
        filters[key] = val;
      }
    }
    activeFilters.set(filters);
  }

  let isAllActive = $derived(Object.keys(activeFiltersVal).length === 0);

  // ─── Launch / Resume ───

  function launch(sessionId, provider, e) {
    if (e?.shiftKey) {
      showModelPicker(sessionId, provider);
      return;
    }
    doLaunch(sessionId, provider);
  }

  async function doLaunch(sessionId, provider, modelOverride) {
    try {
      const data = await launchSession(sessionId, provider, modelOverride);
      if (data.error) {
        if (data.error.includes("PATH")) {
          showToast(data.error, "error");
          openModal = "settings";
        } else {
          showToast(data.error, "error");
        }
      } else {
        showToast("Session launched" + (modelOverride ? " with " + modelOverride : ""), "success");
      }
    } catch (err) {
      showToast("Failed: " + err.message, "error");
    }
  }

  function showModelPicker(sessionId, provider) {
    const sess = sessionList.find((s) => s.id === sessionId);
    const models = configData.models || [];
    const current = sess ? sess.model : "";
    pendingResume = { sessionId, provider, current, models };
    resumeModel = current || (models.length > 0 ? models[0] : "");
    openModal = "resume";
  }

  function confirmResume() {
    if (!pendingResume) return;
    const modelOverride = resumeModel !== pendingResume.current ? resumeModel : undefined;
    doLaunch(pendingResume.sessionId, pendingResume.provider, modelOverride);
    openModal = null;
    pendingResume = null;
  }

  // ─── Delete ───

  function confirmDelete(sessionId, provider, projectName) {
    pendingDelete = { sessionId, provider, projectName };
    openModal = "delete";
  }

  async function doDelete() {
    if (!pendingDelete) return;
    try {
      const data = await deleteSession(pendingDelete.sessionId, pendingDelete.provider);
      if (data.error) {
        showToast(data.error, "error");
      } else {
        showToast("Session deleted", "success");
        await loadSessions();
      }
    } catch (err) {
      showToast("Failed: " + err.message, "error");
    }
    openModal = null;
    pendingDelete = null;
  }

  // ─── New Project ───

  let availableTools = $derived.by(() => {
    const tools = [...new Set(sessionList.map(s => s.provider))].filter(p => !p.startsWith("remote-"));
    if (tools.length === 0) tools.push("claude");
    return tools;
  });

  function showNewProject() {
    newProjectDir = configData.newProjectDir ? configData.newProjectDir + "/" : "";
    newProjectTool = availableTools[0] || "claude";
    newProjectModel = "";
    openModal = "newProject";
  }

  async function doCreateProject() {
    const dir = newProjectDir.trim();
    if (!dir) return;
    try {
      const data = await apiCreateProject(dir, newProjectTool, newProjectModel || undefined);
      if (data.error) {
        showToast(data.error, "error");
      } else {
        showToast(`Project created, launching ${newProjectTool}`, "success");
        openModal = null;
      }
    } catch (err) {
      showToast("Failed: " + err.message, "error");
    }
  }

  // ─── SSH Test ───

  async function doTestSSH(src) {
    showToast(`Testing SSH to ${src.user}@${src.host}...`, "");
    try {
      const data = await testSSH(src.host, src.user, src.port || 22);
      if (data.ok) {
        showToast(`SSH to ${src.host}: connected`, "success");
      } else {
        showToast(`SSH to ${src.host} failed: ${data.error}`, "error");
      }
    } catch (e) {
      showToast(`SSH test failed: ${e.message}`, "error");
    }
  }

  // ─── Remote Sources ───

  function addRemoteSource() {
    const sources = [...(configData.remoteSources || []), { name: "", host: "", user: "", method: "ssh" }];
    const u = { ...configData, remoteSources: sources };
    config.set(u);
    saveConfig(u);
  }

  function removeRemoteSource(index) {
    const sources = [...(configData.remoteSources || [])];
    sources.splice(index, 1);
    const u = { ...configData, remoteSources: sources };
    config.set(u);
    saveConfig(u);
  }

  function updateRemoteSource(index, field, value) {
    const sources = [...(configData.remoteSources || [])];
    sources[index] = { ...sources[index], [field]: value };
    const u = { ...configData, remoteSources: sources };
    config.set(u);
    saveConfig(u);
  }

  // ─── Settings ───

  let settingsTerminal = $state("");
  let settingsNewDir = $state("");
  let settingsProviderPaths = $state({});

  function showSettings() {
    settingsTerminal = configData.terminal || "default";
    settingsNewDir = configData.newProjectDir || "";
    settingsProviderPaths = { ...(configData.providerPaths || {}) };
    openModal = "settings";
  }

  async function doSaveSettings() {
    const paths = {};
    for (const [k, v] of Object.entries(settingsProviderPaths)) {
      const trimmed = v.trim();
      if (trimmed) paths[k] = trimmed;
    }
    const updated = {
      ...configData,
      terminal: settingsTerminal,
      newProjectDir: settingsNewDir,
      providerPaths: paths,
    };
    try {
      await saveConfig(updated);
      config.set(updated);
      showToast("Settings saved", "success");
      openModal = null;
    } catch (err) {
      showToast("Failed to save: " + err.message, "error");
    }
  }

  let settingsProviders = $derived([...new Set(sessionList.map((s) => s.provider).filter(Boolean))]);
  let availableTerminals = $derived(configData.availableTerminals || ["default"]);

  // ─── Toast ───

  let toastId = 0;

  function showToast(msg, type) {
    const id = ++toastId;
    toasts = [...toasts, { id, msg, type }];
    setTimeout(() => {
      toasts = toasts.filter((t) => t.id !== id);
    }, 3000);
  }

  // ─── Grouping ───

  function getGroupKey(s) {
    switch (groupByVal) {
      case "provider":
        return s.provider || "unknown";
      case "project":
        return s.projectName || "unknown";
      case "date":
        return dateGroup(s.modified);
      default:
        return "all";
    }
  }

  let groupedSessions = $derived.by(() => {
    if (groupByVal === "none") return null;
    const groups = [];
    const seen = {};
    for (const s of filterData.filtered) {
      const key = getGroupKey(s);
      if (!seen[key]) {
        seen[key] = [];
        groups.push(key);
      }
      seen[key].push(s);
    }
    return groups.map((key) => ({ key, items: seen[key] }));
  });

  function toggleGroup(key) {
    const next = new Set(collapsedGroups);
    if (next.has(key)) next.delete(key);
    else next.add(key);
    collapsedGroups = next;
  }

  // ─── Tree View ───

  function buildPathTree(sessionsList) {
    const root = { children: {}, sessions: [] };
    for (const s of sessionsList) {
      let path = s.projectPath || "";
      if (!path) {
        if (!root.children["unlinked"]) root.children["unlinked"] = { children: {}, sessions: [] };
        root.children["unlinked"].sessions.push(s);
        continue;
      }
      path = path.replace(/^\/home\/[^/]+/, "~");
      const parts = path.split("/").filter(Boolean);
      let node = root;
      for (const part of parts) {
        if (!node.children[part]) node.children[part] = { children: {}, sessions: [] };
        node = node.children[part];
      }
      node.sessions.push(s);
    }
    return collapseTree(root);
  }

  function collapseTree(node) {
    for (const [k, child] of Object.entries(node.children)) {
      node.children[k] = collapseTree(child);
    }
    const keys = Object.keys(node.children);
    if (keys.length === 1 && node.sessions.length === 0) {
      const key = keys[0];
      const child = node.children[key];
      const merged = { children: child.children, sessions: child.sessions };
      delete node.children[key];
      for (const [ck, cv] of Object.entries(merged.children)) {
        node.children[key + "/" + ck] = cv;
      }
      node.sessions = merged.sessions;
      return collapseTree(node);
    }
    return node;
  }

  function collectSessions(node) {
    let all = [...node.sessions];
    for (const child of Object.values(node.children)) {
      all = all.concat(collectSessions(child));
    }
    return all;
  }

  let treeData = $derived(currentViewVal === "tree" ? buildPathTree(filterData.filtered) : null);

  function toggleTreeNode(name) {
    const next = new Set(collapsedTreeNodes);
    if (next.has(name)) next.delete(name);
    else next.add(name);
    collapsedTreeNodes = next;
  }

  // ─── Highlight Matches ───

  function highlight(text, terms) {
    if (!terms || !terms.length || !text) return text;
    for (const term of terms) {
      const lower = text.toLowerCase();
      const idx = lower.indexOf(term);
      if (idx >= 0) {
        return (
          text.slice(0, idx) +
          "\x00HL_START\x00" +
          text.slice(idx, idx + term.length) +
          "\x00HL_END\x00" +
          text.slice(idx + term.length)
        );
      }
    }
    return text;
  }

  // Converts highlight markers to HTML
  function highlightHtml(text, terms) {
    return highlight(text, terms)
      .replace(/\x00HL_START\x00/g, '<span class="match-hl">')
      .replace(/\x00HL_END\x00/g, "</span>");
  }

  // ─── Overlay Click ───

  function onOverlayClick(e) {
    if (e.target === e.currentTarget) {
      openModal = null;
    }
  }

  // ─── Home path shortening ───

  function homePath(path) {
    return path ? path.replace(/^\/home\/[^/]+/, "~") : "";
  }

  // ─── Resume model list ───

  let resumeModelList = $derived.by(() => {
    if (!pendingResume) return [];
    const current = pendingResume.current;
    const models = pendingResume.models || [];
    return current ? [current, ...models.filter((m) => m !== current)] : models;
  });
</script>

<svelte:window onkeydown={handleKeydown} />

<!-- Header -->
<header>
  <div class="logo">
    <span class="logo-icon">&#9670;</span>
    <span>VibeCockpit</span>
  </div>
  <nav class="header-nav">
    <button class="nav-btn" class:active={page === "dashboard"} onclick={() => navigateTo("dashboard")}>Dashboard</button>
    <button class="nav-btn" class:active={page === "sessions"} onclick={() => navigateTo("sessions")}>Sessions</button>
    <button class="nav-btn" class:active={page === "settings"} onclick={() => navigateTo("settings")}>Settings</button>
  </nav>
  <div class="header-actions">
    <button class="btn btn-sm" onclick={refresh} disabled={scanning} title={lastScanned ? "Last scan: " + relativeTime(lastScanned.toISOString()) : "Scanning..."}>
      {scanning ? "Scanning..." : "Refresh"}
    </button>
    <button class="btn btn-primary btn-sm" onclick={showNewProject}>+ New</button>
    <button class="btn btn-icon btn-ghost" onclick={toggleTheme} title="Toggle theme">{themeIcon}</button>
    <a href="https://github.com/njannasch/vibecockpit" target="_blank" class="btn btn-icon btn-ghost" title="GitHub">&#9733;</a>
  </div>
</header>

{#if page === "dashboard"}
  <main>
    <Dashboard
      sessions={sessionList}
      onnavigate={navigateTo}
      onlaunch={(id, prov) => launch(id, prov)}
      onfilterby={filterByProvider}
    />
  </main>
{:else if page === "sessions"}
<!-- Search Bar -->
<div class="search-wrap">
  <div class="search-box">
    <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round">
      <circle cx="11" cy="11" r="8" />
      <line x1="21" y1="21" x2="16.65" y2="16.65" />
    </svg>
    <input type="text" placeholder="Search sessions...  ( / )" autocomplete="off" value={searchVal} oninput={onSearchInput} />
  </div>
  <div class="search-right">
    <div class="view-toggle">
      <button class="view-btn" class:active={currentViewVal === "list"} onclick={() => setView("list")}>List</button>
      <button class="view-btn" class:active={currentViewVal === "tree"} onclick={() => setView("tree")}>Tree</button>
    </div>
    <select class="sort-select" value={groupByVal} onchange={onGroupChange}>
      <option value="none">No grouping</option>
      <option value="provider">Group by tool</option>
      <option value="project">Group by project</option>
      <option value="date">Group by date</option>
    </select>
    <select class="sort-select" value={sortByVal} onchange={onSortChange}>
      <option value="modified">Recently modified</option>
      <option value="created">Recently created</option>
      <option value="name">Project name A-Z</option>
      <option value="messages">Most messages</option>
    </select>
    <div class="stats">
      {#if filterData.filtered.length > 0}
        <b>{filterData.filtered.length}</b> sessions
        {#if filterData.activeCount > 0}
          &middot; <b style="color:var(--success)">{filterData.activeCount}</b> active
        {/if}
      {/if}
    </div>
  </div>
</div>

<!-- Filter Bar -->
<div class="filter-bar">
  <div class="filter-chips">
    <button class="chip chip-all" class:on={isAllActive} onclick={() => toggleChip("all")}>All</button>
    {#if chipData.activeCount > 0}
      <button class="chip chip-active" class:on={activeFiltersVal.active === true} onclick={() => toggleChip("active")}>
        Active <span style="opacity:.6">({chipData.activeCount})</span>
      </button>
    {/if}
    {#if chipData.providers.length > 1}
      <div class="chip-sep"></div>
      {#each chipData.providers as p}
        {@const cnt = sessionList.filter((s) => s.provider === p).length}
        <button
          class="chip chip-provider"
          class:on={activeFiltersVal.provider === p}
          onclick={() => toggleChip("provider", p)}
        >
          {p} <span style="opacity:.6">({cnt})</span>
        </button>
      {/each}
    {/if}
    {#if chipData.models.length > 1}
      <div class="chip-sep"></div>
      {#each chipData.models as m}
        <button class="chip chip-model" class:on={activeFiltersVal.model === m} onclick={() => toggleChip("model", m)}>
          {m}
        </button>
      {/each}
    {/if}
  </div>
  <div class="filter-hint">Tip: type <b>model:</b>opus or <b>branch:</b>main in search</div>
</div>

<!-- Main Content -->
<main>
  {#if sessionList.length === 0}
    <!-- Onboarding -->
    <div class="onboarding">
      <div class="onboarding-icon">&#9670;</div>
      <h2>Welcome to VibeCockpit</h2>
      <p>Your AI coding session dashboard. Browse, search, and resume all your coding sessions in one place.</p>
      <div class="steps">
        <div class="step">
          <span class="step-num">1</span>
          <div>
            <h4>Install Claude Code</h4>
            <p><code>npm install -g @anthropic-ai/claude-code</code></p>
          </div>
        </div>
        <div class="step">
          <span class="step-num">2</span>
          <div>
            <h4>Start coding</h4>
            <p>Run <code>claude</code> in any project directory</p>
          </div>
        </div>
        <div class="step">
          <span class="step-num">3</span>
          <div>
            <h4>Come back here</h4>
            <p>Your sessions will appear automatically</p>
          </div>
        </div>
      </div>
      <button class="btn btn-primary" onclick={showNewProject}>+ Create Your First Project</button>
    </div>
  {:else if filterData.filtered.length === 0}
    <div class="empty">
      <h3>No matching sessions</h3>
      <p>Try a different search or clear filters</p>
    </div>
  {:else if currentViewVal === "tree" && treeData}
    <!-- Tree View -->
    <div class="tree">
      {#snippet treeNodes(node, prefix)}
        {#each Object.keys(node.children).sort() as name}
          {@const child = node.children[name]}
          {@const allSessions = collectSessions(child)}
          {@const providers = [...new Set(allSessions.map((s) => s.provider))]}
          {@const nodeId = prefix + "/" + name}
          {@const isCollapsed = collapsedTreeNodes.has(nodeId)}
          {@const isUnlinked = name === "unlinked"}
          <div class="tree-node" class:collapsed={isCollapsed}>
            <div class="tree-dir" onclick={() => toggleTreeNode(nodeId)}>
              <span class="tree-chevron">&#9660;</span>
              <span class="tree-dir-icon">{isUnlinked ? "◇" : "▷"}</span>
              <span class="tree-dir-name">{isUnlinked ? "Unlinked sessions" : name}</span>
              <span class="tree-dir-count">{allSessions.length}</span>
              <span class="tree-dir-providers">
                {#each providers as p}
                  <span
                    class="badge"
                    style="background:{providerColors[p] || 'var(--primary)'}18;color:{providerColors[p] ||
                      'var(--primary)'};border-color:{providerColors[p] || 'var(--primary)'}30"
                  >
                    {p}
                  </span>
                {/each}
              </span>
            </div>
            <div class="tree-children">
              {@render treeNodes(child, nodeId)}
              {#each child.sessions.sort((a, b) => new Date(b.modified) - new Date(a.modified)) as s}
                {@const summary = s.summary || s.firstPrompt || "untitled"}
                {@const color = providerColors[s.provider] || "var(--text-secondary)"}
                <div class="tree-session" onclick={() => launch(s.id, s.provider)}>
                  <span class="tree-session-provider" style="background:{color}" title={s.provider}></span>
                  <span class="tree-session-summary">{summary.slice(0, 70)}</span>
                  <span class="tree-session-time">{relativeTime(s.modified)}</span>
                </div>
              {/each}
            </div>
          </div>
        {/each}
      {/snippet}
      {@render treeNodes(treeData, "")}
    </div>
  {:else if groupedSessions}
    <!-- Grouped List View -->
    {#each groupedSessions as { key, items }}
      <div class="group" class:collapsed={collapsedGroups.has(key)}>
        <div class="group-header" onclick={() => toggleGroup(key)}>
          <span class="group-chevron">&#9660;</span>
          <span class="group-name">
            {#if groupByVal === "provider"}{providerLabels[key] || key}{:else}{key}{/if}
          </span>
          <span class="group-count">{items.length}</span>
        </div>
        <div class="group-sessions sessions">
          {#each items as s}
            {@const summary = s.summary || s.firstPrompt || ""}
            {@const displaySummary = summary.length > 120 ? summary.slice(0, 117) + "..." : summary}
            {@const model = shortModel(s.model)}
            {@const hp = homePath(s.projectPath)}
            <div class={s.isActive ? "card active" : "card"} onclick={(e) => launch(s.id, s.provider, e)}>
              <div class="card-status"></div>
              <div class="card-main">
                <div class="card-project">
                  {#if filterData.fuzzyTerms?.length}
                    {@html highlightHtml(s.projectName, filterData.fuzzyTerms)}
                  {:else}
                    {s.projectName}
                  {/if}
                </div>
                {#if hp}<div class="card-path">{hp}</div>{/if}
                <div class="card-summary">{displaySummary}</div>
              </div>
              <div class="card-meta">
                {#if model !== "-"}<span class="badge badge-model">{model}</span>{/if}
                {#if s.gitBranch}<span>{s.gitBranch}</span>{/if}
                {#if s.messageCount}<span>{s.messageCount} msgs</span>{/if}
                {#if s.isActive}<span class="badge badge-active">active</span>{/if}
              </div>
              <div class="card-time">{relativeTime(s.modified)}</div>
              <div class="card-actions">
                <button class="btn btn-sm btn-primary" onclick={(e) => { e.stopPropagation(); launch(s.id, s.provider); }}>Resume</button>
                <button class="btn btn-sm btn-ghost" onclick={(e) => { e.stopPropagation(); showModelPicker(s.id, s.provider); }} title="Resume with different model">&#9881; Model</button>
                {#if !s.isActive}
                  <button
                    class="card-del"
                    onclick={(e) => { e.stopPropagation(); confirmDelete(s.id, s.provider, s.projectName); }}
                    title="Delete session"
                  >&#10005;</button>
                {/if}
              </div>
            </div>
          {/each}
        </div>
      </div>
    {/each}
  {:else}
    <!-- Flat List View -->
    <div class="sessions">
      {#each filterData.filtered as s}
        {@const summary = s.summary || s.firstPrompt || ""}
        {@const displaySummary = summary.length > 120 ? summary.slice(0, 117) + "..." : summary}
        {@const model = shortModel(s.model)}
        {@const hp = homePath(s.projectPath)}
        <div class={s.isActive ? "card active" : "card"} onclick={(e) => launch(s.id, s.provider, e)}>
          <div class="card-status"></div>
          <div class="card-main">
            <div class="card-project">
              {#if filterData.fuzzyTerms?.length}
                {@html highlightHtml(s.projectName, filterData.fuzzyTerms)}
              {:else}
                {s.projectName}
              {/if}
            </div>
            {#if hp}<div class="card-path">{hp}</div>{/if}
            <div class="card-summary">{displaySummary}</div>
          </div>
          <div class="card-meta">
            {#if model !== "-"}<span class="badge badge-model">{model}</span>{/if}
            {#if s.gitBranch}<span>{s.gitBranch}</span>{/if}
            {#if s.messageCount}<span>{s.messageCount} msgs</span>{/if}
            {#if s.isActive}<span class="badge badge-active">active</span>{/if}
          </div>
          <div class="card-time">{relativeTime(s.modified)}</div>
          <div class="card-actions">
            <button class="btn btn-sm btn-primary" onclick={(e) => { e.stopPropagation(); launch(s.id, s.provider); }}>Resume</button>
            <button class="btn btn-sm btn-ghost" onclick={(e) => { e.stopPropagation(); showModelPicker(s.id, s.provider); }} title="Resume with different model">&#9881; Model</button>
            {#if !s.isActive}
              <button
                class="card-del"
                onclick={(e) => { e.stopPropagation(); confirmDelete(s.id, s.provider, s.projectName); }}
                title="Delete session"
              >&#10005;</button>
            {/if}
          </div>
        </div>
      {/each}
    </div>
  {/if}
</main>
{:else if page === "settings"}
  <main>
    {@render settingsPage()}
  </main>
{/if}

{#snippet settingsPage()}
  <div style="max-width:640px;margin:2rem auto;padding:0 1rem">
    <h2 style="margin-bottom:1.5rem">Settings</h2>
    <div class="field">
      <label>Terminal emulator</label>
      <select value={configData.terminal || "default"} onchange={(e) => { const u = {...configData, terminal: e.target.value}; config.set(u); saveConfig(u); }}>
        {#each configData.availableTerminals || ["default"] as t}
          <option value={t}>{t}{t === "default" ? " (auto-detect)" : t === "custom" ? " (custom command)" : ""}</option>
        {/each}
      </select>
    </div>
    <div class="field">
      <label>Default new project directory</label>
      <input type="text" value={configData.newProjectDir || ""} onchange={(e) => { const u = {...configData, newProjectDir: e.target.value}; config.set(u); saveConfig(u); }} />
    </div>
    <div class="field">
      <label>Provider binary paths</label>
      <div class="field-hint" style="margin-bottom:.4rem">Override if a tool isn't in your PATH.</div>
      {#each [...new Set(sessionList.map(s => s.provider))] as p}
        <div style="display:flex;gap:.5rem;margin-bottom:.4rem;align-items:center">
          <label style="width:6rem;font-size:.82rem;font-weight:500">{p}</label>
          <input type="text" value={configData.providerPaths?.[p] || ""} placeholder="auto-detect" style="flex:1"
            onchange={(e) => {
              const paths = {...(configData.providerPaths || {}), [p]: e.target.value};
              if (!e.target.value) delete paths[p];
              const u = {...configData, providerPaths: paths};
              config.set(u); saveConfig(u);
            }} />
        </div>
      {/each}
    </div>
    <div class="field">
      <label>Remote sources (SSH)</label>
      <div class="field-hint" style="margin-bottom:.6rem">Scan AI coding sessions on remote machines. Changes require restart.</div>
      {#each configData.remoteSources || [] as src, i}
        <div style="display:flex;gap:.4rem;margin-bottom:.5rem;align-items:center;flex-wrap:wrap">
          <input type="text" value={src.name || ""} placeholder="name" style="width:6rem"
            onchange={(e) => updateRemoteSource(i, "name", e.target.value)} />
          <input type="text" value={src.user || ""} placeholder="user" style="width:5rem"
            onchange={(e) => updateRemoteSource(i, "user", e.target.value)} />
          <span style="color:var(--text-secondary)">@</span>
          <input type="text" value={src.host || ""} placeholder="hostname or IP" style="flex:1;min-width:8rem"
            onchange={(e) => updateRemoteSource(i, "host", e.target.value)} />
          <select value={src.method || "ssh"} style="width:5rem"
            onchange={(e) => updateRemoteSource(i, "method", e.target.value)}>
            <option value="ssh">SSH</option>
            <option value="http">HTTP</option>
          </select>
          <button class="btn btn-sm" onclick={() => doTestSSH(src)} title="Test connection">Test</button>
          <button class="btn btn-sm" style="color:var(--danger)" onclick={() => removeRemoteSource(i)}>&#10005;</button>
        </div>
      {/each}
      <button class="btn btn-sm" onclick={addRemoteSource}>+ Add remote source</button>
    </div>
  </div>
{/snippet}

<!-- New Project Modal -->
<div class="overlay" class:open={openModal === "newProject"} onclick={onOverlayClick}>
  <div class="modal">
    <h2><span style="color:var(--primary)">&#9670;</span> New Project</h2>
    <div class="field">
      <label>Project directory</label>
      <input type="text" bind:value={newProjectDir} placeholder="/home/user/projects/my-new-app" />
    </div>
    <div style="display:flex;gap:.8rem">
      <div class="field" style="flex:1">
        <label>Tool</label>
        <select bind:value={newProjectTool}>
          {#each availableTools as t}
            <option value={t}>{providerLabels[t] || t}</option>
          {/each}
        </select>
      </div>
      <div class="field" style="flex:1">
        <label>Model (optional)</label>
        <select bind:value={newProjectModel}>
          <option value="">Default</option>
          {#each configData.models || [] as m}
            <option value={m}>{m}</option>
          {/each}
        </select>
      </div>
    </div>
    <div class="field-hint">Creates the directory and launches the selected tool inside it.</div>
    <div class="actions">
      <button class="btn" onclick={() => (openModal = null)}>Cancel</button>
      <button class="btn btn-primary" onclick={doCreateProject}>Create &amp; Launch</button>
    </div>
  </div>
</div>

<!-- Settings Modal -->
<div class="overlay" class:open={openModal === "settings"} onclick={onOverlayClick}>
  <div class="modal">
    <h2><span>&#9881;</span> Settings</h2>
    <div class="field">
      <label>Terminal emulator</label>
      <select bind:value={settingsTerminal}>
        {#each availableTerminals as t}
          <option value={t}>{t}{t === "default" ? " (auto-detect)" : t === "custom" ? " (custom command)" : ""}</option>
        {/each}
      </select>
      <div class="field-hint">Which terminal to open when resuming a session from the web UI.</div>
    </div>
    <div class="field">
      <label>Default new project directory</label>
      <input type="text" bind:value={settingsNewDir} placeholder="/home/user/projects" />
    </div>
    <div class="field">
      <label>Provider binary paths</label>
      <div class="field-hint" style="margin-bottom:.4rem">Override if a tool isn't in your PATH (e.g. installed via nvm).</div>
      {#each settingsProviders as p}
        <div style="display:flex;gap:.5rem;margin-bottom:.4rem;align-items:center">
          <label style="width:5rem;font-size:.8rem;font-weight:500">{p}</label>
          <input
            type="text"
            value={settingsProviderPaths[p] || ""}
            oninput={(e) => (settingsProviderPaths = { ...settingsProviderPaths, [p]: e.target.value })}
            placeholder="auto-detect from PATH"
            style="flex:1;padding:.4rem .6rem;font-size:.8rem"
          />
        </div>
      {/each}
    </div>
    <div class="actions">
      <button class="btn" onclick={() => (openModal = null)}>Cancel</button>
      <button class="btn btn-primary" onclick={doSaveSettings}>Save</button>
    </div>
  </div>
</div>

<!-- Resume with Model Modal -->
<div class="overlay" class:open={openModal === "resume"} onclick={onOverlayClick}>
  <div class="modal">
    <h2><span style="color:var(--primary)">&#9670;</span> Resume Session</h2>
    <div class="field">
      <label>Model</label>
      <select bind:value={resumeModel}>
        {#each resumeModelList as m}
          <option value={m}>{m}{pendingResume && m === pendingResume.current ? " (current)" : ""}</option>
        {/each}
      </select>
      <div class="field-hint">Pick a model to resume with. The 1M context variants give extended context windows.</div>
    </div>
    <div class="actions">
      <button class="btn" onclick={() => (openModal = null)}>Cancel</button>
      <button class="btn btn-primary" onclick={confirmResume}>Resume &rarr;</button>
    </div>
  </div>
</div>

<!-- Delete Confirmation Modal -->
<div class="overlay" class:open={openModal === "delete"} onclick={onOverlayClick}>
  <div class="modal">
    <h2><span style="color:var(--danger)">&#9888;</span> Delete Session</h2>
    <div class="delete-warning">
      {#if pendingDelete}
        This will permanently delete the session data from <b>{pendingDelete.projectName}</b>. This cannot be undone.
      {/if}
    </div>
    <div class="actions">
      <button class="btn" onclick={() => (openModal = null)}>Cancel</button>
      <button class="btn btn-danger" onclick={doDelete}>Delete</button>
    </div>
  </div>
</div>

<!-- Toast Area -->
<div class="toast-area">
  {#each toasts as t (t.id)}
    <div class="toast {t.type}">{t.msg}</div>
  {/each}
</div>
