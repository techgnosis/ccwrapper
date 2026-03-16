(function() {
  'use strict';

  const EventType = {
    TEXT: 'text',
    THINKING: 'thinking',
    TOOL_USE: 'tool_use',
    TOOL_RESULT: 'tool_result',
    STATUS: 'status',
    COMMAND: 'command',
    ERROR: 'error',
    SYSTEM: 'system',
    RATE_LIMIT: 'rate_limit',
    RESULT: 'result',
    RESULT_SUMMARY: 'result_summary',
  };

  const outputPanel = document.getElementById('output-panel');
  const scrollIndicator = document.getElementById('scroll-indicator');
  const systemContent = document.getElementById('system-content');
  const commandContent = document.getElementById('command-content');
  const stateContent = document.getElementById('state-content');
  const claudeJsonContent = document.getElementById('claude-json-content');
  const btnRefreshState = document.getElementById('btn-refresh-state');
  const planEditor = document.getElementById('plan-editor');
  const btnSendPlan = document.getElementById('btn-send-plan');
  const btnSendExecute = document.getElementById('btn-send-execute');
  const executePreview = document.getElementById('execute-preview');
  const brListContent = document.getElementById('br-list-content');
  const btnRefreshWorkitems = document.getElementById('btn-refresh-workitems');
  const btnScrapWork = document.getElementById('btn-scrap-work');
  const btnSendRefine = document.getElementById('btn-send-refine');
  const refinePreview = document.getElementById('refine-preview');
  const answerTab = document.getElementById('answer-tab');
  const answerEditor = document.getElementById('answer-editor');
  const answerPreview = document.getElementById('answer-preview');
  const btnSendAnswers = document.getElementById('btn-send-answers');
  const btnCancelAnswers = document.getElementById('btn-cancel-answers');

  const tokenTotals = document.getElementById('token-totals');

  let autoScroll = true;
  let isRunning = false;
  let currentTurn = null;
  let currentPrompt = null;
  let turnCount = 0;
  let pendingEl = null;
  let totalInputTokens = 0;
  let totalOutputTokens = 0;
  let totalCost = 0;
  const promptContents = { plan: '', execute: '', refine: '', answer: '' };

  // --- Tabs ---
  function initTabGroup(containerSelector, panelParentSelector) {
    const container = document.querySelector(containerSelector);
    const panelParent = document.querySelector(panelParentSelector);
    container.querySelectorAll('.tab').forEach(tab => {
      tab.addEventListener('click', () => {
        container.querySelectorAll('.tab').forEach(t => t.classList.remove('active'));
        panelParent.querySelectorAll(':scope > .panel').forEach(p => p.classList.remove('active'));
        tab.classList.add('active');
        document.getElementById(tab.dataset.panel).classList.add('active');
        if (tab.dataset.panel === 'state-panel') loadState();
        if (tab.dataset.panel === 'execute-panel') loadBrList();
      });
    });
  }
  initTabGroup('.left-panel > .tabs', '.left-panel');
  initTabGroup('.right-panel > .tabs', '.right-panel');

  // --- Auto-scroll toggle ('s' key) ---
  document.addEventListener('keydown', e => {
    if (e.key === 's' && document.activeElement.tagName !== 'INPUT' && document.activeElement.tagName !== 'TEXTAREA') {
      autoScroll = !autoScroll;
      scrollIndicator.textContent = 'scroll: ' + (autoScroll ? 'on' : 'off');
    }
  });

  function doAutoScroll() {
    if (autoScroll) {
      outputPanel.scrollTop = outputPanel.scrollHeight;
    }
  }

  // --- Pending indicator ---
  function showPending() {
    removePending();
    pendingEl = document.createElement('div');
    pendingEl.className = 'pending-line';
    pendingEl.textContent = 'pending';
    outputPanel.appendChild(pendingEl);
    doAutoScroll();
  }

  function removePending() {
    if (pendingEl) {
      pendingEl.remove();
      pendingEl = null;
    }
  }

  // --- Load prompt file from server ---
  function loadPromptFile(name, previewEl) {
    fetch('/api/prompts/' + name)
      .then(r => r.json())
      .then(data => {
        promptContents[name] = data.content || '';
        if (previewEl) previewEl.textContent = promptContents[name] || '(' + name + '.md is empty)';
      })
      .catch(() => {
        promptContents[name] = '';
        if (previewEl) previewEl.textContent = '(failed to load ' + name + '.md)';
      });
  }
  loadPromptFile('plan');
  loadBrList(); // Check open issues on page load to set plan disabled state

  // --- Plan disabled state based on open issues ---
  const planDisabledTitle = 'Plan only works when there are no open issues';

  function updatePlanDisabledState(openCount) {
    const hasOpen = openCount > 0;
    // Don't override the isRunning disabled state
    if (!isRunning) {
      planEditor.disabled = hasOpen;
      btnSendPlan.disabled = hasOpen;
    }
    planEditor.title = hasOpen ? planDisabledTitle : '';
    btnSendPlan.title = hasOpen ? planDisabledTitle : '';
  }

  // --- Load br list from server ---
  function loadBrList() {
    fetch('/api/br-list')
      .then(r => r.json())
      .then(data => {
        brListContent.textContent = data.output || '(no work items)';
        updatePlanDisabledState(data.open_count || 0);
      })
      .catch(() => {
        brListContent.textContent = '(failed to load br list)';
      });
  }

  // --- Refresh button for Work Items panel ---
  btnRefreshWorkitems.addEventListener('click', loadBrList);

  // --- Scrap Work button ---
  btnScrapWork.addEventListener('click', () => {
    if (!window.confirm('Delete all open work items? This cannot be undone.')) return;
    fetch('/api/br-scrap', { method: 'POST' })
      .then(r => r.json())
      .then(data => {
        if (data.error) {
          alert('Scrap failed: ' + data.error);
        } else {
          loadBrList();
        }
      })
      .catch(err => {
        alert('Scrap failed: ' + err.message);
      });
  });

  loadPromptFile('execute', executePreview);

  loadPromptFile('refine', refinePreview);

  loadPromptFile('answer', answerPreview);

  // --- Send prompt (generic) ---
  function sendPrompt(type, prompt) {
    if (!prompt || isRunning) return;
    currentPrompt = type;
    startNewTurn(type);
    fetch('/api/prompt', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ prompt })
    }).catch(err => {
      addBlock(currentTurn, EventType.ERROR, 'Failed to send: ' + err.message);
    });
  }

  // --- Send refine ---
  btnSendRefine.addEventListener('click', () => {
    sendPrompt('refine', promptContents.refine);
  });

  // --- Send answers ---
  btnCancelAnswers.addEventListener('click', () => {
    answerEditor.value = '';
    answerTab.style.display = 'none';
    answerTab.classList.remove('active');
    document.getElementById('answer-panel').classList.remove('active');
  });

  btnSendAnswers.addEventListener('click', () => {
    const text = answerEditor.value.trim();
    if (!text) return;
    const prompt = promptContents.answer ? promptContents.answer + '\n\n' + text : text;
    sendPrompt('answers', prompt);
  });

  // --- Send execute ---
  btnSendExecute.addEventListener('click', () => {
    sendPrompt('execute', promptContents.execute);
  });

  // --- Send plan ---
  btnSendPlan.addEventListener('click', () => {
    const userText = planEditor.value.trim();
    if (!userText) return;
    const prompt = promptContents.plan + '\n\n' + userText;
    sendPrompt(userText, prompt);
  });

  // --- State tab ---
  btnRefreshState.addEventListener('click', loadState);
  function loadState() {
    fetch('/api/state')
      .then(r => r.json())
      .then(sections => {
        stateContent.innerHTML = sections.map(s => {
          var header = '<strong>' + esc(s.path) + '</strong>';
          if (s.created && !s.path.endsWith('.cache')) {
            header += '  <span style="opacity:0.5;font-size:0.72rem;">created: ' + esc(new Date(s.created).toLocaleString()) + '</span>';
          }
          return header + '\n' + esc(s.content);
        }).join('\n\n');
      })
      .catch(err => {
        stateContent.innerHTML = '<pre>Error: ' + esc(err.message) + '</pre>';
      });
    fetch('/api/claude-json')
      .then(r => r.json())
      .then(data => {
        if (data.error) {
          claudeJsonContent.innerHTML = '<strong>~/.claude.json</strong>\n' + esc(data.error);
          return;
        }
        var modTime = data._lastModified || '';
        var header = '<strong>~/.claude.json</strong>';
        if (modTime) {
          header += '  <span style="opacity:0.5;font-size:0.72rem;">modified: ' + esc(new Date(modTime).toLocaleString()) + '</span>';
        }
        var lines = Object.keys(data).sort().filter(k => k !== '_lastModified').map(k => {
          var val = typeof data[k] === 'string' ? data[k] : JSON.stringify(data[k]);
          return '<strong>' + esc(k) + '</strong>: ' + esc(val);
        });
        claudeJsonContent.innerHTML = header + '\n\n' + lines.join('\n');
      })
      .catch(err => {
        claudeJsonContent.innerHTML = '<pre>Error: ' + esc(err.message) + '</pre>';
      });
  }

  // --- Turn management ---
  function startNewTurn(prompt) {
    turnCount++;
    // Collapse all existing turns before creating the new one
    outputPanel.querySelectorAll('.turn').forEach(t => t.classList.add('collapsed'));

    const turn = document.createElement('div');
    turn.className = 'turn';
    turn.dataset.turnId = turnCount;

    const summary = document.createElement('div');
    summary.className = 'turn-summary';
    summary.innerHTML = '<span class="label">user</span>' + esc(prompt);
    turn.appendChild(summary);

    const body = document.createElement('div');
    body.className = 'turn-body';
    turn.appendChild(body);

    turn.addEventListener('click', e => {
      const wasCollapsed = turn.classList.contains('collapsed');
      if (wasCollapsed) {
        // Collapse all other turns
        outputPanel.querySelectorAll('.turn').forEach(t => {
          if (t !== turn) t.classList.add('collapsed');
        });
      }
      turn.classList.toggle('collapsed');
    });

    outputPanel.appendChild(turn);
    currentTurn = turn;
    showPending();
  }

  function collapseTurn(turn) {
    if (!turn) return;
    turn.classList.add('collapsed');
  }

  function autoPopulateAnswerEditor(turn) {
    const textBlocks = turn.querySelectorAll('.turn-body .block-text');
    if (textBlocks.length === 0) return;
    const allText = Array.from(textBlocks).map(b => b.textContent).join('\n\n');
    // Show the Answer tab and switch to it
    answerTab.style.display = '';
    const leftPanel = document.querySelector('.left-panel');
    leftPanel.querySelectorAll('.tab').forEach(t => t.classList.remove('active'));
    leftPanel.querySelectorAll(':scope > .panel').forEach(p => p.classList.remove('active'));
    answerTab.classList.add('active');
    document.getElementById('answer-panel').classList.add('active');
    answerEditor.value = allText;
  }

  function addBlock(turn, type, content, extra) {
    if (!turn) return;
    removePending();

    const body = turn.querySelector('.turn-body');
    const block = document.createElement('div');
    block.className = 'block';

    switch (type) {
      case EventType.TEXT:
        block.classList.add('block-text');
        block.textContent = content;
        break;

      case EventType.THINKING:
        block.classList.add('block-thinking', 'collapsed');
        block.textContent = content;
        block.addEventListener('click', e => { e.stopPropagation(); block.classList.toggle('collapsed'); });
        break;

      case EventType.TOOL_USE:
        block.classList.add('block-tool');
        block.dataset.toolId = extra.toolId || '';
        block.innerHTML = '<span class="tool-name">' + esc(extra.toolName || '') + '</span>'
          + '<span class="tool-input">' + esc(content) + '</span>';
        break;

      case EventType.TOOL_RESULT:
        block.classList.add('block-result', 'collapsed');
        if (extra && extra.isError) block.classList.add('error');
        block.textContent = content;
        block.addEventListener('click', e => { e.stopPropagation(); block.classList.toggle('collapsed'); });
        break;

      case EventType.ERROR:
        block.classList.add('block-result', 'error');
        block.textContent = content;
        break;

      case EventType.RESULT_SUMMARY:
        block.classList.add('block-result-summary');
        block.innerHTML = content;
        break;
    }

    body.appendChild(block);
    showPending();
  }

  // Map tool IDs to their parent turn for indenting results
  const toolIdToTurn = {};

  // --- SSE connection ---
  function connectSSE() {
    const evtSource = new EventSource('/events');

    evtSource.onmessage = function(e) {
      let data;
      try {
        data = JSON.parse(e.data);
      } catch {
        return;
      }

      switch (data.type) {
        case EventType.STATUS:
          isRunning = data.running;
          btnSendExecute.disabled = isRunning;
          btnSendRefine.disabled = isRunning;
          btnSendAnswers.disabled = isRunning;
          if (isRunning) {
            btnSendPlan.disabled = true;
            planEditor.disabled = true;
          }
          if (!isRunning) {
            removePending();
            if (currentTurn) {
              if (currentPrompt === 'refine') {
                autoPopulateAnswerEditor(currentTurn);
              }
              collapseTurn(currentTurn);
            }
            loadBrList(); // Re-check open issues to update plan disabled state
          }
          break;

        case EventType.SYSTEM:
          if (data.system_raw) {
            try {
              systemContent.textContent = JSON.stringify(data.system_raw, null, 2);
            } catch {
              systemContent.textContent = JSON.stringify(data, null, 2);
            }
          }
          break;

        case EventType.COMMAND:
          commandContent.textContent = data.content || '';
          break;

        case EventType.TEXT:
          if (!currentTurn) startNewTurn('(continued)');
          addBlock(currentTurn, EventType.TEXT, data.content);
          break;

        case EventType.THINKING:
          if (!currentTurn) startNewTurn('(continued)');
          addBlock(currentTurn, EventType.THINKING, data.content);
          break;

        case EventType.TOOL_USE:
          if (!currentTurn) startNewTurn('(continued)');
          toolIdToTurn[data.tool_id] = currentTurn;
          addBlock(currentTurn, EventType.TOOL_USE, data.tool_input, {
            toolName: data.tool_name,
            toolId: data.tool_id
          });
          break;

        case EventType.TOOL_RESULT:
          const parentTurn = toolIdToTurn[data.parent_tool_id] || currentTurn;
          addBlock(parentTurn, EventType.TOOL_RESULT, data.content, {
            isError: data.is_error
          });
          break;

        case EventType.RATE_LIMIT:
          break;

        case EventType.RESULT:
          if (data.input_tokens) totalInputTokens += data.input_tokens;
          if (data.output_tokens) totalOutputTokens += data.output_tokens;
          if (data.total_cost_usd) totalCost += data.total_cost_usd;
          updateTokenTotals();
          if (currentTurn) {
            let summaryHTML = '';
            if (data.stop_reason) summaryHTML += '<span>stop_reason: ' + esc(data.stop_reason) + '</span>';
            if (data.duration_ms) summaryHTML += '<span>duration_ms: ' + data.duration_ms + '</span>';
            if (data.total_cost_usd) summaryHTML += '<span>total_cost_usd: $' + data.total_cost_usd.toFixed(4) + '</span>';
            if (data.input_tokens) summaryHTML += '<span>input_tokens: ' + data.input_tokens + '</span>';
            if (data.output_tokens) summaryHTML += '<span>output_tokens: ' + data.output_tokens + '</span>';
            if (data.num_turns) summaryHTML += '<span>num_turns: ' + data.num_turns + '</span>';
            if (summaryHTML) {
              addBlock(currentTurn, EventType.RESULT_SUMMARY, summaryHTML);
            }
          }
          break;

        case EventType.ERROR:
          if (currentTurn) {
            addBlock(currentTurn, EventType.ERROR, data.content);
          }
          break;
      }
    };

    evtSource.onerror = function() {
      setTimeout(connectSSE, 2000);
      evtSource.close();
    };
  }

  connectSSE();

  // --- Token totals ---
  function updateTokenTotals() {
    let parts = [];
    if (totalInputTokens) parts.push('in: ' + totalInputTokens.toLocaleString());
    if (totalOutputTokens) parts.push('out: ' + totalOutputTokens.toLocaleString());
    if (totalCost) parts.push('$' + totalCost.toFixed(4));
    tokenTotals.textContent = parts.join(' · ');
  }

  // --- Escape HTML ---
  function esc(s) {
    if (!s) return '';
    const div = document.createElement('div');
    div.textContent = s;
    return div.innerHTML;
  }
})();
