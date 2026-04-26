import { writable, derived } from "svelte/store";
import { fetchSessions, fetchConfig } from "./api.js";
import { parseQuery, fuzzyMatchMulti } from "./search.js";

// ─── Writable Stores ───

/** @type {import('svelte/store').Writable<Array>} */
export const sessions = writable([]);

/** @type {import('svelte/store').Writable<Object>} */
export const config = writable({});

/** @type {import('svelte/store').Writable<Object>} Active chip filters (e.g. { provider: "claude", active: true }) */
export const activeFilters = writable({});

/** @type {import('svelte/store').Writable<"list"|"tree">} */
export const currentView = writable("list");

/** @type {import('svelte/store').Writable<string>} */
export const searchQuery = writable("");

/** @type {import('svelte/store').Writable<string>} */
export const sortBy = writable("modified");

/** @type {import('svelte/store').Writable<string>} */
export const groupBy = writable("none");

// ─── Derived: filteredSessions ───

/**
 * A derived store that applies fuzzy search, structured filters, chip filters,
 * and sorting to the raw sessions list. Mirrors the original render() logic exactly.
 *
 * Returns: { results: Array<{s, score}>, filtered: Array, activeCount: number }
 */
export const filteredSessions = derived(
  [sessions, searchQuery, activeFilters, sortBy],
  ([$sessions, $searchQuery, $activeFilters, $sortBy]) => {
    if ($sessions.length === 0) {
      return { results: [], filtered: [], activeCount: 0 };
    }

    const q = parseQuery($searchQuery);

    // Merge chip filters into query
    if ($activeFilters.active) q.activeOnly = true;
    for (const [k, v] of Object.entries($activeFilters)) {
      if (k !== "active" && !q.filters[k]) q.filters[k] = v;
    }

    const isEmpty =
      q.fuzzy.length === 0 &&
      Object.keys(q.filters).length === 0 &&
      !q.activeOnly;

    let results = [];

    for (const s of $sessions) {
      // Active-only filter
      if (q.activeOnly && !s.isActive) continue;

      // Structured filters
      let filterOk = true;
      for (const [key, val] of Object.entries(q.filters)) {
        const field =
          key === "model"
            ? s.model
            : key === "branch"
              ? s.gitBranch
              : key === "project"
                ? s.projectName
                : s.provider;
        if (!(field || "").toLowerCase().includes(val)) {
          filterOk = false;
          break;
        }
      }
      if (!filterOk) continue;

      // Fuzzy matching
      if (q.fuzzy.length === 0) {
        results.push({ s, score: 0 });
        continue;
      }
      const summary = s.summary || s.firstPrompt || "";
      const r = fuzzyMatchMulti(
        q.fuzzy,
        s.projectName,
        summary,
        s.gitBranch,
        s.model,
        s.projectPath,
      );
      if (r.match) results.push({ s, score: r.score });
    }

    // Sort
    results.sort((a, b) => {
      if (!isEmpty && a.score !== b.score) return b.score - a.score;
      switch ($sortBy) {
        case "created":
          return new Date(b.s.created) - new Date(a.s.created);
        case "name":
          return (a.s.projectName || "").localeCompare(b.s.projectName || "");
        case "messages":
          return (b.s.messageCount || 0) - (a.s.messageCount || 0);
        default:
          return new Date(b.s.modified) - new Date(a.s.modified);
      }
    });

    const filtered = results.map((r) => r.s);
    const activeCount = filtered.filter((s) => s.isActive).length;

    return { results, filtered, activeCount, fuzzyTerms: q.fuzzy };
  },
);

// ─── Data Loaders ───

/**
 * Fetch sessions from the API and update the sessions store.
 */
export async function loadSessions(forceRefresh = false) {
  try {
    const data = await fetchSessions(forceRefresh);
    sessions.set(data);
  } catch (e) {
    console.error("Failed to load sessions:", e);
  }
}

/**
 * Fetch config from the API and update the config store.
 * Also restores persisted sortBy/groupBy preferences.
 */
export async function loadConfig() {
  try {
    const data = await fetchConfig();
    config.set(data);
    if (data.sortBy) sortBy.set(data.sortBy);
    if (data.groupBy) groupBy.set(data.groupBy);
  } catch (e) {
    console.error("Failed to load config:", e);
  }
}

// ─── Auto-Refresh ───

let refreshInterval = null;

/**
 * Start auto-refreshing sessions every 30 seconds.
 */
export function startAutoRefresh() {
  stopAutoRefresh();
  refreshInterval = setInterval(loadSessions, 30000);
}

/**
 * Stop auto-refresh.
 */
export function stopAutoRefresh() {
  if (refreshInterval) {
    clearInterval(refreshInterval);
    refreshInterval = null;
  }
}
