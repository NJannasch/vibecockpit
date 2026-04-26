/**
 * Provider brand colors.
 */
export const providerColors = {
  claude: "#7c3aed",
  codex: "#0284c7",
  gemini: "#d97706",
  opencode: "#db2777",
  copilot: "#059669",
};

/**
 * Provider display labels.
 */
export const providerLabels = {
  claude: "Claude Code",
  codex: "Codex CLI",
  copilot: "Copilot CLI",
  opencode: "OpenCode",
  gemini: "Gemini CLI",
};

/**
 * Provider type (for local/remote/agent badges).
 */
export const providerTypes = {
  claude: "cli",
  codex: "cli",
  copilot: "cli",
  opencode: "cli",
  gemini: "cli",
  hermes: "agent",
};

export function getProviderType(name) {
  if (name.startsWith("remote-")) return "remote";
  return providerTypes[name] || "cli";
}

/**
 * Shorten a model identifier for display.
 * Strips the "claude-" prefix and condenses version numbers.
 * @param {string} m — Full model string (e.g. "claude-sonnet-4-20250514")
 * @returns {string} — Shortened form (e.g. "sonnet 4.2")
 */
export function shortModel(m) {
  if (!m) return "-";
  let s = m.replace(/^claude-/, "");
  const p = s.split("-");
  if (p.length >= 3) {
    let minor = p[2];
    if (minor.length > 4) minor = minor[0];
    return p[0] + " " + p[1] + "." + minor;
  }
  return s;
}

/**
 * Convert an ISO timestamp to a human-readable relative time string.
 * @param {string} iso — ISO 8601 date string.
 * @returns {string} — e.g. "just now", "5m ago", "2h ago", "3d ago", or "Apr 12".
 */
export function relativeTime(iso) {
  if (!iso) return "-";
  const d = Date.now() - new Date(iso).getTime();
  if (d < 60000) return "just now";
  if (d < 3600000) return Math.floor(d / 60000) + "m ago";
  if (d < 86400000) return Math.floor(d / 3600000) + "h ago";
  if (d < 604800000) return Math.floor(d / 86400000) + "d ago";
  return new Date(iso).toLocaleDateString("en", { month: "short", day: "numeric" });
}

/**
 * Determine a date-group label for grouping sessions.
 * @param {string} iso — ISO 8601 date string.
 * @returns {string} — "Today", "Yesterday", "This week", "This month", or "Older".
 */
export function dateGroup(iso) {
  if (!iso) return "Unknown";
  const d = Date.now() - new Date(iso).getTime();
  if (d < 86400000) return "Today";
  if (d < 172800000) return "Yesterday";
  if (d < 604800000) return "This week";
  if (d < 2592000000) return "This month";
  return "Older";
}

/**
 * Compute dashboard status data from a flat list of sessions.
 * Groups sessions by provider, counts totals/active, finds last activity,
 * and returns the 8 most recent sessions across all providers.
 *
 * @param {Array<Object>} sessions — Array of session objects from the API.
 * @returns {{ providers: Array, totalSessions: number, totalActive: number, recentSessions: Array }}
 */
export function computeDashboardData(sessions) {
  const providers = {};
  for (const s of sessions) {
    if (!providers[s.provider]) {
      providers[s.provider] = { id: s.provider, sessions: 0, active: 0, lastActivity: null };
    }
    const p = providers[s.provider];
    p.sessions++;
    if (s.isActive) p.active++;
    const mod = new Date(s.modified);
    if (!p.lastActivity || mod > p.lastActivity) p.lastActivity = mod;
  }

  const sorted = [...sessions].sort((a, b) => new Date(b.modified) - new Date(a.modified));
  return {
    providers: Object.values(providers),
    totalSessions: sessions.length,
    totalActive: sessions.filter(s => s.isActive).length,
    recentSessions: sorted.slice(0, 8),
  };
}
