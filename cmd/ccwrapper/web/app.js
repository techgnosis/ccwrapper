(function() {
  'use strict';

  const outputPanel = document.getElementById('output-panel');
  const btnStop = document.getElementById('btn-stop');
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
  let planMdContents = ''; // loaded from server
  let executeMdContents = ''; // loaded from server
  let refineMdContents = ''; // loaded from server
  let answerMdContents = ''; // loaded from server

  // --- Tabs ---
  document.querySelectorAll('.tab').forEach(tab => {
    tab.addEventListener('click', () => {
      document.querySelectorAll('.tab').forEach(t => t.classList.remove('active'));
      document.querySelectorAll('.panel').forEach(p => p.classList.remove('active'));
      tab.classList.add('active');
      document.getElementById(tab.dataset.panel).classList.add('active');
      if (tab.dataset.panel === 'state-panel') loadState();
      if (tab.dataset.panel === 'execute-panel') loadBrList();
    });
  });

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

  // --- Load plan.md from server ---
  function loadPlanMd() {
    fetch('/api/prompts/plan')
      .then(r => r.json())
      .then(data => { planMdContents = data.content || ''; })
      .catch(() => { planMdContents = ''; });
  }
  loadPlanMd();
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

  // --- Load execute.md from server ---
  function loadExecuteMd() {
    fetch('/api/prompts/execute')
      .then(r => r.json())
      .then(data => {
        executeMdContents = data.content || '';
        executePreview.textContent = executeMdContents || '(execute.md is empty)';
      })
      .catch(() => {
        executeMdContents = '';
        executePreview.textContent = '(failed to load execute.md)';
      });
  }
  loadExecuteMd();

  // --- Load refine.md from server ---
  function loadRefineMd() {
    fetch('/api/prompts/refine')
      .then(r => r.json())
      .then(data => {
        refineMdContents = data.content || '';
        refinePreview.textContent = refineMdContents || '(refine.md is empty)';
      })
      .catch(() => {
        refineMdContents = '';
        refinePreview.textContent = '(failed to load refine.md)';
      });
  }
  loadRefineMd();

  // --- Load answer.md from server ---
  function loadAnswerMd() {
    fetch('/api/prompts/answer')
      .then(r => r.json())
      .then(data => {
        answerMdContents = data.content || '';
        answerPreview.textContent = answerMdContents || '(answer.md is empty)';
      })
      .catch(() => {
        answerMdContents = '';
        answerPreview.textContent = '(failed to load answer.md)';
      });
  }
  loadAnswerMd();

  // --- Send refine ---
  btnSendRefine.addEventListener('click', sendRefine);

  function sendRefine() {
    if (!refineMdContents || isRunning) return;

    const prompt = refineMdContents;
    currentPrompt = 'refine';

    startNewTurn('refine');

    fetch('/api/prompt', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ prompt })
    }).catch(err => {
      addBlock(currentTurn, 'error', 'Failed to send: ' + err.message);
    });
  }

  // --- Send answers ---
  btnSendAnswers.addEventListener('click', sendAnswers);

  function sendAnswers() {
    const text = answerEditor.value.trim();
    if (!text || isRunning) return;

    currentPrompt = 'answers';

    // Prepend answer.md contents to the prompt
    const prompt = answerMdContents ? answerMdContents + '\n\n' + text : text;

    startNewTurn('answers');

    fetch('/api/prompt', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ prompt })
    }).catch(err => {
      addBlock(currentTurn, 'error', 'Failed to send: ' + err.message);
    });
  }

  // --- Send execute ---
  btnSendExecute.addEventListener('click', sendExecute);

  function sendExecute() {
    if (!executeMdContents || isRunning) return;

    const prompt = executeMdContents;
    currentPrompt = 'execute';

    startNewTurn('execute');

    fetch('/api/prompt', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ prompt })
    }).catch(err => {
      addBlock(currentTurn, 'error', 'Failed to send: ' + err.message);
    });
  }

  // --- Send plan ---
  btnSendPlan.addEventListener('click', sendPlan);

  function sendPlan() {
    const userText = planEditor.value.trim();
    if (!userText || isRunning) return;

    const prompt = planMdContents + '\n\n' + userText;
    currentPrompt = userText;

    startNewTurn(userText);

    fetch('/api/prompt', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ prompt })
    }).catch(err => {
      addBlock(currentTurn, 'error', 'Failed to send: ' + err.message);
    });
  }

  // --- Stop ---
  btnStop.addEventListener('click', () => {
    fetch('/api/stop', { method: 'POST' }).catch(() => {});
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
    document.querySelectorAll('.tab').forEach(t => t.classList.remove('active'));
    document.querySelectorAll('.panel').forEach(p => p.classList.remove('active'));
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
      case 'text':
        block.classList.add('block-text');
        block.textContent = content;
        updateTurnResponse(turn, content);
        break;

      case 'thinking':
        block.classList.add('block-thinking', 'collapsed');
        block.textContent = content;
        block.addEventListener('click', e => { e.stopPropagation(); block.classList.toggle('collapsed'); });
        break;

      case 'tool_use':
        block.classList.add('block-tool');
        block.dataset.toolId = extra.toolId || '';
        block.innerHTML = '<span class="tool-name">' + esc(extra.toolName || '') + '</span>'
          + '<span class="tool-input">' + esc(content) + '</span>';
        break;

      case 'tool_result':
        block.classList.add('block-result', 'collapsed');
        if (extra && extra.isError) block.classList.add('error');
        block.textContent = content;
        block.addEventListener('click', e => { e.stopPropagation(); block.classList.toggle('collapsed'); });
        break;

      case 'error':
        block.classList.add('block-result', 'error');
        block.textContent = content;
        break;

      case 'result_summary':
        block.classList.add('block-result-summary');
        block.innerHTML = content;
        break;
    }

    body.appendChild(block);
    showPending();
  }

  function updateTurnResponse(turn, text) {
    const summary = turn.querySelector('.turn-summary');
    const promptText = summary.querySelector('.label').nextSibling;
    const userText = promptText ? promptText.textContent : '';
    const truncated = text.length > 120 ? text.substring(0, 120) + '...' : text;
    summary.innerHTML = '<span class="label">user</span>' + esc(currentPrompt || userText)
      + '<br><span class="label">assistant</span>' + esc(truncated);
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
        case 'status':
          isRunning = data.running;
          btnStop.style.display = isRunning ? '' : 'none';
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

        case 'system':
          if (data.system_raw) {
            try {
              systemContent.textContent = JSON.stringify(data.system_raw, null, 2);
            } catch {
              systemContent.textContent = JSON.stringify(data, null, 2);
            }
          }
          break;

        case 'command':
          commandContent.textContent = data.content || '';
          break;

        case 'text':
          if (!currentTurn) startNewTurn('(continued)');
          addBlock(currentTurn, 'text', data.content);
          break;

        case 'thinking':
          if (!currentTurn) startNewTurn('(continued)');
          addBlock(currentTurn, 'thinking', data.content);
          break;

        case 'tool_use':
          if (!currentTurn) startNewTurn('(continued)');
          toolIdToTurn[data.tool_id] = currentTurn;
          addBlock(currentTurn, 'tool_use', data.tool_input, {
            toolName: data.tool_name,
            toolId: data.tool_id
          });
          break;

        case 'tool_result':
          const parentTurn = toolIdToTurn[data.parent_tool_id] || currentTurn;
          addBlock(parentTurn, 'tool_result', data.content, {
            isError: data.is_error
          });
          break;

        case 'rate_limit':
          break;

        case 'result':
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
              addBlock(currentTurn, 'result_summary', summaryHTML);
            }
          }
          break;

        case 'error':
          if (currentTurn) {
            addBlock(currentTurn, 'error', data.content);
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
