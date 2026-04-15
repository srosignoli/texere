<script>
  // Tutor tab placeholder — full implementation pending FSRS/MySQL and SStorytime-KV integration
</script>

<div class="tutor-tab">
  <p class="coming-soon">Knowledge Architect — coming soon</p>
  <select class="llm-selector" disabled>
    <option value="">Select LLM model</option>
  </select>
</div>

<style>
  .tutor-tab {
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    height: 100%;
    gap: 16px;
    color: #4a5a6a;
  }

  .coming-soon {
    margin: 0;
    font-size: 0.9rem;
  }

  .llm-selector {
    padding: 6px 12px;
    background: #1e2f40;
    border: 1px solid #2a3a4a;
    border-radius: 4px;
    color: #4a5a6a;
    font-size: 0.85rem;
    cursor: not-allowed;
  }
</style>
