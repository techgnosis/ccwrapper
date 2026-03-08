(function() {
  'use strict';

  const outputPanel = document.getElementById('output-panel');
  const contextPanel = document.getElementById('context-panel');
  const promptInput = document.getElementById('prompt-input');
  const btnSend = document.getElementById('btn-send');
  const btnStop = document.getElementById('btn-stop');
  const btnClear = document.getElementById('btn-clear');
  const btnRefreshCtx = document.getElementById('btn-refresh-ctx');
  const contextContent = document.getElementById('context-content');
  const scrollIndicator = document.getElementById('scroll-indicator');
  const statusState = document.getElementById('status-state');
  const statusCost = document.getElementById('status-cost');
  const statusTokens = document.getElementById('status-tokens');
  const statusDuration = document.getElementById('status-duration');

  let autoScroll = true;
  let isRunning = false;
  let currentTurn = null;      // the active turn DOM element
  let currentPrompt = null;    // the user prompt that started this turn
  let turnCount = 0;

  // --- Tabs ---
  document.querySelectorAll('.tab').forEach(tab => {
    tab.addEventListener('click', () => {
      document.querySelectorAll('.tab').forEach(t => t.classList.remove('active'));
      document.querySelectorAll('.panel').forEach(p => p.classList.remove('active'));
      tab.classList.add('active');
      document.getElementById(tab.dataset.panel).classList.add('active');
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

  // --- Send prompt ---
  btnSend.addEventListener('click', sendPrompt);
  promptInput.addEventListener('keydown', e => {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault();
      sendPrompt();
    }
  });

  function sendPrompt() {
    const prompt = promptInput.value.trim();
    if (!prompt || isRunning) return;

    currentPrompt = prompt;
    promptInput.value = '';

    // Create a new turn
    startNewTurn(prompt);

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

  // --- Clear ---
  btnClear.addEventListener('click', () => {
    if (isRunning) return;
    fetch('/api/clear', { method: 'POST' }).then(() => {
      outputPanel.innerHTML = '';
      currentTurn = null;
      turnCount = 0;
      statusCost.textContent = '';
      statusTokens.textContent = '';
      statusDuration.textContent = '';
    }).catch(() => {});
  });

  // --- Context tab refresh ---
  btnRefreshCtx.addEventListener('click', loadContext);

  function loadContext() {
    fetch('/api/context')
      .then(r => r.json())
      .then(data => {
        let html = '';

        html += '<h4>~/.claude/</h4>';
        if (data.claude_dir && data.claude_dir.length > 0) {
          html += '<pre>' + esc(data.claude_dir.join('\n')) + '</pre>';
        } else {
          html += '<pre>(empty or not found)</pre>';
        }

        html += '<h4>~/claude.json*</h4>';
        const jsonFiles = data.claude_json_files || {};
        const keys = Object.keys(jsonFiles);
        if (keys.length > 0) {
          keys.forEach(k => {
            html += '<h4>' + esc(k) + '</h4>';
            html += '<pre>' + esc(jsonFiles[k]) + '</pre>';
          });
        } else {
          html += '<pre>(none found)</pre>';
        }

        html += '<h4>Conversation Context</h4>';
        html += '<pre>' + esc(data.context || '(empty)') + '</pre>';

        contextContent.innerHTML = html;
      })
      .catch(err => {
        contextContent.innerHTML = '<pre>Error: ' + esc(err.message) + '</pre>';
      });
  }

  // --- Turn management ---
  function startNewTurn(prompt) {
    turnCount++;
    const turn = document.createElement('div');
    turn.className = 'turn';
    turn.dataset.turnId = turnCount;

    const summary = document.createElement('div');
    summary.className = 'turn-summary';
    summary.innerHTML = '<span class="label">User:</span>' + esc(prompt);
    turn.appendChild(summary);

    const body = document.createElement('div');
    body.className = 'turn-body';
    turn.appendChild(body);

    // Click to toggle expand/collapse
    turn.addEventListener('click', e => {
      // Don't toggle if clicking inside a question input
      if (e.target.closest('.question-input-wrap')) return;
      turn.classList.toggle('collapsed');
    });

    outputPanel.appendChild(turn);
    currentTurn = turn;
    doAutoScroll();
  }

  function collapseTurn(turn) {
    if (!turn) return;
    turn.classList.add('collapsed');
  }

  function addBlock(turn, type, content, extra) {
    if (!turn) return;
    const body = turn.querySelector('.turn-body');
    const block = document.createElement('div');
    block.className = 'block';

    switch (type) {
      case 'text':
        block.classList.add('block-text');
        block.textContent = content;
        // Update turn summary line 2
        updateTurnResponse(turn, content);
        break;

      case 'thinking':
        block.classList.add('block-thinking');
        block.textContent = content;
        break;

      case 'tool_use':
        block.classList.add('block-tool');
        block.dataset.toolId = extra.toolId || '';
        block.innerHTML = '<span class="tool-name">' + esc(extra.toolName || '') + '</span>'
          + '<span class="tool-input">' + esc(content) + '</span>';
        break;

      case 'tool_result':
        block.classList.add('block-result');
        if (extra && extra.isError) block.classList.add('error');
        block.textContent = content;
        break;

      case 'rate_limit':
        block.classList.add('block-rate-limit');
        block.textContent = 'rate limit: ' + content;
        break;

      case 'error':
        block.classList.add('block-result', 'error');
        block.textContent = content;
        break;

      case 'result_summary':
        block.classList.add('block-result-summary');
        block.innerHTML = content; // pre-formatted HTML
        break;
    }

    body.appendChild(block);
    doAutoScroll();
  }

  function updateTurnResponse(turn, text) {
    const summary = turn.querySelector('.turn-summary');
    // Show user prompt on line 1, model response on line 2
    const promptText = summary.querySelector('.label').nextSibling;
    const userText = promptText ? promptText.textContent : '';
    const truncated = text.length > 120 ? text.substring(0, 120) + '...' : text;
    summary.innerHTML = '<span class="label">User:</span>' + esc(currentPrompt || userText)
      + '<br><span class="label">Claude:</span>' + esc(truncated);
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
          statusState.textContent = isRunning ? 'running' : 'idle';
          btnSend.style.display = isRunning ? 'none' : '';
          btnStop.style.display = isRunning ? '' : 'none';
          promptInput.disabled = isRunning;
          if (!isRunning && currentTurn) {
            collapseTurn(currentTurn);
          }
          break;

        case 'init':
          statusState.textContent = 'connected: ' + (data.model || '');
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
          if (currentTurn) {
            addBlock(currentTurn, 'rate_limit', data.content);
          }
          break;

        case 'result':
          if (currentTurn) {
            let summaryHTML = '';
            if (data.duration_ms) summaryHTML += '<span>duration: ' + (data.duration_ms / 1000).toFixed(1) + 's</span>';
            if (data.total_cost_usd) summaryHTML += '<span>cost: $' + data.total_cost_usd.toFixed(4) + '</span>';
            if (data.input_tokens) summaryHTML += '<span>in: ' + data.input_tokens + '</span>';
            if (data.output_tokens) summaryHTML += '<span>out: ' + data.output_tokens + '</span>';
            if (data.num_turns) summaryHTML += '<span>turns: ' + data.num_turns + '</span>';
            if (summaryHTML) {
              addBlock(currentTurn, 'result_summary', summaryHTML);
            }
            // Update status bar
            if (data.total_cost_usd) statusCost.textContent = 'cost: $' + data.total_cost_usd.toFixed(4);
            if (data.input_tokens || data.output_tokens) statusTokens.textContent = 'tokens: ' + (data.input_tokens||0) + ' in / ' + (data.output_tokens||0) + ' out';
            if (data.duration_ms) statusDuration.textContent = 'time: ' + (data.duration_ms/1000).toFixed(1) + 's';

            // Check if result ends with a question — offer inline answer
            if (data.content && data.content.trim().match(/\?$/)) {
              addQuestionInput(currentTurn);
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
      statusState.textContent = 'disconnected';
      setTimeout(connectSSE, 2000);
      evtSource.close();
    };
  }

  connectSSE();

  // --- Question answering ---
  function addQuestionInput(turn) {
    const body = turn.querySelector('.turn-body');
    const wrap = document.createElement('div');
    wrap.className = 'question-input-wrap';
    const inp = document.createElement('input');
    inp.type = 'text';
    inp.placeholder = 'Type your answer...';
    const btn = document.createElement('button');
    btn.textContent = 'Answer';

    function submitAnswer() {
      const answer = inp.value.trim();
      if (!answer || isRunning) return;
      wrap.remove();
      currentPrompt = answer;
      startNewTurn(answer);
      fetch('/api/prompt', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ prompt: answer })
      }).catch(err => {
        addBlock(currentTurn, 'error', 'Failed to send: ' + err.message);
      });
    }

    btn.addEventListener('click', submitAnswer);
    inp.addEventListener('keydown', e => {
      if (e.key === 'Enter') { e.preventDefault(); submitAnswer(); }
    });

    wrap.appendChild(inp);
    wrap.appendChild(btn);
    body.appendChild(wrap);
    // Don't collapse this turn since it has a question
    turn.classList.remove('collapsed');
    inp.focus();
    doAutoScroll();
  }

  // --- Escape HTML ---
  function esc(s) {
    if (!s) return '';
    const div = document.createElement('div');
    div.textContent = s;
    return div.innerHTML;
  }
})();
