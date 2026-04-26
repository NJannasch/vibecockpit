/**
 * Check if a character is a separator.
 * @param {string} c
 * @returns {boolean}
 */
export function isSep(c) {
  return " -_/.\\".includes(c);
}

/**
 * Fuzzy-match a pattern against a text string.
 * @param {string} pattern
 * @param {string} text
 * @returns {{ match: boolean, score: number }}
 */
export function fuzzyMatch(pattern, text) {
  if (!pattern) return { match: true, score: 0 };
  pattern = pattern.toLowerCase();
  text = text.toLowerCase();
  if (pattern.length > text.length) return { match: false, score: 0 };

  // Exact substring match — highest score
  const idx = text.indexOf(pattern);
  if (idx >= 0) {
    let score = 100 + pattern.length * 3;
    if (idx === 0) score += 50;
    if (idx > 0 && isSep(text[idx - 1])) score += 30;
    return { match: true, score };
  }

  // Fuzzy character-by-character match
  let pi = 0;
  let score = 0;
  let consec = 0;
  for (let ti = 0; ti < text.length && pi < pattern.length; ti++) {
    if (text[ti] === pattern[pi]) {
      score++;
      consec++;
      if (consec > 1) score += consec;
      if (ti === 0 || isSep(text[ti - 1])) score += 5;
      pi++;
    } else {
      consec = 0;
    }
  }
  if (pi < pattern.length) return { match: false, score: 0 };
  return { match: true, score };
}

/**
 * Match multiple fuzzy terms against multiple text fields.
 * Every term must match at least one field.
 * @param {string[]} terms
 * @param {...string} fields
 * @returns {{ match: boolean, score: number }}
 */
export function fuzzyMatchMulti(terms, ...fields) {
  let total = 0;
  for (const term of terms) {
    let best = 0;
    let matched = false;
    for (const f of fields) {
      const r = fuzzyMatch(term, f || "");
      if (r.match && r.score > best) {
        best = r.score;
        matched = true;
      }
    }
    if (!matched) return { match: false, score: 0 };
    total += best;
  }
  return { match: true, score: total };
}

/**
 * Parse a search input string into fuzzy terms, structured filters, and activeOnly flag.
 * Supports: model:X, branch:X, project:X, provider:X, and the literal "active" keyword.
 * @param {string} input
 * @returns {{ fuzzy: string[], filters: Object, activeOnly: boolean }}
 */
export function parseQuery(input) {
  const parts = input.trim().split(/\s+/).filter(Boolean);
  const q = { fuzzy: [], filters: {}, activeOnly: false };
  for (const p of parts) {
    const lower = p.toLowerCase();
    if (lower === "active") {
      q.activeOnly = true;
      continue;
    }
    const ci = p.indexOf(":");
    if (ci > 0 && ci < p.length - 1) {
      const key = p.slice(0, ci).toLowerCase();
      const val = p.slice(ci + 1).toLowerCase();
      if (["model", "branch", "project", "provider"].includes(key)) {
        q.filters[key] = val;
        continue;
      }
    }
    q.fuzzy.push(lower);
  }
  return q;
}
