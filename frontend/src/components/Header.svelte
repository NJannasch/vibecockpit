<script>
  /**
   * Header.svelte — Top header bar with logo, subtitle, and action buttons.
   *
   * Props:
   *   theme: 'light' | 'dark'
   *
   * Events (callback props):
   *   onnewproject()
   *   onsettings()
   *   ontoggletheme()
   */

  let { theme = 'light', onnewproject, onsettings, ontoggletheme } = $props();

  let themeIcon = $derived(theme === 'dark' ? '☽' : '☼');
</script>

<header>
  <div class="logo">
    <span class="logo-icon">&#9670;</span>
    <span>VibeCockpit</span>
    <span class="logo-sub">The cockpit for your copilots</span>
  </div>
  <div class="header-actions">
    <button class="btn btn-primary" onclick={onnewproject}>
      <span>+</span> New Project
    </button>
    <button
      class="btn btn-icon btn-ghost"
      onclick={ontoggletheme}
      title="Toggle theme"
    >{themeIcon}</button>
    <button
      class="btn btn-icon btn-ghost"
      onclick={onsettings}
      title="Settings"
    >&#9881;</button>
    <a
      href="https://github.com/njannasch/vibecockpit"
      target="_blank"
      class="btn btn-icon btn-ghost"
      title="GitHub"
    >&#9733;</a>
  </div>
</header>
