(function() {
  'use strict';

  const outputPanel = document.getElementById('output-panel');
  const contextPanel = document.getElementById('context-panel');
  const promptInput = document.getElementById('prompt-input');
  const btnSend = document.getElementById('btn-send');
  const btnStop = document.getElementById('btn-stop');
  const btnClear = document.getElementById('btn-clear');
  const btnCancelAnswer = document.getElementById('btn-cancel-answer');
  const btnRefreshCtx = document.getElementById('btn-refresh-ctx');
  const contextContent = document.getElementById('context-content');
  const scrollIndicator = document.getElementById('scroll-indicator');

  let autoScroll = true;
  let isRunning = false;
  let currentTurn = null;
  let currentPrompt = null;
  let turnCount = 0;
  let pendingEl = null;
  let answerMode = false;

  // --- Tabs ---
  document.querySelectorAll('.tab').forEach(tab => {
    tab.addEventListener('click', () => {
      document.querySelectorAll('.tab').forEach(t => t.classList.remove('active'));
      document.querySelectorAll('.panel').forEach(p => p.classList.remove('active'));
      tab.classList.add('active');
      document.getElementById(tab.dataset.panel).classList.add('active');
      if (tab.dataset.panel === 'context-panel') loadContext();
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

  // --- Send prompt ---
  btnSend.addEventListener('click', sendPrompt);
  promptInput.addEventListener('keydown', e => {
    if (e.key === 'Enter' && !e.shiftKey && !answerMode) {
      e.preventDefault();
      sendPrompt();
    }
  });

  function sendPrompt() {
    const prompt = promptInput.value.trim();
    if (!prompt || isRunning) return;

    currentPrompt = prompt;
    promptInput.value = '';
    exitAnswerMode();

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
      const divider = document.createElement('div');
      divider.className = 'context-cleared';
      divider.textContent = '— Context cleared —';
      outputPanel.appendChild(divider);
      currentTurn = null;
      loadContext();
      doAutoScroll();
    }).catch(() => {});
  });

  // --- Context tab refresh ---
  btnRefreshCtx.addEventListener('click', loadContext);

  function loadContext() {
    fetch('/api/context')
      .then(r => r.json())
      .then(data => {
        const size = data.size_bytes || 0;
        const sizeStr = size < 1024 ? size + ' B' : (size / 1024).toFixed(1) + ' KB';
        contextContent.innerHTML =
          '<div style="font-family:var(--pico-font-family-monospace);font-size:0.78rem;opacity:0.6;margin-bottom:0.5rem">'
          + esc(data.file_path || '') + '  (' + sizeStr + ')</div>'
          + '<pre>' + esc(data.context || '(empty)') + '</pre>';
      })
      .catch(err => {
        contextContent.innerHTML = '<pre>Error: ' + esc(err.message) + '</pre>';
      });
  }

  // --- Turn management ---
  function startNewTurn(prompt) {
    turnCount++;
    const turn = document.createElement('div');
    turn.className = 'turn collapsed';
    turn.dataset.turnId = turnCount;

    const summary = document.createElement('div');
    summary.className = 'turn-summary';
    summary.innerHTML = '<span class="label">user</span>' + esc(prompt);
    turn.appendChild(summary);

    const body = document.createElement('div');
    body.className = 'turn-body';
    turn.appendChild(body);

    turn.addEventListener('click', e => {
      turn.classList.toggle('collapsed');
    });

    outputPanel.appendChild(turn);
    currentTurn = turn;
    showPending();
  }

  function collapseTurn(turn) {
    if (!turn) return;
    turn.classList.add('collapsed');
    addAnswerButton(turn);
  }

  function addAnswerButton(turn) {
    if (turn.querySelector('.btn-answer')) return;
    const textBlocks = turn.querySelectorAll('.block-text');
    if (textBlocks.length === 0) return;

    // Gather all text content from the turn
    let fullText = '';
    textBlocks.forEach(b => { fullText += b.textContent + '\n'; });
    fullText = fullText.trimEnd();

    const btn = document.createElement('button');
    btn.className = 'btn-answer';
    btn.textContent = 'Answer Question';
    btn.addEventListener('click', e => {
      e.stopPropagation();
      enterAnswerMode(fullText);
    });
    turn.appendChild(btn);
  }

  function enterAnswerMode(text) {
    answerMode = true;
    document.querySelector('header').classList.add('answer-mode');
    btnCancelAnswer.style.display = 'inline-block';
    promptInput.value = text;
    promptInput.focus();
  }

  function exitAnswerMode() {
    answerMode = false;
    document.querySelector('header').classList.remove('answer-mode');
    btnCancelAnswer.style.display = 'none';
  }

  btnCancelAnswer.addEventListener('click', () => {
    promptInput.value = '';
    exitAnswerMode();
  });

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

      case 'rate_limit':
        block.classList.add('block-rate-limit');
        block.textContent = 'rate_limit_event: ' + content;
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
          btnSend.style.display = isRunning ? 'none' : '';
          btnStop.style.display = isRunning ? '' : 'none';
          promptInput.disabled = isRunning;
          if (!isRunning) {
            removePending();
            if (currentTurn) collapseTurn(currentTurn);
          }
          break;

        case 'init':
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

  // --- Escape HTML ---
  function esc(s) {
    if (!s) return '';
    const div = document.createElement('div');
    div.textContent = s;
    return div.innerHTML;
  }
})();
