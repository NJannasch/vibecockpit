<script>
  import { providerColors, providerLabels } from "../lib/utils.js";

  let { sessions } = $props();

  let tooltip = $state(null); // { x, y, content }

  let metric = $state("cost");
  let timeView = $state("weekly"); // daily, weekly, monthly
  let groupBy = $state("provider");
  let filterProvider = $state("all");
  let showAllRecs = $state(false);
  let showPricing = $state(false);

  const metrics = [
    { id: "cost", label: "Est. Cost ($)", fmt: v => "$" + v.toFixed(2) },
    { id: "activity", label: "Sessions", fmt: v => v.toFixed(0) },
    { id: "messages", label: "Messages", fmt: v => fmtNum(v) },
    { id: "tokensIn", label: "Tokens In", fmt: v => fmtNum(v) },
    { id: "tokensOut", label: "Tokens Out", fmt: v => fmtNum(v) },
    { id: "tokensTotal", label: "Total Tokens", fmt: v => fmtNum(v) },
  ];

  function fmtNum(v) {
    if (v >= 1_000_000) return (v / 1_000_000).toFixed(1) + "M";
    if (v >= 1_000) return (v / 1_000).toFixed(1) + "K";
    return v.toFixed(0);
  }

  function fmtMetric(v) {
    return metrics.find(m => m.id === metric)?.fmt(v) || v.toFixed(0);
  }

  function getValue(s) {
    switch (metric) {
      case "cost": return s.estCostUsd || 0;
      case "activity": return 1;
      case "messages": return s.messageCount || 0;
      case "tokensIn": return s.tokens?.inputTokens || 0;
      case "tokensOut": return s.tokens?.outputTokens || 0;
      case "tokensTotal": return s.tokens?.totalTokens || 0;
      default: return 0;
    }
  }

  let filtered = $derived(
    filterProvider === "all" ? sessions : sessions.filter(s => s.provider === filterProvider)
  );

  let providers = $derived([...new Set(sessions.map(s => s.provider))]);
  let hasRemote = $derived(sessions.some(s => s.provider?.startsWith("remote-")));

  // ─── Heatmap (last 365 days, newest on the right) ───
  let heatmap = $derived.by(() => {
    const today = new Date();
    today.setHours(0, 0, 0, 0);

    // Build a map of date → value
    const dayValues = {};
    for (const s of filtered) {
      if (!s.modified) continue;
      const day = s.modified.slice(0, 10);
      dayValues[day] = (dayValues[day] || 0) + getValue(s);
    }

    // Build 53 weeks × 7 days grid, ending at today
    const dayOfWeek = today.getDay(); // 0=Sun
    const weeks = [];
    const months = [];
    let lastMonth = -1;

    for (let w = 52; w >= 0; w--) {
      const week = [];
      for (let d = 0; d < 7; d++) {
        const daysAgo = w * 7 + (dayOfWeek - d);
        const date = new Date(today);
        date.setDate(date.getDate() - daysAgo);
        const key = date.toISOString().slice(0, 10);
        const mon = date.getMonth();
        if (mon !== lastMonth) {
          months.push({ week: 52 - w, label: date.toLocaleString("en", { month: "short" }) });
          lastMonth = mon;
        }
        week.push({ date: key, value: dayValues[key] || 0 });
      }
      weeks.push(week);
    }

    const max = Math.max(...Object.values(dayValues), 1);
    return { weeks, months, max };
  });

  function heatColor(val, max) {
    if (val === 0) return "var(--surface-active)";
    const t = Math.min(val / max, 1);
    const alpha = 0.2 + t * 0.8;
    return `rgba(124, 58, 237, ${alpha})`;
  }

  // ─── Bar chart (bucketed by day/week) ───
  let chart = $derived.by(() => {
    const bucketDays = timeView === "daily" ? 1 : timeView === "weekly" ? 7 : 30;
    const rangeDays = timeView === "daily" ? 30 : timeView === "weekly" ? 90 : 365;

    const now = new Date();
    const since = new Date(now);
    since.setDate(since.getDate() - rangeDays);
    since.setHours(0, 0, 0, 0);

    // Collect data per bucket per group
    const bucketMap = {};
    const groupSet = new Set();

    for (const s of filtered) {
      if (!s.modified) continue;
      const d = new Date(s.modified);
      if (d < since) continue;

      const daysSince = Math.floor((d - since) / 86400000);
      const bucketIdx = Math.floor(daysSince / bucketDays);
      const bucketStart = new Date(since);
      bucketStart.setDate(bucketStart.getDate() + bucketIdx * bucketDays);
      const key = bucketStart.toISOString().slice(0, 10);

      if (!bucketMap[key]) bucketMap[key] = {};
      const group = groupBy === "provider" ? s.provider : s.projectName;
      groupSet.add(group);
      bucketMap[key][group] = (bucketMap[key][group] || 0) + getValue(s);
    }

    // Fill empty buckets
    const totalBuckets = Math.ceil(rangeDays / bucketDays);
    const dates = [];
    for (let i = 0; i < totalBuckets; i++) {
      const d = new Date(since);
      d.setDate(d.getDate() + i * bucketDays);
      const key = d.toISOString().slice(0, 10);
      dates.push(key);
      if (!bucketMap[key]) bucketMap[key] = {};
    }

    const groups = [...groupSet];
    const max = Math.max(...dates.map(d => Object.values(bucketMap[d]).reduce((a, b) => a + b, 0)), 1);

    return { dates, groups, bucketMap, max, bucketDays };
  });

  // ─── Summary ───
  let summary = $derived.by(() => {
    let cost = 0, tokIn = 0, tokOut = 0, msgs = 0;
    for (const s of filtered) {
      cost += s.estCostUsd || 0;
      tokIn += s.tokens?.inputTokens || 0;
      tokOut += s.tokens?.outputTokens || 0;
      msgs += s.messageCount || 0;
    }
    return { cost, tokIn, tokOut, msgs, sessions: filtered.length };
  });

  const dayLabels = ["S", "M", "T", "W", "T", "F", "S"];

  // Stable colors for projects (hashed from name)
  const projectColorPalette = [
    "#7c3aed", "#0284c7", "#d97706", "#db2777", "#059669",
    "#6366f1", "#dc2626", "#0891b2", "#65a30d", "#c026d3",
    "#ea580c", "#2563eb", "#4f46e5", "#0d9488", "#b91c1c",
  ];
  const projectColorCache = {};
  function projectColor(name) {
    if (projectColorCache[name]) return projectColorCache[name];
    let hash = 0;
    for (let i = 0; i < name.length; i++) hash = ((hash << 5) - hash + name.charCodeAt(i)) | 0;
    const color = projectColorPalette[Math.abs(hash) % projectColorPalette.length];
    projectColorCache[name] = color;
    return color;
  }

  function groupColor(group) {
    return providerColors[group] || projectColor(group);
  }

  // Hero = the single biggest actionable saving. Prefer plan recs over model mix.
  let heroRec = $derived.by(() => {
    const saves = recommendations.filter(r => r.type === "save" && r.savings > 50);
    if (saves.length === 0) return null;
    // Prefer plan recs (have .plan) over model mix, then by savings
    const planRecs = saves.filter(r => r.plan);
    return planRecs.length > 0 ? planRecs[0] : saves[0];
  });
  let otherRecs = $derived(heroRec ? recommendations.filter(r => r !== heroRec) : recommendations);
  let visibleRecs = $derived(showAllRecs ? otherRecs : otherRecs.slice(0, 3));
  let hiddenCount = $derived(Math.max(0, otherRecs.length - 3));

  // ─── Plan Recommendations ───
  // Subscription plans with estimated API-equivalent value (April 2026).
  // Sources: openai.com, anthropic.com, gemini.google, github.com/features/copilot
  const plans = {
    claude: [
      { name: "Pro", price: 20, estValue: 300, desc: "~45min Opus or ~5h Sonnet/day" },
      { name: "Max 5x", price: 100, estValue: 1500, desc: "5x Pro limits, ~3.5h Opus/day" },
      { name: "Max 20x", price: 200, estValue: 6000, desc: "20x Pro limits, ~14h Opus/day" },
    ],
    codex: [
      { name: "ChatGPT Plus", price: 20, estValue: 200, desc: "600 local msgs / 200 cloud tasks per 5h" },
      { name: "ChatGPT Pro", price: 100, estValue: 1000, desc: "5x Plus, 3000 msgs / 1200 tasks per 5h" },
      { name: "ChatGPT Pro 20x", price: 200, estValue: 4000, desc: "20x Plus limits" },
    ],
    copilot: [
      { name: "Individual", price: 10, estValue: 50, desc: "Flat rate, rate limited" },
      { name: "Business", price: 19, estValue: 100, desc: "Per user, higher limits" },
    ],
    gemini: [
      { name: "Free tier", price: 0, estValue: 30, desc: "60 req/min, light use" },
      { name: "Google AI Pro", price: 20, estValue: 300, desc: "Higher CLI limits + $10 Cloud credits" },
      { name: "Google AI Ultra", price: 250, estValue: 3000, desc: "Highest limits + $100 Cloud credits" },
    ],
  };

  let recommendations = $derived.by(() => {
    const recs = [];
    const now = new Date();
    const thirtyDaysAgo = new Date(now); thirtyDaysAgo.setDate(thirtyDaysAgo.getDate() - 30);
    const sevenDaysAgo = new Date(now); sevenDaysAgo.setDate(sevenDaysAgo.getDate() - 7);
    const prevWeekStart = new Date(now); prevWeekStart.setDate(prevWeekStart.getDate() - 14);

    // Collect per-provider, per-project, per-model, per-day data
    const byProvider = {};
    const byProject = {};
    const byModel = {};
    const dailyCosts = {};
    const thisWeekCost = {};
    const lastWeekCost = {};

    for (const s of sessions) {
      if (!s.modified) continue;
      const d = new Date(s.modified);
      const prov = s.provider;
      const provLabel = providerLabels[prov] || prov;

      // Week-over-week
      if (d >= sevenDaysAgo) thisWeekCost[prov] = (thisWeekCost[prov] || 0) + (s.estCostUsd || 0);
      else if (d >= prevWeekStart) lastWeekCost[prov] = (lastWeekCost[prov] || 0) + (s.estCostUsd || 0);

      if (d < thirtyDaysAgo) continue;

      // By provider
      if (!byProvider[prov]) byProvider[prov] = { cost: 0, sessions: 0, tokens: 0, days: new Set() };
      byProvider[prov].cost += s.estCostUsd || 0;
      byProvider[prov].sessions++;
      byProvider[prov].tokens += s.tokens?.totalTokens || 0;
      const day = s.modified.slice(0, 10);
      byProvider[prov].days.add(day);

      // By project
      const proj = s.projectName || "unknown";
      if (!byProject[proj]) byProject[proj] = { cost: 0, sessions: 0, provider: prov };
      byProject[proj].cost += s.estCostUsd || 0;
      byProject[proj].sessions++;

      // By model
      const model = s.model || "unknown";
      if (!byModel[model]) byModel[model] = { cost: 0, sessions: 0, provider: prov };
      byModel[model].cost += s.estCostUsd || 0;
      byModel[model].sessions++;

      // Daily costs
      const key = prov + ":" + day;
      dailyCosts[key] = (dailyCosts[key] || 0) + (s.estCostUsd || 0);
    }

    // ─── 1. Plan recommendations (with data basis) ───
    for (const [prov, data] of Object.entries(byProvider)) {
      const provPlans = plans[prov];
      if (!provPlans) continue;

      const monthlyCost = data.cost;
      const activeDays = data.days.size;
      const avgDaily = activeDays > 0 ? monthlyCost / activeDays : 0;
      const provLabel = providerLabels[prov] || prov;
      const basis = `Based on ${data.sessions} sessions across ${activeDays} active days, ${fmtNum(data.tokens)} tokens.`;

      let peakDay = 0;
      for (const [key, val] of Object.entries(dailyCosts)) {
        if (key.startsWith(prov + ":") && val > peakDay) peakDay = val;
      }

      if (monthlyCost === 0 && data.sessions > 0) {
        if (data.sessions <= 3 && provPlans[0]?.price > 0) {
          recs.push({ type: "downsize", provider: prov, basis,
            text: `${provLabel}: Only ${data.sessions} sessions this month. The $${provPlans[0].price}/mo subscription may not be worth it.` });
        }
        continue;
      }
      if (monthlyCost < 1) continue;

      const sortedPlans = [...provPlans].sort((a, b) => a.price - b.price);
      const coveringPlan = sortedPlans.find(p => p.estValue >= monthlyCost);
      const biggestPlan = sortedPlans[sortedPlans.length - 1];

      if (coveringPlan) {
        const savings = monthlyCost - coveringPlan.price;
        if (savings > 5) {
          const cheaperOption = sortedPlans.find(p => p.estValue >= monthlyCost * 0.8 && p.price < coveringPlan.price);
          recs.push({ type: "save", provider: prov, savings, plan: coveringPlan, basis,
            text: `${provLabel}: ~$${monthlyCost.toFixed(0)}/mo API-equivalent. ${coveringPlan.name} ($${coveringPlan.price}/mo, ~$${coveringPlan.estValue.toLocaleString()} value) covers this.` });
          if (cheaperOption && cheaperOption.name !== coveringPlan.name) {
            recs.push({ type: "insight", provider: prov, basis,
              text: `${provLabel}: ${cheaperOption.name} ($${cheaperOption.price}/mo) might also work — you'd use ${Math.round(monthlyCost / cheaperOption.estValue * 100)}% of capacity. Saves $${coveringPlan.price - cheaperOption.price}/mo but risk hitting limits on peak days.` });
          }
        }
      } else if (monthlyCost > biggestPlan.estValue) {
        recs.push({ type: "insight", provider: prov, basis,
          text: `${provLabel}: ~$${monthlyCost.toFixed(0)}/mo exceeds even ${biggestPlan.name} (~$${biggestPlan.estValue.toLocaleString()} value). Consider spreading across cheaper models.` });
      }

      if (monthlyCost < sortedPlans[0].estValue * 0.2 && sortedPlans[0].price > 0) {
        recs.push({ type: "downsize", provider: prov, basis,
          text: `${provLabel}: Usage ($${monthlyCost.toFixed(0)}/mo) is only ${Math.round(monthlyCost / sortedPlans[0].estValue * 100)}% of the cheapest plan's capacity. Pay-as-you-go may be cheaper.` });
      }

      if (peakDay > avgDaily * 3 && activeDays >= 5) {
        const peakPct = coveringPlan ? Math.round(peakDay / (coveringPlan.estValue / 30) * 100) : 0;
        recs.push({ type: "insight", provider: prov, basis,
          text: `${provLabel}: Peak day $${peakDay.toFixed(0)} vs avg $${avgDaily.toFixed(0)}/day. ${peakPct > 100 ? "Peak days exceed daily plan budget — expect rate limiting." : "Spiky but within plan limits."}` });
      }
    }

    // ─── 2. Model mix insight ───
    const modelEntries = Object.entries(byModel).filter(([,v]) => v.cost > 1).sort((a, b) => b[1].cost - a[1].cost);
    if (modelEntries.length >= 2) {
      const [topModel, topData] = modelEntries[0];
      const totalModelCost = modelEntries.reduce((a, [,v]) => a + v.cost, 0);
      const topPct = Math.round(topData.cost / totalModelCost * 100);
      if (topPct > 60) {
        const isExpensive = topModel.includes("opus") || topModel.includes("o3") || topModel.includes("5.4");
        if (isExpensive) {
          const cheaperModels = modelEntries.filter(([m]) => m.includes("sonnet") || m.includes("haiku") || m.includes("mini") || m.includes("flash"));
          const suggestion = cheaperModels.length > 0 ? cheaperModels[0][0] : "a lighter model";
          recs.push({ type: "save", provider: topData.provider, savings: topData.cost * 0.4,
            text: `Model mix: ${topPct}% of spend is ${topModel} ($${topData.cost.toFixed(0)}). Switching routine tasks to ${suggestion} could save ~$${(topData.cost * 0.4).toFixed(0)}/mo.`,
            basis: `Based on ${modelEntries.length} models used across ${modelEntries.reduce((a,[,v]) => a+v.sessions, 0)} sessions.` });
        }
      }
    }

    // ─── 3. Top projects by cost ───
    const topProjects = Object.entries(byProject).filter(([,v]) => v.cost > 1).sort((a, b) => b[1].cost - a[1].cost).slice(0, 5);
    if (topProjects.length >= 2) {
      const lines = topProjects.map(([name, v]) => `${name} ($${v.cost.toFixed(0)}, ${v.sessions} sessions)`).join(", ");
      recs.push({ type: "insight", provider: "",
        text: `Top projects by cost: ${lines}`,
        basis: `${Object.keys(byProject).length} projects in last 30 days.` });
    }

    // ─── 4. Unused / underused tools (one per provider, not per session) ───
    const unusedProviders = new Set();
    for (const s of sessions) {
      if (!s.provider || s.provider.startsWith("remote-")) continue;
      const prov = s.provider;
      if (byProvider[prov] || unusedProviders.has(prov)) continue;
      unusedProviders.add(prov);
      const lastUsed = new Date(s.modified);
      const daysAgo = Math.round((now - lastUsed) / 86400000);
      if (daysAgo > 30) {
        const provLabel = providerLabels[prov] || prov;
        const totalSessions = sessions.filter(x => x.provider === prov).length;
        recs.push({ type: "downsize", provider: prov,
          text: `${provLabel}: No activity in ${daysAgo} days (${totalSessions} total sessions). Consider whether you still need this tool.`,
          basis: `Last session: ${s.modified?.slice(0, 10)}.` });
      }
    }

    for (const [prov, data] of Object.entries(byProvider)) {
      if (data.sessions <= 2 && data.sessions > 0 && !unusedProviders.has(prov)) {
        const provLabel = providerLabels[prov] || prov;
        recs.push({ type: "downsize", provider: prov,
          text: `${provLabel}: Only ${data.sessions} session${data.sessions > 1 ? "s" : ""} this month — is this tool earning its keep?`,
          basis: `${data.sessions} sessions in last 30 days.` });
      }
    }

    // ─── 5. Week-over-week trend ───
    for (const [prov, thisWeek] of Object.entries(thisWeekCost)) {
      const lastWeek = lastWeekCost[prov] || 0;
      if (lastWeek < 1 && thisWeek < 1) continue;
      const provLabel = providerLabels[prov] || prov;
      if (lastWeek > 0) {
        const change = ((thisWeek - lastWeek) / lastWeek) * 100;
        if (Math.abs(change) > 25) {
          const dir = change > 0 ? "up" : "down";
          const icon = change > 0 ? "↑" : "↓";
          recs.push({ type: change > 0 ? "insight" : "save", provider: prov,
            text: `${provLabel}: Spend is ${icon} ${Math.abs(change).toFixed(0)}% vs last week ($${thisWeek.toFixed(0)} vs $${lastWeek.toFixed(0)}).`,
            basis: `Comparing last 7 days vs prior 7 days.` });
        }
      } else if (thisWeek > 10) {
        recs.push({ type: "insight", provider: prov,
          text: `${provLabel}: $${thisWeek.toFixed(0)} this week — no usage the prior week. New activity.`,
          basis: `Comparing last 7 days vs prior 7 days.` });
      }
    }

    return recs.sort((a, b) => (b.savings || 0) - (a.savings || 0));
  });

  function showTooltip(e, content) {
    tooltip = {
      x: e.clientX + 12,
      y: e.clientY - 8,
      content
    };
  }
  function hideTooltip() { tooltip = null; }

  function heatmapTooltip(e, day) {
    const daySessions = filtered.filter(s => s.modified?.slice(0, 10) === day.date);
    if (daySessions.length === 0 && day.value === 0) {
      showTooltip(e, `<b>${day.date}</b><br>No activity`);
      return;
    }
    const lines = [`<b>${day.date}</b>`];
    const byProv = {};
    for (const s of daySessions) {
      const p = s.provider;
      if (!byProv[p]) byProv[p] = { count: 0, cost: 0, tokens: 0 };
      byProv[p].count++;
      byProv[p].cost += s.estCostUsd || 0;
      byProv[p].tokens += s.tokens?.totalTokens || 0;
    }
    for (const [p, v] of Object.entries(byProv)) {
      lines.push(`<span style="color:${providerColors[p] || 'var(--text)'}">●</span> ${providerLabels[p] || p}: ${v.count} sessions, $${v.cost.toFixed(2)}, ${fmtNum(v.tokens)} tokens`);
    }
    lines.push(`<b>Total: ${fmtMetric(day.value)}</b>`);
    showTooltip(e, lines.join("<br>"));
  }

  function barTooltip(e, date, bucket) {
    const entries = Object.entries(bucket);
    const total = entries.reduce((a, [,v]) => a + v, 0);
    const lines = [`<b>${date}</b>`];
    for (const [group, val] of entries.sort((a, b) => b[1] - a[1])) {
      const color = groupColor(group);
      lines.push(`<span style="color:${color}">●</span> ${providerLabels[group] || group}: ${fmtMetric(val)}`);
    }
    lines.push(`<b>Total: ${fmtMetric(total)}</b>`);
    showTooltip(e, lines.join("<br>"));
  }
</script>

<div class="costs">
  <!-- Hero -->
  {#if heroRec}
    <div class="costs-hero">
      <span class="costs-hero-icon">&#9660;</span>
      <div class="costs-hero-body">
        <span class="costs-hero-text">{heroRec.text}</span>
        {#if heroRec.basis}<span class="costs-hero-basis">{heroRec.basis}</span>{/if}
      </div>
      {#if heroRec.savings > 0}
        <span class="costs-hero-savings">Save ~${heroRec.savings.toFixed(0)}/mo</span>
      {/if}
    </div>
  {/if}

  <!-- Click a stat to change what the heatmap and chart show -->
  <div class="costs-summary">
    <button class="costs-stat" class:active={metric === "cost"} onclick={() => metric = "cost"}>
      <span class="costs-stat-value">${summary.cost.toFixed(2)}</span>
      <span class="costs-stat-label">Est. Cost</span>
    </button>
    <button class="costs-stat" class:active={metric === "tokensIn"} onclick={() => metric = "tokensIn"}>
      <span class="costs-stat-value">{fmtNum(summary.tokIn)}</span>
      <span class="costs-stat-label">Tokens In</span>
    </button>
    <button class="costs-stat" class:active={metric === "tokensOut"} onclick={() => metric = "tokensOut"}>
      <span class="costs-stat-value">{fmtNum(summary.tokOut)}</span>
      <span class="costs-stat-label">Tokens Out</span>
    </button>
    <button class="costs-stat" class:active={metric === "messages"} onclick={() => metric = "messages"}>
      <span class="costs-stat-value">{summary.msgs.toLocaleString()}</span>
      <span class="costs-stat-label">Messages</span>
    </button>
    <button class="costs-stat" class:active={metric === "activity"} onclick={() => metric = "activity"}>
      <span class="costs-stat-value">{summary.sessions}</span>
      <span class="costs-stat-label">Sessions</span>
    </button>
  </div>

  {#if hasRemote}
    <div class="costs-info">
      Cost data is not yet available for remote sessions (SSH). Remote sessions are shown in the session list but excluded from cost calculations.
    </div>
  {/if}

  <!-- Controls -->
  <div class="costs-toolbar">
    <div class="costs-toggle">
      <button class:active={timeView === "daily"} onclick={() => timeView = "daily"}>Daily</button>
      <button class:active={timeView === "weekly"} onclick={() => timeView = "weekly"}>Weekly</button>
      <button class:active={timeView === "monthly"} onclick={() => timeView = "monthly"}>Monthly</button>
    </div>
    <div class="costs-toggle">
      <button class:active={groupBy === "provider"} onclick={() => groupBy = "provider"}>By tool</button>
      <button class:active={groupBy === "project"} onclick={() => groupBy = "project"}>By project</button>
    </div>
    <select class="costs-select" bind:value={filterProvider}>
      <option value="all">All providers</option>
      {#each providers as p}
        <option value={p}>{providerLabels[p] || p}</option>
      {/each}
    </select>
  </div>

  <!-- Heatmap -->
  <div class="costs-card">
    <div class="costs-card-header">
      <h3>Activity Heatmap</h3>
      <span class="costs-card-sub">{metrics.find(m => m.id === metric)?.label} — last 12 months</span>
    </div>
    <div class="heatmap-scroll">
      <div class="heatmap-grid">
        <!-- Day labels -->
        <div class="heatmap-day-labels">
          {#each dayLabels as label, i}
            <span class="heatmap-day-label" style="grid-row:{i + 1}">{i % 2 === 1 ? label : ""}</span>
          {/each}
        </div>
        <!-- Cells -->
        <svg viewBox="0 0 {heatmap.weeks.length * 15} 105" class="heatmap-svg">
          <!-- Month labels -->
          {#each heatmap.months as m}
            <text x={m.week * 15} y="-2" font-size="9" fill="var(--text-secondary)" font-family="system-ui,sans-serif">{m.label}</text>
          {/each}
          {#each heatmap.weeks as week, wi}
            {#each week as day, di}
              <rect
                x={wi * 15}
                y={di * 15}
                width="12"
                height="12"
                rx="2"
                fill={heatColor(day.value, heatmap.max)}
                style="cursor:pointer"
                onmouseenter={(e) => heatmapTooltip(e, day)}
                onmouseleave={hideTooltip}
              />
            {/each}
          {/each}
        </svg>
      </div>
      <div class="heatmap-legend">
        <span>Less</span>
        {#each [0, 0.25, 0.5, 0.75, 1] as t}
          <span class="heatmap-legend-box" style="background:rgba(124,58,237,{0.2 + t * 0.8})"></span>
        {/each}
        <span>More</span>
      </div>
    </div>
  </div>

  <!-- Chart -->
  <div class="costs-card">
    <div class="costs-card-header">
      <h3>Trend</h3>
      <span class="costs-card-sub">{metrics.find(m => m.id === metric)?.label} — {timeView}, by {groupBy}</span>
    </div>
    <div class="chart-container">
      <!-- Y axis labels -->
      <div class="chart-y-axis">
        <span>{fmtMetric(chart.max)}</span>
        <span>{fmtMetric(chart.max / 2)}</span>
        <span>0</span>
      </div>
      <div class="chart-area">
      {#if true}
      {@const barSpacing = 24}
      {@const chartW = chart.dates.length * barSpacing + 4}
      {@const chartH = 150}
      {@const labelH = 40}
        <svg viewBox="0 0 {chartW} {chartH + labelH}" class="chart-svg" style="height:{chartH + labelH}px;min-width:{chartW}px">
          <!-- Grid lines -->
          <line x1="0" y1="5" x2={chartW} y2="5" stroke="var(--border)" stroke-width=".3" stroke-dasharray="2 4"/>
          <line x1="0" y1={chartH / 2} x2={chartW} y2={chartH / 2} stroke="var(--border)" stroke-width=".3" stroke-dasharray="2 4"/>
          <line x1="0" y1={chartH - 2} x2={chartW} y2={chartH - 2} stroke="var(--border)" stroke-width=".5"/>
          {#each chart.dates as date, i}
            {@const bucket = chart.bucketMap[date] || {}}
            {@const entries = Object.entries(bucket)}
            {@const total = entries.reduce((a, [,v]) => a + v, 0)}
            {@const barW = barSpacing * 0.7}
            {@const x = i * barSpacing + 4}

            <!-- Stacked segments -->
            {#each entries as [group, val], gi}
              {@const segH = (val / chart.max) * (chartH - 10)}
              {@const prevH = entries.slice(0, gi).reduce((a, [,v]) => a + (v / chart.max) * (chartH - 10), 0)}
              <rect
                x={x}
                y={chartH - 2 - prevH - segH}
                width={barW}
                height={Math.max(segH, 1)}
                rx="2"
                fill={groupColor(group)}
                opacity="0.85"
                style="cursor:pointer"
                onmouseenter={(e) => barTooltip(e, date, bucket)}
                onmouseleave={hideTooltip}
              />
            {/each}

            <!-- Total label on top -->
            {#if total > 0 && chart.dates.length <= 14}
              <text
                x={x + barW / 2}
                y={Math.max(chartH - 2 - (total / chart.max) * (chartH - 10) - 4, 10)}
                text-anchor="middle"
                font-size="7"
                fill="var(--text-secondary)"
                font-family="system-ui,sans-serif"
              >{fmtMetric(total)}</text>
            {/if}

            <!-- X axis label (angled, below bar) -->
            {#if i % Math.max(1, Math.ceil(chart.dates.length / 15)) === 0}
              <text
                x={x + barW / 2}
                y={chartH + 6}
                font-size="8"
                fill="var(--text-secondary)"
                font-family="system-ui,sans-serif"
                text-anchor="end"
                transform="rotate(-45, {x + barW / 2}, {chartH + 6})"
              >{date.slice(5)}</text>
            {/if}
          {/each}
        </svg>
      {/if}
      </div>
    </div>
    <!-- Legend -->
    {#if chart.groups.length > 0}
      <div class="chart-legend">
        {#each chart.groups.slice(0, 10) as group}
          <span class="chart-legend-item">
            <span class="chart-legend-dot" style="background:{groupColor(group)}"></span>
            {providerLabels[group] || group}
          </span>
        {/each}
      </div>
    {/if}
  </div>
  <!-- Recommendations -->
  {#if recommendations.length > 0}
    <div class="costs-card">
      <div class="costs-card-header">
        <h3>All Insights</h3>
        <span class="costs-card-sub">{otherRecs.length} insights based on last 30 days</span>
      </div>
      <div class="recs-list">
        {#each visibleRecs as rec}
          <div class="rec rec-{rec.type}">
            <span class="rec-icon">
              {#if rec.type === "save"}&#9660;{:else if rec.type === "downsize"}&#9664;{:else}&#9679;{/if}
            </span>
            <div class="rec-body">
              <span class="rec-text">{rec.text}</span>
              {#if rec.basis}
                <span class="rec-basis">{rec.basis}</span>
              {/if}
            </div>
            {#if rec.savings > 0}
              <span class="rec-savings">-${rec.savings.toFixed(0)}/mo</span>
            {/if}
          </div>
        {/each}
      </div>
      {#if hiddenCount > 0}
        <button class="recs-toggle" onclick={() => showAllRecs = !showAllRecs}>
          {showAllRecs ? "Show less" : `Show ${hiddenCount} more`}
        </button>
      {/if}
    </div>
  {/if}

  <!-- Pricing Reference (collapsible) -->
  <button class="pricing-toggle" onclick={() => showPricing = !showPricing}>
    {showPricing ? "▼" : "▶"} Pricing assumptions
    <span class="costs-card-sub" style="margin-left:.4rem">per million tokens — estimates, may vary</span>
  </button>
  {#if showPricing}
  <div class="costs-card" style="margin-top:0;border-top-left-radius:0;border-top-right-radius:0">
    <div class="pricing-tables">
      <div class="pricing-group">
        <h4>Anthropic</h4>
        <table class="pricing-table">
          <thead><tr><th>Model</th><th>Input</th><th>Output</th><th>Cache Read</th></tr></thead>
          <tbody>
            <tr><td>Opus 4.6 / 4.7</td><td>$15.00</td><td>$75.00</td><td>$1.50</td></tr>
            <tr><td>Sonnet 4.6</td><td>$3.00</td><td>$15.00</td><td>$0.30</td></tr>
            <tr><td>Haiku 4.5</td><td>$0.80</td><td>$4.00</td><td>$0.08</td></tr>
          </tbody>
        </table>
      </div>
      <div class="pricing-group">
        <h4>OpenAI</h4>
        <table class="pricing-table">
          <thead><tr><th>Model</th><th>Input</th><th>Output</th><th></th></tr></thead>
          <tbody>
            <tr><td>GPT-5.4</td><td>$2.50</td><td>$10.00</td><td></td></tr>
            <tr><td>GPT-5-mini</td><td>$0.15</td><td>$0.60</td><td></td></tr>
            <tr><td>o3-pro</td><td>$20.00</td><td>$80.00</td><td></td></tr>
          </tbody>
        </table>
      </div>
      <div class="pricing-group">
        <h4>Google</h4>
        <table class="pricing-table">
          <thead><tr><th>Model</th><th>Input</th><th>Output</th><th></th></tr></thead>
          <tbody>
            <tr><td>Gemini 3 Pro</td><td>$1.25</td><td>$5.00</td><td></td></tr>
            <tr><td>Gemini 3 Flash</td><td>$0.075</td><td>$0.30</td><td></td></tr>
          </tbody>
        </table>
      </div>
    </div>
    <div class="plans-note">
      <h4>Subscription plans (estimated API-equivalent value)</h4>
      <div class="pricing-tables">
        <div class="pricing-group">
          <h4>Anthropic (Claude)</h4>
          <table class="pricing-table">
            <thead><tr><th>Plan</th><th>Price</th><th>~Value</th></tr></thead>
            <tbody>
              <tr><td>Pro</td><td>$20/mo</td><td>~$300</td></tr>
              <tr><td>Max 5x</td><td>$100/mo</td><td>~$1,500</td></tr>
              <tr><td>Max 20x</td><td>$200/mo</td><td>~$6,000</td></tr>
            </tbody>
          </table>
        </div>
        <div class="pricing-group">
          <h4>OpenAI (Codex)</h4>
          <table class="pricing-table">
            <thead><tr><th>Plan</th><th>Price</th><th>~Value</th></tr></thead>
            <tbody>
              <tr><td>Plus</td><td>$20/mo</td><td>~$200</td></tr>
              <tr><td>Pro 5x</td><td>$100/mo</td><td>~$1,000</td></tr>
              <tr><td>Pro 20x</td><td>$200/mo</td><td>~$4,000</td></tr>
            </tbody>
          </table>
        </div>
        <div class="pricing-group">
          <h4>Google (Gemini)</h4>
          <table class="pricing-table">
            <thead><tr><th>Plan</th><th>Price</th><th>~Value</th></tr></thead>
            <tbody>
              <tr><td>Free</td><td>$0</td><td>~$30</td></tr>
              <tr><td>AI Pro</td><td>$20/mo</td><td>~$300</td></tr>
              <tr><td>AI Ultra</td><td>$250/mo</td><td>~$3,000</td></tr>
            </tbody>
          </table>
        </div>
      </div>
      <p class="field-hint" style="margin-top:.5rem">~Value = estimated API-equivalent based on rate limits and community reports. Actual throughput depends on model mix, caching, and concurrency. Prices as of April 2026.</p>
    </div>
  </div>
  {/if}

  <!-- Tooltip -->
  {#if tooltip}
    <div class="costs-tooltip" style="left:{tooltip.x}px;top:{tooltip.y}px">
      {@html tooltip.content}
    </div>
  {/if}
</div>

<style>
  .costs { max-width: 920px; margin: 0 auto; padding: 0 1rem 2rem; position: relative; }

  /* Hero insight */
  .costs-hero {
    display: flex; align-items: flex-start; gap: .7rem;
    padding: .9rem 1.1rem; margin-bottom: 1rem;
    background: linear-gradient(135deg, rgba(22,163,98,.06), rgba(22,163,98,.02));
    border: 1px solid rgba(22,163,98,.2); border-radius: var(--radius);
  }
  .costs-hero-icon { color: var(--success); font-size: 1rem; flex-shrink: 0; margin-top: .1rem; }
  .costs-hero-body { flex: 1; }
  .costs-hero-text { font-size: .88rem; font-weight: 500; line-height: 1.4; }
  .costs-hero-basis { display: block; font-size: .7rem; color: var(--text-secondary); margin-top: .2rem; }
  .costs-hero-savings {
    flex-shrink: 0; font-size: 1rem; font-weight: 700; color: var(--success);
    background: rgba(22,163,98,.1); padding: .3rem .7rem; border-radius: 8px; white-space: nowrap;
  }

  .costs-tooltip {
    position: fixed; z-index: 1000;
    background: var(--surface); border: 1px solid var(--border);
    border-radius: var(--radius-sm); padding: .5rem .7rem;
    font-size: .75rem; line-height: 1.5; color: var(--text);
    box-shadow: var(--shadow); pointer-events: none;
    max-width: 320px; white-space: nowrap;
  }

  /* Summary — clickable stat cards that select the metric */
  .costs-summary { display: flex; gap: .6rem; margin-bottom: 1rem; flex-wrap: wrap; }
  .costs-stat {
    flex: 1; min-width: 90px; padding: .7rem .5rem; text-align: center;
    background: var(--surface); border: 1px solid var(--border); border-radius: var(--radius);
    cursor: pointer; font-family: inherit; color: var(--text); transition: all 150ms ease;
  }
  .costs-stat:hover { border-color: var(--primary-dim); }
  .costs-stat.active { border-color: var(--primary); background: var(--primary-glow); }
  .costs-stat-value { display: block; font-size: 1.2rem; font-weight: 700; line-height: 1.2; }
  .costs-stat.active .costs-stat-value { color: var(--primary); }
  .costs-stat-label { font-size: .65rem; color: var(--text-secondary); text-transform: uppercase; letter-spacing: .5px; }

  .costs-info {
    font-size: .78rem; color: var(--text-secondary); padding: .5rem .8rem;
    background: var(--surface); border: 1px solid var(--border); border-radius: var(--radius-sm);
    margin-bottom: .8rem;
  }

  /* Toolbar */
  .costs-toolbar { display: flex; gap: .6rem; margin-bottom: 1rem; flex-wrap: wrap; align-items: center; }
  .costs-toggle {
    display: flex; border: 1px solid var(--border); border-radius: var(--radius-sm); overflow: hidden;
  }
  .costs-toggle button {
    padding: .35rem .65rem; font-size: .78rem; font-weight: 500; cursor: pointer;
    border: none; background: var(--surface); color: var(--text-secondary);
    font-family: inherit; transition: all 150ms ease;
  }
  .costs-toggle button:hover { background: var(--surface-hover); }
  .costs-toggle button.active { background: var(--primary-glow); color: var(--primary); }
  .costs-toggle button:not(:last-child) { border-right: 1px solid var(--border); }
  .costs-select {
    padding: .35rem .5rem; border-radius: var(--radius-sm);
    border: 1px solid var(--border); background: var(--surface); color: var(--text);
    font-size: .78rem; cursor: pointer;
  }

  /* Cards */
  .costs-card {
    background: var(--surface); border: 1px solid var(--border); border-radius: var(--radius);
    padding: 1rem 1.2rem; margin-bottom: 1rem;
  }
  .costs-card-header { display: flex; align-items: baseline; gap: .6rem; margin-bottom: .8rem; }
  .costs-card-header h3 { font-size: .88rem; font-weight: 600; }
  .costs-card-sub { font-size: .75rem; color: var(--text-secondary); }

  /* Heatmap */
  .heatmap-scroll { overflow-x: auto; }
  .heatmap-grid { display: flex; gap: .3rem; min-width: 700px; padding-top: 14px; }
  .heatmap-day-labels { display: grid; grid-template-rows: repeat(7, 15px); align-items: center; }
  .heatmap-day-label { font-size: .6rem; color: var(--text-secondary); width: 14px; text-align: right; }
  .heatmap-svg { flex: 1; overflow: visible; }
  .heatmap-legend {
    display: flex; align-items: center; gap: .25rem; margin-top: .5rem;
    justify-content: flex-end; font-size: .65rem; color: var(--text-secondary);
  }
  .heatmap-legend-box { width: 12px; height: 12px; border-radius: 2px; }

  /* Chart */
  .chart-container { display: flex; gap: .3rem; }
  .chart-y-axis {
    display: flex; flex-direction: column; justify-content: space-between;
    font-size: .6rem; color: var(--text-secondary); text-align: right;
    padding-bottom: 18px; min-width: 36px;
  }
  .chart-area { flex: 1; overflow-x: auto; }
  .chart-svg { display: block; }
  .chart-legend { display: flex; gap: .7rem; margin-top: .6rem; flex-wrap: wrap; }
  .chart-legend-item { display: flex; align-items: center; gap: .25rem; font-size: .7rem; color: var(--text-secondary); }
  .chart-legend-dot { width: 8px; height: 8px; border-radius: 50%; }

  /* Recommendations */
  .recs-list { display: flex; flex-direction: column; gap: .5rem; }
  .rec {
    display: flex; align-items: flex-start; gap: .5rem;
    padding: .6rem .8rem; border-radius: var(--radius-sm);
    font-size: .82rem; line-height: 1.4;
  }
  .rec-save { background: rgba(22, 163, 98, .06); }
  .rec-downsize { background: rgba(234, 179, 8, .06); }
  .rec-insight { background: var(--surface-active); }
  .rec-icon { flex-shrink: 0; font-size: .7rem; margin-top: .15rem; }
  .rec-save .rec-icon { color: var(--success); }
  .rec-downsize .rec-icon { color: #eab308; }
  .rec-insight .rec-icon { color: var(--text-secondary); }
  .rec-body { flex: 1; display: flex; flex-direction: column; gap: .15rem; }
  .rec-text { }
  .rec-basis { font-size: .7rem; color: var(--text-secondary); opacity: .7; }
  .rec-savings {
    flex-shrink: 0; font-weight: 700; color: var(--success);
    font-size: .85rem; white-space: nowrap;
  }

  .recs-toggle {
    display: block; width: 100%; margin-top: .5rem; padding: .4rem;
    text-align: center; font-size: .78rem; color: var(--primary);
    background: none; border: none; cursor: pointer; font-family: inherit;
  }
  .recs-toggle:hover { text-decoration: underline; }

  .pricing-toggle {
    display: flex; align-items: center; width: 100%;
    padding: .6rem 1rem; margin-bottom: 0;
    background: var(--surface); border: 1px solid var(--border);
    border-radius: var(--radius); cursor: pointer;
    font-size: .82rem; font-weight: 500; color: var(--text);
    font-family: inherit; transition: background 150ms ease;
  }
  .pricing-toggle:hover { background: var(--surface-hover); }

  /* Pricing */
  .pricing-tables { display: flex; gap: 1rem; flex-wrap: wrap; }
  .pricing-group { flex: 1; min-width: 200px; }
  .pricing-group h4 { font-size: .78rem; font-weight: 600; margin-bottom: .4rem; color: var(--text); }
  .pricing-table { width: 100%; border-collapse: collapse; font-size: .72rem; }
  .pricing-table th { text-align: left; padding: .25rem .4rem; color: var(--text-secondary); font-weight: 500; border-bottom: 1px solid var(--border); }
  .pricing-table td { padding: .25rem .4rem; color: var(--text-secondary); }
  .pricing-table td:first-child { color: var(--text); font-weight: 500; }
  .plans-note { margin-top: .8rem; padding-top: .8rem; border-top: 1px solid var(--border); }
  .plans-note h4 { font-size: .75rem; font-weight: 600; margin-bottom: .4rem; }
  .plans-grid { display: flex; gap: .5rem; flex-wrap: wrap; }
  .plan {
    font-size: .7rem; padding: .2rem .5rem; border-radius: 6px;
    background: var(--surface-active); color: var(--text-secondary);
  }

  @media (max-width: 600px) {
    .costs-stat { min-width: 70px; padding: .5rem .3rem; }
    .costs-stat-value { font-size: 1rem; }
  }
</style>
