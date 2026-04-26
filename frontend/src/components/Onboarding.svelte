<script>
  /**
   * Onboarding.svelte — Welcome screen shown when no sessions exist.
   *
   * Events (callback props):
   *   onnewproject()
   */

  let { onnewproject } = $props();
</script>

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
  <button class="btn btn-primary" onclick={onnewproject}>+ Create Your First Project</button>
</div>
