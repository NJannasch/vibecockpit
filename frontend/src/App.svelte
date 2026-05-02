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
  import { saveConfig, launchSession, deleteSession, createProject as apiCreateProject, testSSH, startSecretScan, getScanStatus } from "./lib/api.js";
  import { shortModel, relativeTime, providerColors, providerLabels, dateGroup } from "./lib/utils.js";
  import Dashboard from "./components/Dashboard.svelte";
  import CostsDashboard from "./components/CostsDashboard.svelte";
  import ToolInventory from "./components/ToolInventory.svelte";
  import BoardView from "./components/BoardView.svelte";

  // ─── Reactive State (Svelte 5 runes) ───

  const validPages = ["dashboard", "planner", "sessions", "costs", "inventory", "settings"];
  const redirects = { stats: "dashboard", security: "inventory", mcp: "settings", boards: "planner" };
  function pageFromPath() {
    const p = window.location.pathname.replace(/^\/+/, "").split("/")[0];
    if (redirects[p]) return redirects[p];
    return validPages.includes(p) ? p : "dashboard";
  }
  let page = $state(pageFromPath());
  let openModal = $state(null); // "newProject" | "resume" | "delete" | null
  let toasts = $state([]);
  let newProjectDir = $state("");
  let newProjectTool = $state("");
  let newProjectModel = $state("");
  let lastScanned = $state(null);
  let scanning = $state(false);
  let scanStatus = $state({ state: "idle", findings: [], findingCount: 0, sessionsDone: 0, sessionsTotal: 0 });
  let scanPollTimer = $state(null);
  let pendingDelete = $state(null);
  let pendingResume = $state(null);
  let resumeModel = $state("");
  let collapsedGroups = $state(new Set());
  let collapsedTreeNodes = $state(new Set());
  let showWizard = $state(false);
  let wizardDisabled = $state([]);

  // Store subscriptions for local reactivity
  let sessionList = $state([]);
  let configData = $state({});
  let filterData = $state({ filtered: [], activeCount: 0, fuzzyTerms: [] });
  let currentViewVal = $state("list");
  let searchVal = $state("");
  let sortByVal = $state("modified");
  let groupByVal = $state("none");
  let activeFiltersVal = $state({});

  let activeSessionCount = $derived(sessionList.filter(s => s.isActive).length);
  let totalEstCost = $derived(sessionList.reduce((sum, s) => sum + (s.estCostUsd || 0), 0));
  let versionInfo = $state(null);
  let showPrivacyNotice = $state(!localStorage.getItem("vibecockpit-privacy-ack"));
  let mcpAuditLog = $state([]);
  let mcpAuditExpanded = $state(new Set());

  async function loadMCPAudit() {
    try {
      const r = await fetch("/api/mcp-audit?limit=200");
      if (r.ok) mcpAuditLog = await r.json();
    } catch { /* optional */ }
  }

  function toggleAuditRow(i) {
    const next = new Set(mcpAuditExpanded);
    if (next.has(i)) next.delete(i);
    else next.add(i);
    mcpAuditExpanded = next;
  }

  async function loadVersionInfo() {
    try {
      const r = await fetch("/api/version");
      if (r.ok) versionInfo = await r.json();
    } catch { /* version endpoint optional */ }
  }

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
    window.removeEventListener("popstate", handlePopState);
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

  let scanDurationMs = $state(0);

  async function refresh(force = true) {
    scanning = true;
    const start = performance.now();
    await loadSessions(force);
    scanDurationMs = Math.round(performance.now() - start);
    lastScanned = new Date();
    scanning = false;
  }

  function handlePopState() {
    page = pageFromPath();
  }

  onMount(async () => {
    window.addEventListener("popstate", handlePopState);
    await loadConfig();
    applyTheme(configData.theme === "dark" ? "dark" : "light");
    loadVersionInfo();
    if (!localStorage.getItem("vc-wizard-done")) {
      showWizard = true;
      return;
    }
    await refresh(false);
    startAutoRefresh();
  });

  function wizardToggle(id) {
    if (wizardDisabled.includes(id)) {
      wizardDisabled = wizardDisabled.filter(x => x !== id);
    } else {
      wizardDisabled = [...wizardDisabled, id];
    }
  }

  async function wizardFinish() {
    const updated = {...configData, disabledProviders: wizardDisabled};
    config.set(updated);
    await saveConfig(updated);
    localStorage.setItem("vc-wizard-done", "1");
    showWizard = false;
    await refresh(false);
    startAutoRefresh();
  }

  async function wizardScanAll() {
    const updated = {...configData, disabledProviders: []};
    config.set(updated);
    await saveConfig(updated);
    localStorage.setItem("vc-wizard-done", "1");
    showWizard = false;
    await refresh(false);
    startAutoRefresh();
  }

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
    const url = p === "dashboard" ? "/" : "/" + p;
    if (window.location.pathname !== url) {
      history.pushState({ page: p }, "", url);
    }
    if (p === "security" && !scanPollTimer) {
      getScanStatus().then(s => {
        scanStatus = s;
        if (s.state === "scanning") pollScan();
      }).catch(() => {});
    }
    if (p === "settings" || p === "mcp") {
      loadMCPAudit();
    }
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

  let highlightProvider = $state("");
  let settingsSaved = $state(false);
  let settingsSavedTimer = $state(null);
  let settingsTab = $state("general");

  async function doLaunch(sessionId, provider, modelOverride) {
    try {
      const data = await launchSession(sessionId, provider, modelOverride);
      if (data.error) {
        if (data.error.includes("PATH")) {
          showToast(
            `"${provider}" not found in PATH. Configure its location in Settings → Provider paths.`,
            "error"
          );
          highlightProvider = provider;
          page = "settings";
          // Scroll to provider paths after render
          setTimeout(() => {
            const el = document.getElementById("provider-path-" + provider);
            if (el) { el.focus(); el.scrollIntoView({ behavior: "smooth", block: "center" }); }
          }, 200);
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

  // ─── Secret Scan ───

  async function triggerScan() {
    if (scanStatus.state === "scanning") {
      // TODO: stop scan via API
      return;
    }
    scanStatus = { state: "starting", findings: [], findingCount: 0, sessionsDone: 0, sessionsTotal: 0, filesScanned: 0, linesScanned: 0 };
    await startSecretScan();
    pollScan();
  }

  function pollScan() {
    if (scanPollTimer) clearInterval(scanPollTimer);
    scanPollTimer = setInterval(async () => {
      try {
        scanStatus = await getScanStatus();
        if (scanStatus.state === "done") {
          clearInterval(scanPollTimer);
          scanPollTimer = null;
        }
      } catch { /* poll failure is transient */ }
    }, 1500);
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

  // eslint-disable-next-line no-unused-vars
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

  // Converts highlight markers (NUL-delimited) to HTML
  function highlightHtml(text, terms) {
    /* eslint-disable no-control-regex */
    return highlight(text, terms)
      .replace(/\x00HL_START\x00/g, '<span class="match-hl">')
      .replace(/\x00HL_END\x00/g, "</span>");
    /* eslint-enable no-control-regex */
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

  function formatAuditParams(params) {
    if (!params || typeof params !== "object") return "—";
    const entries = Object.entries(params).filter(([, v]) => v !== undefined && v !== null && v !== "");
    if (entries.length === 0) return "—";
    return entries.map(([k, v]) => `${k}=${v}`).join(", ");
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

<!-- Sidebar -->
<aside class="sidebar">
  <div class="sidebar-logo">
    <span class="logo-icon">&#9670;</span>
    <span class="sidebar-logo-text">VibeCockpit</span>
  </div>
  <nav class="sidebar-nav">
    <button class="sidebar-btn" class:active={page === "dashboard"} onclick={() => navigateTo("dashboard")}>
      <svg class="sidebar-svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><rect x="3" y="3" width="7" height="7" rx="1"/><rect x="14" y="3" width="7" height="7" rx="1"/><rect x="3" y="14" width="7" height="7" rx="1"/><rect x="14" y="14" width="7" height="7" rx="1"/></svg>
      <span class="sidebar-label">Dashboard</span>
    </button>
    <button class="sidebar-btn" class:active={page === "planner"} onclick={() => navigateTo("planner")}>
      <svg class="sidebar-svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><rect x="3" y="3" width="18" height="18" rx="2"/><line x1="9" y1="3" x2="9" y2="21"/><line x1="15" y1="3" x2="15" y2="21"/></svg>
      <span class="sidebar-label">Planner</span>
    </button>
    <button class="sidebar-btn" class:active={page === "sessions"} onclick={() => navigateTo("sessions")}>
      <svg class="sidebar-svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M21 15a2 2 0 01-2 2H7l-4 4V5a2 2 0 012-2h14a2 2 0 012 2z"/></svg>
      <span class="sidebar-label">Sessions</span>
      {#if activeSessionCount > 0}<span class="sidebar-count sidebar-count-active">{activeSessionCount}</span>{/if}
    </button>
    <button class="sidebar-btn" class:active={page === "costs"} onclick={() => navigateTo("costs")}>
      <svg class="sidebar-svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><line x1="12" y1="1" x2="12" y2="23"/><path d="M17 5H9.5a3.5 3.5 0 000 7h5a3.5 3.5 0 010 7H6"/></svg>
      <span class="sidebar-label">Costs</span>
      {#if totalEstCost > 0}<span class="sidebar-count">~${totalEstCost >= 1000 ? (totalEstCost/1000).toFixed(1) + "k" : totalEstCost.toFixed(0)}</span>{/if}
    </button>
    <button class="sidebar-btn" class:active={page === "inventory"} onclick={() => navigateTo("inventory")}>
      <svg class="sidebar-svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M20 7l-8-4-8 4m16 0l-8 4m8-4v10l-8 4m0-10L4 7m8 4v10M4 7v10l8 4"/></svg>
      <span class="sidebar-label">Inventory</span>
    </button>
    <button class="sidebar-btn" class:active={page === "settings"} onclick={() => navigateTo("settings")}>
      <svg class="sidebar-svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="12" cy="12" r="3"/><path d="M19.4 15a1.65 1.65 0 00.33 1.82l.06.06a2 2 0 010 2.83 2 2 0 01-2.83 0l-.06-.06a1.65 1.65 0 00-1.82-.33 1.65 1.65 0 00-1 1.51V21a2 2 0 01-4 0v-.09A1.65 1.65 0 009 19.4a1.65 1.65 0 00-1.82.33l-.06.06a2 2 0 01-2.83-2.83l.06-.06A1.65 1.65 0 004.68 15a1.65 1.65 0 00-1.51-1H3a2 2 0 010-4h.09A1.65 1.65 0 004.6 9a1.65 1.65 0 00-.33-1.82l-.06-.06a2 2 0 012.83-2.83l.06.06A1.65 1.65 0 009 4.68a1.65 1.65 0 001-1.51V3a2 2 0 014 0v.09a1.65 1.65 0 001 1.51 1.65 1.65 0 001.82-.33l.06-.06a2 2 0 012.83 2.83l-.06.06A1.65 1.65 0 0019.4 9a1.65 1.65 0 001.51 1H21a2 2 0 010 4h-.09a1.65 1.65 0 00-1.51 1z"/></svg>
      <span class="sidebar-label">Settings</span>
    </button>
  </nav>
  <div class="sidebar-bottom">
    <button class="sidebar-btn sidebar-btn-sm" onclick={refresh} disabled={scanning} title={lastScanned ? "Last scan: " + relativeTime(lastScanned.toISOString()) : "Scan for sessions"}>
      <svg class="sidebar-svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><polyline points="23 4 23 10 17 10"/><path d="M20.49 15a9 9 0 11-2.12-9.36L23 10"/></svg>
      <span class="sidebar-label">{scanning ? "Scanning..." : "Refresh"}</span>
    </button>
    <button class="sidebar-btn sidebar-btn-sm" onclick={showNewProject} title="Start a new coding session">
      <svg class="sidebar-svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><line x1="12" y1="5" x2="12" y2="19"/><line x1="5" y1="12" x2="19" y2="12"/></svg>
      <span class="sidebar-label">New Project</span>
    </button>
    <div class="sidebar-row-inline">
      <button class="sidebar-btn-icon" onclick={toggleTheme} title="Toggle theme">{themeIcon}</button>
      <a href="https://github.com/njannasch/vibecockpit" target="_blank" class="sidebar-btn-icon" title="GitHub" aria-label="GitHub">
        <svg width="14" height="14" viewBox="0 0 24 24" fill="currentColor" aria-hidden="true">
          <path d="M12 0C5.37 0 0 5.37 0 12c0 5.3 3.438 9.8 8.205 11.385.6.111.82-.26.82-.577v-2.234c-3.338.726-4.033-1.416-4.033-1.416-.546-1.387-1.333-1.756-1.333-1.756-1.089-.745.083-.729.083-.729 1.205.084 1.838 1.236 1.838 1.236 1.07 1.835 2.807 1.305 3.492.998.108-.776.418-1.305.762-1.605-2.665-.305-5.467-1.334-5.467-5.93 0-1.31.469-2.381 1.236-3.221-.124-.303-.535-1.524.117-3.176 0 0 1.008-.322 3.301 1.23A11.5 11.5 0 0112 5.803c1.02.005 2.045.138 3.003.404 2.291-1.552 3.297-1.23 3.297-1.23.654 1.652.243 2.873.119 3.176.77.84 1.235 1.911 1.235 3.221 0 4.61-2.807 5.624-5.479 5.92.43.372.823 1.102.823 2.222v3.293c0 .319.218.694.825.576C20.565 21.796 24 17.298 24 12c0-6.63-5.37-12-12-12z"/>
        </svg>
      </a>
    </div>
    {#if versionInfo?.current}
      <div class="sidebar-version">
        {#if versionInfo.updateAvailable}
          <a class="version version-update" href={versionInfo.releaseUrl} target="_blank">{versionInfo.current} ↑</a>
        {:else}
          <span class="version">{versionInfo.current}</span>
        {/if}
      </div>
    {/if}
  </div>
</aside>
<div class="app-content">
{#if !lastScanned && scanning}
  <main>
    <div style="max-width:800px;margin:3rem auto;padding:0 1rem">
      <div class="skeleton">
        <div style="font-size:1.5rem;color:var(--primary);margin-bottom:.5rem">&#9670;</div>
        <div style="font-size:.95rem;font-weight:500;margin-bottom:.3rem">Scanning your AI coding tools...</div>
        <div style="font-size:.78rem;color:var(--text-secondary);margin-bottom:1.5rem">Detecting sessions from Claude, Codex, Copilot, Gemini, OpenCode</div>
        <div class="skeleton-grid">
          <div class="skeleton-card"></div>
          <div class="skeleton-card"></div>
          <div class="skeleton-card"></div>
          <div class="skeleton-card"></div>
          <div class="skeleton-card"></div>
        </div>
      </div>
    </div>
  </main>
{:else if page === "dashboard"}
  <div class="page-bar">
    <h1 class="page-bar-title">Dashboard</h1>
    <span class="page-bar-subtitle">{sessionList.length} sessions · {activeSessionCount > 0 ? `${activeSessionCount} active` : "none active"}</span>
  </div>
  <main>
    <Dashboard
      sessions={sessionList}
      onnavigate={navigateTo}
      onlaunch={(id, prov) => launch(id, prov)}
      onfilterby={filterByProvider}
      mcpEnabled={configData.enableMcp || false}
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
      {#each chipData.providers as p (p)}
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
      {#each chipData.models as m (m)}
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
      <p>Your AI coding control plane. VibeCockpit auto-detects sessions from Claude Code, Codex, Copilot, Gemini, OpenCode, and more.</p>
      <div class="steps">
        <div class="step">
          <span class="step-num">1</span>
          <div>
            <h4>Use any AI coding tool</h4>
            <p>Claude Code, Codex CLI, Copilot, Gemini CLI, OpenCode, Cursor</p>
          </div>
        </div>
        <div class="step">
          <span class="step-num">2</span>
          <div>
            <h4>Sessions appear here</h4>
            <p>VibeCockpit scans your tool directories automatically</p>
          </div>
        </div>
        <div class="step">
          <span class="step-num">3</span>
          <div>
            <h4>Connect agents via MCP</h4>
            <p>Add <code>.mcp.json</code> to your project and agents can track tasks and costs</p>
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
        {#each Object.keys(node.children).sort() as name (name)}
          {@const child = node.children[name]}
          {@const allSessions = collectSessions(child)}
          {@const providers = [...new Set(allSessions.map((s) => s.provider))]}
          {@const nodeId = prefix + "/" + name}
          {@const isCollapsed = collapsedTreeNodes.has(nodeId)}
          {@const isUnlinked = name === "unlinked"}
          <div class="tree-node" class:collapsed={isCollapsed}>
            <button class="tree-dir" onclick={() => toggleTreeNode(nodeId)}>
              <span class="tree-chevron">&#9660;</span>
              <span class="tree-dir-icon">{isUnlinked ? "◇" : "▷"}</span>
              <span class="tree-dir-name">{isUnlinked ? "Unlinked sessions" : name}</span>
              <span class="tree-dir-count">{allSessions.length}</span>
              <span class="tree-dir-providers">
                {#each providers as p (p)}
                  <span
                    class="badge"
                    style="background:{providerColors[p] || 'var(--primary)'}18;color:{providerColors[p] ||
                      'var(--primary)'};border-color:{providerColors[p] || 'var(--primary)'}30"
                  >
                    {p}
                  </span>
                {/each}
              </span>
            </button>
            <div class="tree-children">
              {@render treeNodes(child, nodeId)}
              {#each child.sessions.sort((a, b) => new Date(b.modified) - new Date(a.modified)) as s, si (s.id + '-' + si)}
                {@const summary = s.summary || s.firstPrompt || "untitled"}
                {@const color = providerColors[s.provider] || "var(--text-secondary)"}
                <button class="tree-session" onclick={() => launch(s.id, s.provider)}>
                  <span class="tree-session-provider" style="background:{color}" title={s.provider}></span>
                  <span class="tree-session-summary">{summary.slice(0, 70)}</span>
                  <span class="tree-session-time">{relativeTime(s.modified)}</span>
                </button>
              {/each}
            </div>
          </div>
        {/each}
      {/snippet}
      {@render treeNodes(treeData, "")}
    </div>
  {:else if groupedSessions}
    <!-- Grouped List View -->
    {#each groupedSessions as { key, items } (key)}
      <div class="group" class:collapsed={collapsedGroups.has(key)}>
        <button class="group-header" onclick={() => toggleGroup(key)}>
          <span class="group-chevron">&#9660;</span>
          <span class="group-name">
            {#if groupByVal === "provider"}{providerLabels[key] || key}{:else}{key}{/if}
          </span>
          <span class="group-count">{items.length}</span>
        </button>
        <div class="group-sessions sessions">
          {#each items as s, si (s.id + '-' + si)}
            {@const summary = s.summary || s.firstPrompt || ""}
            {@const displaySummary = summary.length > 120 ? summary.slice(0, 117) + "..." : summary}
            {@const model = shortModel(s.model)}
            {@const hp = homePath(s.projectPath)}
            <div class={s.isActive ? "card active" : "card"} role="button" tabindex="0" onclick={(e) => launch(s.id, s.provider, e)} onkeydown={(e) => { if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); launch(s.id, s.provider, e); } }}>
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
                {#if s.estCostUsd > 0.01}
                  <span class="cost-badge {s.tokens?.inputTokens ? 'cost-badge-value' : 'cost-badge-estimate'}">
                    {s.tokens?.inputTokens ? "$" : "~$"}{s.estCostUsd.toFixed(2)}
                  </span>
                {:else if s.provider?.startsWith("remote-")}
                  <span class="cost-badge cost-badge-remote">remote</span>
                {/if}
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
      {#each filterData.filtered as s, si (s.id + '-' + si)}
        {@const summary = s.summary || s.firstPrompt || ""}
        {@const displaySummary = summary.length > 120 ? summary.slice(0, 117) + "..." : summary}
        {@const model = shortModel(s.model)}
        {@const hp = homePath(s.projectPath)}
        <div class={s.isActive ? "card active" : "card"} role="button" tabindex="0" onclick={(e) => launch(s.id, s.provider, e)} onkeydown={(e) => { if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); launch(s.id, s.provider, e); } }}>
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
            {#if s.estCostUsd > 0.01}
              <span class="cost-badge {s.tokens?.inputTokens ? 'cost-badge-value' : 'cost-badge-estimate'}">
                {s.tokens?.inputTokens ? "$" : "~$"}{s.estCostUsd.toFixed(2)}
              </span>
            {:else if s.provider?.startsWith("remote-")}
              <span class="cost-badge cost-badge-remote">remote</span>
            {/if}
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
  <div class="privacy-notice">
    <span class="privacy-icon">&#128274;</span>
    VibeCockpit scans your local AI tool directories (e.g. <code>~/.claude</code>, <code>~/.codex</code>) to discover sessions, configs, and extensions. All analysis happens entirely on your machine — no data is sent anywhere.
  </div>
</main>
{:else if page === "planner"}
  <div class="page-bar">
    <h1 class="page-bar-title">Planner</h1>
    <span class="page-bar-subtitle">Plan and track agentic tasks</span>
  </div>
  <main>
    <BoardView sessions={sessionList} />
  </main>
{:else if page === "costs"}
  <div class="page-bar">
    <h1 class="page-bar-title">Costs</h1>
    <span class="page-bar-subtitle">~${totalEstCost >= 1000 ? (totalEstCost/1000).toFixed(1) + "k" : totalEstCost.toFixed(0)} estimated across all sessions</span>
  </div>
  <main>
    <CostsDashboard sessions={sessionList} />
  </main>
{:else if page === "inventory"}
  <div class="page-bar">
    <h1 class="page-bar-title">Inventory</h1>
    <span class="page-bar-subtitle">Tools, extensions, MCP servers, configs</span>
  </div>
  <main>
    <ToolInventory />
    {#if configData.enableScanner}
      {@render securityPage()}
    {/if}
  </main>
{:else if page === "settings"}
  <div class="page-bar">
    <h1 class="page-bar-title">Settings</h1>
    <span class="page-bar-spacer"></span>
    <div class="page-bar-actions">
      <button class="settings-tab" class:active={settingsTab === "general"} onclick={() => { settingsTab = "general"; }}>General</button>
      {#if configData.enableMcp}
        <button class="settings-tab" class:active={settingsTab === "mcp"} onclick={() => { settingsTab = "mcp"; loadMCPAudit(); }}>MCP</button>
      {/if}
    </div>
  </div>
  <main>
    {#if settingsTab === "general"}
      {@render settingsPage()}
    {:else if settingsTab === "mcp"}
      {@render mcpPage()}
    {/if}
  </main>
{/if}
</div>

{#snippet mcpPage()}
  {@const toolColors = {
    list_sessions: "#6366f1",
    search_sessions: "#8b5cf6",
    get_session_detail: "#a78bfa",
    scan_secrets: "#ef4444",
    get_costs: "#f59e0b",
    get_stats: "#10b981",
    get_inventory: "#3b82f6",
    rescan: "#06b6d4"
  }}
  {@const toolCounts = mcpAuditLog.reduce((acc, e) => { acc[e.tool] = (acc[e.tool] || 0) + 1; return acc; }, {})}
  <div class="mcp-page">
    <div class="mcp-page-header">
      <div>
        <h2>MCP Server</h2>
        <p style="font-size:.82rem;color:var(--text-secondary)">Tool call history from AI assistants connected via Model Context Protocol.</p>
      </div>
      <button class="btn btn-sm" onclick={loadMCPAudit}>Refresh</button>
    </div>

    <!-- Stats bar -->
    <div class="mcp-stats">
      <div class="mcp-stat">
        <span class="mcp-stat-value">{mcpAuditLog.length}</span>
        <span class="mcp-stat-label">total calls</span>
      </div>
      <div class="mcp-stat">
        <span class="mcp-stat-value">{Object.keys(toolCounts).length}</span>
        <span class="mcp-stat-label">tools used</span>
      </div>
      <div class="mcp-stat">
        <span class="mcp-stat-value">{mcpAuditLog.length > 0 ? relativeTime(mcpAuditLog[0].timestamp) : "—"}</span>
        <span class="mcp-stat-label">last call</span>
      </div>
    </div>

    <!-- Tool breakdown -->
    {#if Object.keys(toolCounts).length > 0}
    <div class="mcp-tool-bar">
      {#each Object.entries(toolCounts).sort((a, b) => b[1] - a[1]) as [tool, count] (tool)}
        <div class="mcp-tool-chip" style="--chip-color:{toolColors[tool] || '#888'}">
          <span class="mcp-tool-chip-dot"></span>
          <span>{tool}</span>
          <span class="mcp-tool-chip-count">{count}</span>
        </div>
      {/each}
    </div>
    {/if}

    <!-- Audit log table -->
    {#if mcpAuditLog.length === 0}
      <div class="mcp-empty">
        <p>No MCP tool calls recorded yet.</p>
        <p style="font-size:.82rem;color:var(--text-secondary)">Tool calls will appear here once an AI assistant uses VibeCockpit via MCP.</p>
      </div>
    {:else}
      <div class="mcp-audit-table mcp-audit-full">
        <div class="mcp-audit-row mcp-audit-header">
          <span class="mcp-col-time">Time</span>
          <span class="mcp-col-tool">Tool</span>
          <span class="mcp-col-params">Parameters</span>
          <span class="mcp-col-results">Results</span>
          <span class="mcp-col-hash">Hash</span>
        </div>
        {#each mcpAuditLog as entry, i (i)}
          <button class="mcp-audit-row mcp-audit-data" class:mcp-row-expanded={mcpAuditExpanded.has(i)} onclick={() => toggleAuditRow(i)}>
            <span class="mcp-col-time" title={entry.timestamp}>{relativeTime(entry.timestamp)}</span>
            <span class="mcp-col-tool">
              <span class="mcp-tool-dot" style="background:{toolColors[entry.tool] || '#888'}"></span>
              <code>{entry.tool}</code>
            </span>
            <span class="mcp-col-params">{formatAuditParams(entry.params)}</span>
            <span class="mcp-col-results">{entry.resultCount}</span>
            <span class="mcp-col-hash"><code>{entry.resultHash}</code></span>
          </button>
          {#if mcpAuditExpanded.has(i)}
            <div class="mcp-audit-detail">
              <div class="mcp-detail-section">
                <span class="mcp-detail-label">Timestamp</span>
                <span>{entry.timestamp}</span>
              </div>
              <div class="mcp-detail-section">
                <span class="mcp-detail-label">Tool</span>
                <code>{entry.tool}</code>
              </div>
              <div class="mcp-detail-section">
                <span class="mcp-detail-label">Parameters</span>
                <pre class="mcp-detail-json">{JSON.stringify(entry.params, null, 2)}</pre>
              </div>
              <div class="mcp-detail-section">
                <span class="mcp-detail-label">Result count</span>
                <span>{entry.resultCount}</span>
              </div>
              <div class="mcp-detail-section">
                <span class="mcp-detail-label">Result hash</span>
                <code>{entry.resultHash}</code>
              </div>
            </div>
          {/if}
        {/each}
      </div>
    {/if}
  </div>
{/snippet}

{#snippet securityPage()}
  <div style="max-width:900px;margin:2rem auto;padding:0 1rem">
    <!-- Header -->
    <div style="display:flex;align-items:center;justify-content:space-between;margin-bottom:1.5rem">
      <div>
        <h2 style="margin-bottom:.3rem">Secret Scanner</h2>
        <p style="font-size:.82rem;color:var(--text-secondary)">Scan session files for leaked API keys, tokens, passwords, and other secrets.</p>
      </div>
      <button
        class="btn"
        class:btn-primary={scanStatus.state !== "scanning"}
        class:btn-danger={scanStatus.state === "scanning"}
        onclick={triggerScan}
        disabled={scanStatus.state === "starting"}
      >
        {#if scanStatus.state === "scanning"}
          Stop Scan
        {:else if scanStatus.state === "starting"}
          Starting...
        {:else}
          Start Scan
        {/if}
      </button>
    </div>

    <!-- Progress panel (scanning or done) -->
    {#if scanStatus.state === "scanning" || scanStatus.state === "starting"}
      <div style="background:var(--surface);border:1px solid var(--border);border-radius:var(--radius);padding:1rem;margin-bottom:1rem">
        <div style="display:flex;justify-content:space-between;font-size:.85rem;margin-bottom:.5rem">
          <span>Scanning: <b>{scanStatus.currentFile || "initializing..."}</b></span>
          <span style="color:var(--text-secondary)">{scanStatus.sessionsDone}/{scanStatus.sessionsTotal} sessions</span>
        </div>
        <div style="height:6px;background:var(--surface-active);border-radius:3px;overflow:hidden;margin-bottom:.5rem">
          <div style="height:100%;background:var(--primary);border-radius:3px;transition:width .5s;width:{scanStatus.sessionsTotal ? (scanStatus.sessionsDone / scanStatus.sessionsTotal * 100) : 0}%"></div>
        </div>
        <div style="display:flex;gap:1.2rem;font-size:.75rem;color:var(--text-secondary)">
          <span>{scanStatus.filesScanned || 0} files scanned</span>
          <span>{(scanStatus.linesScanned || 0).toLocaleString()} lines processed</span>
          <span>{scanStatus.findingCount || 0} finding{scanStatus.findingCount !== 1 ? "s" : ""}</span>
          <span>{scanStatus.patternsLoaded || 0} rules loaded</span>
        </div>
      </div>
    {/if}

    {#if scanStatus.state === "done"}
      <div style="background:var(--surface);border:1px solid var(--border);border-radius:var(--radius);padding:1rem;margin-bottom:1rem">
        <div style="display:flex;gap:1.5rem;font-size:.85rem;flex-wrap:wrap">
          <span><b style="color:{scanStatus.findingCount > 0 ? 'var(--danger)' : 'var(--success)'}">{scanStatus.findingCount}</b> finding{scanStatus.findingCount !== 1 ? "s" : ""}</span>
          <span><b>{scanStatus.sessionsTotal}</b> sessions</span>
          <span><b>{scanStatus.filesScanned || 0}</b> files</span>
          <span><b>{(scanStatus.linesScanned || 0).toLocaleString()}</b> lines</span>
          <span><b>{scanStatus.patternsLoaded}</b> rules</span>
          <span style="color:var(--text-secondary)">{(scanStatus.durationMs / 1000).toFixed(1)}s</span>
        </div>
      </div>
    {/if}

    <!-- Findings table (shown during scan AND after done) -->
    {#if scanStatus.findings && scanStatus.findings.length > 0}
      <div style="background:var(--surface);border:1px solid var(--border);border-radius:var(--radius);overflow:hidden">
        <table style="width:100%;border-collapse:collapse;font-size:.82rem">
          <thead>
            <tr style="border-bottom:1px solid var(--border);text-align:left">
              <th style="padding:.5rem .8rem;font-weight:600">Rule</th>
              <th style="padding:.5rem .8rem;font-weight:600">Provider</th>
              <th style="padding:.5rem .8rem;font-weight:600">Project</th>
              <th style="padding:.5rem .8rem;font-weight:600">Line</th>
              <th style="padding:.5rem .8rem;font-weight:600">Match (partially redacted)</th>
            </tr>
          </thead>
          <tbody>
            {#each scanStatus.findings as f, i (i)}
              <tr style="border-bottom:1px solid var(--border)">
                <td style="padding:.4rem .8rem"><span style="background:var(--danger-dim);color:var(--danger);padding:.1rem .4rem;border-radius:4px;font-size:.72rem;font-weight:500">{f.ruleId}</span></td>
                <td style="padding:.4rem .8rem;font-size:.78rem">{f.provider}</td>
                <td style="padding:.4rem .8rem;font-weight:500;font-size:.78rem">{f.projectName}</td>
                <td style="padding:.4rem .8rem;font-family:monospace;font-size:.75rem">{f.line}</td>
                <td style="padding:.4rem .8rem;font-family:monospace;font-size:.73rem;color:var(--text-secondary);max-width:300px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap">{f.match}</td>
              </tr>
            {/each}
          </tbody>
        </table>
      </div>
    {:else if scanStatus.state === "done"}
      <div style="text-align:center;padding:3rem;color:var(--success)">
        <div style="font-size:2rem;margin-bottom:.5rem">&#10003;</div>
        <h3>No secrets found</h3>
        <p style="color:var(--text-secondary);font-size:.85rem">All {scanStatus.filesScanned || 0} files are clean.</p>
      </div>
    {:else if scanStatus.state === "idle"}
      <div style="text-align:center;padding:3rem;color:var(--text-secondary)">
        <p>Click "Start Scan" to check your session files for leaked secrets.</p>
        <p style="font-size:.78rem;margin-top:.5rem">Uses gitleaks detection rules. Read-only — nothing is modified.</p>
      </div>
    {/if}
  </div>
{/snippet}

{#snippet settingsPage()}
  {@const save = (updater) => {
    const u = updater(configData);
    config.set(u);
    saveConfig(u);
    settingsSaved = true;
    clearTimeout(settingsSavedTimer);
    settingsSavedTimer = setTimeout(() => settingsSaved = false, 2000);
  }}
  <div class="settings-page">
    <div class="settings-header">
      <h2>Settings</h2>
      {#if settingsSaved}
        <span class="settings-saved">Saved</span>
      {/if}
    </div>

    <!-- Status -->
    <div class="settings-card" style="background:var(--bg)">
      <h3 class="settings-section">Status</h3>
      <div style="display:flex;gap:1.5rem;font-size:.82rem;flex-wrap:wrap">
        <div><span style="color:var(--text-secondary)">Sessions:</span> <b>{sessionList.length}</b></div>
        <div><span style="color:var(--text-secondary)">Providers:</span> <b>{[...new Set(sessionList.map(s => s.provider))].length}</b></div>
        <div><span style="color:var(--text-secondary)">Last scan:</span> <b>{lastScanned ? relativeTime(lastScanned.toISOString()) : "never"}</b></div>
        <div><span style="color:var(--text-secondary)">Scan time:</span> <b>{scanDurationMs > 1000 ? (scanDurationMs / 1000).toFixed(1) + "s" : scanDurationMs + "ms"}</b></div>
        <div><span style="color:var(--text-secondary)">Version:</span> <b>{versionInfo?.current || "?"}</b></div>
      </div>
    </div>

    <!-- General -->
    <div class="settings-card">
      <h3 class="settings-section">General</h3>
      <div class="settings-row">
        <div class="settings-label">
          <span>Terminal emulator</span>
          <span class="field-hint">Used when launching sessions from the web UI</span>
        </div>
        <select value={configData.terminal || "default"} onchange={(e) => save(c => ({...c, terminal: e.target.value}))}>
          {#each configData.availableTerminals || ["default"] as t (t)}
            <option value={t}>{t}{t === "default" ? " (auto-detect)" : t === "custom" ? " (custom command)" : ""}</option>
          {/each}
        </select>
      </div>
      <div class="settings-row">
        <div class="settings-label">
          <span>Default project directory</span>
          <small style="color:var(--text-muted);font-weight:normal">Used for new projects and to fuzzy-match sessions to local folders</small>
        </div>
        <input type="text" value={configData.newProjectDir || ""} onchange={(e) => save(c => ({...c, newProjectDir: e.target.value}))} />
      </div>
    </div>

    <!-- Providers -->
    <div class="settings-card">
      <h3 class="settings-section">Providers</h3>
      <p style="color:var(--text-muted);font-size:.85rem;margin:0 0 .5rem">Choose which coding tools to scan for sessions. Disabled providers won't appear in the session list.</p>
      <div class="provider-toggles">
        {#each configData.allProviders || [] as p (p.id)}
          {@const disabled = (configData.disabledProviders || []).includes(p.id)}
          <label class="provider-toggle" class:disabled>
            <input type="checkbox" checked={!disabled} onchange={() => {
              const current = configData.disabledProviders || [];
              const next = disabled ? current.filter(id => id !== p.id) : [...current, p.id];
              save(c => ({...c, disabledProviders: next}));
            }} />
            <span class="provider-dot" style="background:{providerColors[p.id] || '#888'}"></span>
            {p.name}
          </label>
        {/each}
      </div>
    </div>

    <!-- Paths -->
    <div class="settings-card">
      <h3 class="settings-section">Paths</h3>
      <div class="settings-row" style="flex-direction:column;align-items:stretch">
        <div class="settings-label" style="margin-bottom:.4rem">
          <span>Extra PATH directories</span>
          <span class="field-hint">Prepended to PATH when launching tools (comma-separated)</span>
        </div>
        <input type="text" value={(configData.extraPath || []).join(", ")} placeholder="~/.nvm/versions/node/v23.11.1/bin"
          onchange={(e) => save(c => ({...c, extraPath: e.target.value.split(",").map(s => s.trim()).filter(Boolean)}))} />
      </div>
      <div class="settings-divider"></div>
      <div style="padding:.2rem 0">
        <div class="settings-label" style="margin-bottom:.5rem">
          <span>Provider binary paths</span>
          <span class="field-hint">Override if a tool isn't in your system PATH</span>
        </div>
        {#each [...new Set(sessionList.map(s => s.provider))] as p (p)}
          <div class="settings-path-row">
            <label class="settings-path-label" for="provider-path-{p}">{p}</label>
            <input id="provider-path-{p}" type="text" value={configData.providerPaths?.[p] || ""}
              placeholder="/path/to/{p}"
              class:settings-path-highlight={highlightProvider === p}
              onfocus={() => { if (highlightProvider === p) highlightProvider = ""; }}
              onchange={(e) => save(c => {
                const paths = {...(c.providerPaths || {}), [p]: e.target.value};
                if (!e.target.value) delete paths[p];
                return {...c, providerPaths: paths};
              })} />
          </div>
        {/each}
      </div>
    </div>

    <!-- Remote -->
    <div class="settings-card">
      <h3 class="settings-section">Remote Sources</h3>
      <p class="field-hint" style="margin-bottom:.8rem">Scan AI coding sessions on remote machines via SSH. Requires restart after changes.</p>
      {#each configData.remoteSources || [] as src, i (i)}
        <div class="settings-remote-row">
          <input type="text" value={src.name || ""} placeholder="name" style="width:6rem"
            onchange={(e) => updateRemoteSource(i, "name", e.target.value)} />
          <input type="text" value={src.user || ""} placeholder="user" style="width:5rem"
            onchange={(e) => updateRemoteSource(i, "user", e.target.value)} />
          <span style="color:var(--text-secondary);font-size:.85rem">@</span>
          <input type="text" value={src.host || ""} placeholder="hostname" style="flex:1;min-width:7rem"
            onchange={(e) => updateRemoteSource(i, "host", e.target.value)} />
          <select value={src.method || "ssh"} style="width:5rem"
            onchange={(e) => updateRemoteSource(i, "method", e.target.value)}>
            <option value="ssh">SSH</option>
            <option value="http">HTTP</option>
          </select>
          <button class="btn btn-sm" onclick={() => doTestSSH(src)}>Test</button>
          <button class="btn btn-sm" style="color:var(--danger)" onclick={() => removeRemoteSource(i)}>&#10005;</button>
        </div>
      {/each}
      <button class="btn btn-sm" onclick={addRemoteSource} style="margin-top:.3rem">+ Add remote source</button>
    </div>

    <!-- MCP Server -->
    <div class="settings-card">
      <h3 class="settings-section">MCP Server</h3>
      <div class="settings-row">
        <div class="settings-label">
          <span>Enable MCP server</span>
          <span class="field-hint">Expose VibeCockpit as an MCP tool server (JSON-RPC over stdio)</span>
        </div>
        <label class="toggle">
          <input type="checkbox" checked={configData.enableMcp || false}
            onchange={(e) => save(c => ({...c, enableMcp: e.target.checked}))} />
          <span class="toggle-slider"></span>
        </label>
      </div>
      {#if configData.enableMcp}
      <div class="mcp-info">
        <p style="font-size:.82rem;color:var(--text-secondary);margin:0 0 .6rem">
          When enabled, run <code>vibecockpit --mcp</code> to start the server. Add it to your AI tool's MCP config:
        </p>
        <pre class="mcp-snippet">{`{
  "mcpServers": {
    "vibecockpit": {
      "command": "vibecockpit",
      "args": ["--mcp"]
    }
  }
}`}</pre>
        <p style="font-size:.78rem;color:var(--text-muted);margin:.6rem 0 0">
          Tools: <code>list_sessions</code>, <code>search_sessions</code>, <code>get_session_detail</code>, <code>scan_secrets</code>, <code>get_costs</code>, <code>get_stats</code>, <code>get_inventory</code>, <code>rescan</code>
        </p>
        <p style="font-size:.78rem;color:var(--text-muted);margin:.4rem 0 0">
          View tool call history in the <button class="link-btn" onclick={() => { settingsTab = "mcp"; loadMCPAudit(); }}>MCP tab</button>.
        </p>
      </div>
      {/if}
    </div>

    <!-- Scanner (feature-flagged) -->
    {#if configData.enableScanner}
    <div class="settings-card">
      <h3 class="settings-section">Secret Scanner</h3>
      <div class="settings-row" style="flex-direction:column;align-items:stretch">
        <div class="settings-label" style="margin-bottom:.4rem">
          <span>Skip rules</span>
          <span class="field-hint">Rule IDs to exclude (comma-separated). <code>generic-api-key</code> is always skipped.</span>
        </div>
        <input type="text" value={(configData.scanSkipRules || []).join(", ")} placeholder="rule-id-1, rule-id-2"
          onchange={(e) => save(c => ({...c, scanSkipRules: e.target.value.split(",").map(s => s.trim()).filter(Boolean)}))} />
      </div>
      <div class="settings-row" style="flex-direction:column;align-items:stretch">
        <div class="settings-label" style="margin-bottom:.4rem">
          <span>Extra keywords</span>
          <span class="field-hint">Additional trigger keywords for the scanner (comma-separated)</span>
        </div>
        <input type="text" value={(configData.scanExtraHints || []).join(", ")} placeholder="my-service, custom-prefix"
          onchange={(e) => save(c => ({...c, scanExtraHints: e.target.value.split(",").map(s => s.trim()).filter(Boolean)}))} />
      </div>
    </div>
    {/if}
  </div>
{/snippet}

<!-- Privacy / Welcome Modal -->
{#if showPrivacyNotice}
<div class="overlay open" role="dialog" aria-modal="true" tabindex="-1" onclick={() => { localStorage.setItem("vibecockpit-privacy-ack", "1"); showPrivacyNotice = false; }} onkeydown={(e) => { if (e.key === 'Escape') { localStorage.setItem("vibecockpit-privacy-ack", "1"); showPrivacyNotice = false; } }}>
  <div class="modal privacy-modal" role="presentation" onclick={(e) => e.stopPropagation()}>
    <div class="privacy-modal-icon">&#9670;</div>
    <h2>Welcome to VibeCockpit</h2>
    <p class="privacy-modal-text">
      VibeCockpit scans your local AI tool directories (e.g. <code>~/.claude</code>, <code>~/.codex</code>, <code>~/.cursor</code>) to discover sessions, configuration files, IDE extensions, and usage statistics.
    </p>
    <p class="privacy-modal-text">
      <strong>All analysis happens entirely on your machine.</strong> No data is collected, transmitted, or shared with any external service.
    </p>
    <button class="btn btn-primary" onclick={() => { localStorage.setItem("vibecockpit-privacy-ack", "1"); showPrivacyNotice = false; }}>
      Got it
    </button>
  </div>
</div>
{/if}

<!-- New Project Modal -->
<div class="overlay" class:open={openModal === "newProject"} role="dialog" aria-modal="true" tabindex="-1" onclick={onOverlayClick} onkeydown={(e) => { if (e.key === 'Escape') onOverlayClick(); }}>
  <div class="modal" role="presentation">
    <h2><span style="color:var(--primary)">&#9670;</span> New Project</h2>
    <div class="field">
      <label for="new-project-dir">Project directory</label>
      <input id="new-project-dir" type="text" bind:value={newProjectDir} placeholder="/home/user/projects/my-new-app" />
    </div>
    <div style="display:flex;gap:.8rem">
      <div class="field" style="flex:1">
        <label for="new-project-tool">Tool</label>
        <select id="new-project-tool" bind:value={newProjectTool}>
          {#each availableTools as t (t)}
            <option value={t}>{providerLabels[t] || t}</option>
          {/each}
        </select>
      </div>
      <div class="field" style="flex:1">
        <label for="new-project-model">Model (optional)</label>
        <select id="new-project-model" bind:value={newProjectModel}>
          <option value="">Default</option>
          {#each configData.models || [] as m (m)}
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
<div class="overlay" class:open={openModal === "settings"} role="dialog" aria-modal="true" tabindex="-1" onclick={onOverlayClick} onkeydown={(e) => { if (e.key === 'Escape') onOverlayClick(); }}>
  <div class="modal" role="presentation">
    <h2><span>&#9881;</span> Settings</h2>
    <div class="field">
      <label for="settings-terminal">Terminal emulator</label>
      <select id="settings-terminal" bind:value={settingsTerminal}>
        {#each availableTerminals as t (t)}
          <option value={t}>{t}{t === "default" ? " (auto-detect)" : t === "custom" ? " (custom command)" : ""}</option>
        {/each}
      </select>
      <div class="field-hint">Which terminal to open when resuming a session from the web UI.</div>
    </div>
    <div class="field">
      <label for="settings-new-dir">Default new project directory</label>
      <input id="settings-new-dir" type="text" bind:value={settingsNewDir} placeholder="/home/user/projects" />
    </div>
    <div class="field">
      <span style="display:block;font-size:.82rem;color:var(--text-secondary);margin-bottom:.3rem">Provider binary paths</span>
      <div class="field-hint" style="margin-bottom:.4rem">Override if a tool isn't in your PATH (e.g. installed via nvm).</div>
      {#each settingsProviders as p (p)}
        <div style="display:flex;gap:.5rem;margin-bottom:.4rem;align-items:center">
          <label for="settings-provider-path-{p}" style="width:5rem;font-size:.8rem;font-weight:500">{p}</label>
          <input
            id="settings-provider-path-{p}"
            type="text"
            value={settingsProviderPaths[p] || ""}
            oninput={(e) => (settingsProviderPaths = { ...settingsProviderPaths, [p]: e.target.value })}
            placeholder="e.g. ~/.nvm/versions/node/v22.21.1/bin/{p}"
            style="flex:1;padding:.4rem .6rem;font-size:.8rem;{highlightProvider === p ? 'border-color:var(--danger);box-shadow:0 0 0 2px var(--danger-dim)' : ''}"
            onfocus={() => { if (highlightProvider === p) highlightProvider = ""; }}
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
<div class="overlay" class:open={openModal === "resume"} role="dialog" aria-modal="true" tabindex="-1" onclick={onOverlayClick} onkeydown={(e) => { if (e.key === 'Escape') onOverlayClick(); }}>
  <div class="modal" role="presentation">
    <h2><span style="color:var(--primary)">&#9670;</span> Resume Session</h2>
    <div class="field">
      <label for="resume-model">Model</label>
      <select id="resume-model" bind:value={resumeModel}>
        {#each resumeModelList as m (m)}
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
<div class="overlay" class:open={openModal === "delete"} role="dialog" aria-modal="true" tabindex="-1" onclick={onOverlayClick} onkeydown={(e) => { if (e.key === 'Escape') onOverlayClick(); }}>
  <div class="modal" role="presentation">
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

<!-- Wizard -->
{#if showWizard}
<div class="wizard-overlay">
  <div class="wizard">
    <h2>Welcome to VibeCockpit</h2>
    <p>Which AI coding tools would you like to scan? You can change this anytime in Settings.</p>
    <div class="provider-toggles">
      {#each configData.allProviders || [] as p (p.id)}
        {@const off = wizardDisabled.includes(p.id)}
        <label class="provider-toggle" class:disabled={off}>
          <input type="checkbox" checked={!off} onchange={() => wizardToggle(p.id)} />
          <span class="provider-dot" style="background:{providerColors[p.id] || '#888'}"></span>
          {p.name}
        </label>
      {/each}
    </div>
    <div class="wizard-actions">
      <button class="btn" onclick={wizardScanAll}>Scan everything</button>
      <button class="btn btn-primary" onclick={wizardFinish}>Continue</button>
    </div>
  </div>
</div>
{/if}

<!-- Toast Area -->
<div class="toast-area">
  {#each toasts as t (t.id)}
    <div class="toast {t.type}">{t.msg}</div>
  {/each}
</div>
