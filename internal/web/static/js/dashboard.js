/**
 * AGENTIC CONTROL CENTER - CLIENT ENGINE
 * Pure Vanilla JavaScript: CSRF, Real-Time SSE, Concurrency Matrix, Steering Stream.
 */

// ---------------------------------------------------------------------------
// CSRF Protection Interceptor
// ---------------------------------------------------------------------------
function getCsrfToken() {
    const match = document.cookie.match(/(?:^|;\s*)_csrf_token=([^;]+)/);
    return match ? decodeURIComponent(match[1]) : '';
}

const _origFetch = window.fetch;
window.fetch = function(url, init) {
    init = init || {};
    const method = (init.method || 'GET').toUpperCase();
    if (method !== 'GET' && method !== 'HEAD') {
        const token = getCsrfToken();
        if (token) {
            if (!init.headers) {
                init.headers = { 'X-CSRF-Token': token };
            } else if (init.headers instanceof Headers) {
                if (!init.headers.has('X-CSRF-Token')) {
                    init.headers.set('X-CSRF-Token', token);
                }
            } else if (Array.isArray(init.headers)) {
                init.headers.push(['X-CSRF-Token', token]);
            } else if (typeof init.headers === 'object') {
                init.headers['X-CSRF-Token'] = token;
            }
        }
    }
    return _origFetch.call(this, url, init);
};

document.addEventListener('htmx:configRequest', function(evt) {
    const method = (evt.detail.verb || '').toUpperCase();
    if (method !== 'GET' && method !== 'HEAD') {
        evt.detail.headers['X-CSRF-Token'] = getCsrfToken();
    }
});

// ---------------------------------------------------------------------------
// Theme Management (Persisted Dark / Light Mode)
// ---------------------------------------------------------------------------
function toggleTheme() {
    const isLight = document.documentElement.classList.toggle('light-theme');
    if (isLight) {
        document.documentElement.classList.remove('dark');
    } else {
        document.documentElement.classList.add('dark');
    }
    localStorage.setItem('theme_preference', isLight ? 'light' : 'dark');
    updateThemeUI(isLight);
}

function updateThemeUI(isLight) {
    const iconEl = document.getElementById('theme-toggle-icon');
    const btn = document.getElementById('theme-toggle-btn');
    if (iconEl) iconEl.textContent = isLight ? '☀️' : '🌙';
    if (btn) btn.title = isLight ? 'Switch to Dark Mode' : 'Switch to Light Mode';
}

// ---------------------------------------------------------------------------
// String & Markdown Helpers
// ---------------------------------------------------------------------------
function escapeHtml(str) {
    if (!str) return '';
    return str
        .replace(/&/g, "&amp;")
        .replace(/</g, "&lt;")
        .replace(/>/g, "&gt;")
        .replace(/"/g, "&quot;")
        .replace(/'/g, "&#039;");
}

function getRawContent(el) {
    if (!el) return '';
    const b64 = el.getAttribute('data-b64');
    if (b64) {
        try {
            return decodeURIComponent(escape(atob(b64)));
        } catch(e) {
            try { return atob(b64); } catch(e2) {}
        }
    }
    return el.getAttribute('data-raw') || el.textContent || '';
}

function cleanOutcomeText(str) {
    if (!str) return '';
    let text = str;
    // Strip leading model tags e.g. [gemini-3.1-flash-lite]
    text = text.replace(/^\[[\w\.\-]+\]\s*/i, '');
    // Strip leading [Final Outcome]
    text = text.replace(/^\[Final Outcome\]\s*/i, '');
    return text.trim();
}

function normalizeMarkdown(str) {
    return cleanOutcomeText(str);
}

function renderMarkdown(str) {
    if (!str) return '';
    const cleaned = cleanOutcomeText(str);
    if (typeof marked !== 'undefined' && typeof marked.parse === 'function') {
        try {
            return marked.parse(cleaned, { breaks: true, gfm: true });
        } catch(e) {
            return escapeHtml(cleaned);
        }
    }
    return '<p>' + escapeHtml(cleaned).replace(/\n/g, '<br>') + '</p>';
}

function enhanceCodeBlocks(container) {
    if (!container) return;
    if (typeof hljs !== 'undefined') {
        container.querySelectorAll('pre code').forEach(block => {
            try { hljs.highlightElement(block); } catch(e) {}
        });
    }
}

function fetchSchedules() {
    const list = document.getElementById('schedules-list');
    if (!list) return;
    fetch('/api/schedules')
        .then(res => res.text())
        .then(html => {
            if (html.trim()) {
                list.innerHTML = html;
            }
        })
        .catch(err => console.warn('Fetch schedules warning:', err));
}

// ---------------------------------------------------------------------------
// Navigation & Tab Switching
// ---------------------------------------------------------------------------
function switchTab(tabId) {
    document.querySelectorAll('.tab-pane').forEach(el => el.classList.remove('active'));
    document.querySelectorAll('.nav-tab-btn').forEach(btn => btn.classList.remove('active'));

    const targetPane = document.getElementById('tab-content-' + tabId);
    const targetBtn = document.getElementById('tab-btn-' + tabId);

    if (targetPane) targetPane.classList.add('active');
    if (targetBtn) targetBtn.classList.add('active');

    window.location.hash = tabId;

    if (tabId === 'audit') fetchAuditExplorerData();
    if (tabId === 'studio' && typeof fetchStudioTools === 'function') fetchStudioTools();
    if (tabId === 'network' && typeof initNetworkVisualizer === 'function') setTimeout(initNetworkVisualizer, 50);
    if (tabId === 'scheduler') fetchSchedules();
}

function switchDrawerTab(tabName) {
    document.querySelectorAll('.drawer-pane').forEach(el => el.classList.remove('active'));
    document.querySelectorAll('.drawer-tab-btn').forEach(btn => btn.classList.remove('active'));

    const targetPane = document.getElementById('drawer-pane-' + tabName);
    const targetBtn = document.getElementById('drawer-btn-' + tabName);

    if (targetPane) targetPane.classList.add('active');
    if (targetBtn) targetBtn.classList.add('active');

    if (tabName === 'artifacts') {
        fetchLatestArtifacts();
    }
}

function fetchLatestArtifacts() {
    const list = document.getElementById('artifacts-list');
    if (!list) return;
    fetch('/api/artifacts?format=html')
        .then(res => res.text())
        .then(html => {
            if (html.trim()) {
                list.innerHTML = html;
            }
        })
        .catch(err => console.warn('Fetch artifacts warning:', err));
}

// ---------------------------------------------------------------------------
// Real-Time Log Processing & Telemetry
// ---------------------------------------------------------------------------
window.knownAgents = new Set();
window.currentLogCategory = 'all';

function processNewLogEntries() {
    const logs = document.querySelectorAll('#console-logs .log-entry');
    logs.forEach(log => {
        if (log.dataset.rendered === 'true') return;
        log.dataset.rendered = 'true';

        const sender = log.getAttribute('data-sender') || '';
        const recipient = log.getAttribute('data-recipient') || '';
        const eventType = log.getAttribute('data-event-type') || 'INFO';
        const tool = log.getAttribute('data-tool') || '';

        // Register known dynamic agents
        [sender, recipient].forEach(a => {
            if (a && a !== 'USER' && a !== 'SYSTEM' && a !== 'WEB' && a !== 'WARNING' && !window.knownAgents.has(a)) {
                window.knownAgents.add(a);
                const select = document.getElementById('agent-filter-select');
                if (select) {
                    const opt = document.createElement('option');
                    opt.value = a;
                    opt.textContent = a;
                    select.appendChild(opt);
                }
            }
        });

        processLogEntry(log, tool, eventType);
        applyLogFiltersToElement(log);

        const mdContainer = log.querySelector('.md-content');
        const raw = mdContainer ? getRawContent(mdContainer) : '';
        if (typeof updateParallelismSwimlane === 'function') {
            updateParallelismSwimlane(sender, recipient, raw, eventType, tool);
        }
        if (typeof dispatchLogToChatView === 'function') {
            dispatchLogToChatView(sender, recipient, eventType, tool, raw);
        }
    });
}

function processLogEntry(log, tool, eventType) {
    const mdContainer = log.querySelector('.md-content');
    if (!mdContainer) return;
    const raw = getRawContent(mdContainer);
    const renderedHTML = renderMarkdown(raw);

    if (tool || eventType === 'TOOL_CALL') {
        const toolName = tool || 'tool_call';
        mdContainer.innerHTML = `
            <details style="margin: 0.25rem 0; border-radius: var(--radius-sm); border: 1px solid var(--border-subtle); background: var(--bg-app); overflow: hidden;">
                <summary style="padding: 0.35rem 0.65rem; font-family: var(--font-mono); font-size: 0.7rem; font-weight: 700; color: var(--text-secondary); cursor: pointer; display: flex; align-items: center; justify-content: space-between; user-select: none; background: var(--bg-surface);">
                    <span style="display: flex; align-items: center; gap: 0.4rem;">
                        <span style="color: var(--status-cyan);">⚡ Tool Call:</span>
                        <span style="padding: 0.1rem 0.4rem; border-radius: 4px; background: var(--border-subtle); color: var(--text-primary);">${escapeHtml(toolName)}</span>
                    </span>
                    <span style="font-size: 0.65rem; color: var(--text-dim);">Expand ➔</span>
                </summary>
                <div style="padding: 0.65rem; font-family: var(--font-mono); font-size: 0.72rem; color: var(--text-secondary); overflow-x: auto; border-top: 1px solid var(--border-subtle);">${renderedHTML}</div>
            </details>
        `;
    } else {
        mdContainer.innerHTML = renderedHTML;
    }

    if (typeof emitLiveNetworkPulse === 'function') {
        emitLiveNetworkPulse(log.getAttribute('data-sender') || '', log.getAttribute('data-recipient') || '');
    }
}

// Log Filtering
function setLogCategoryFilter(category) {
    window.currentLogCategory = category;
    document.querySelectorAll('#log-filter-bar button').forEach(btn => {
        btn.style.background = 'transparent';
        btn.style.color = 'var(--text-muted)';
    });
    const activeBtn = document.getElementById('filter-btn-' + category);
    if (activeBtn) {
        activeBtn.style.background = 'var(--bg-surface-elevated)';
        activeBtn.style.color = 'var(--text-primary)';
    }
    applyLogFilters();
}

function applyLogFilters() {
    const logs = document.querySelectorAll('#console-logs .log-entry');
    logs.forEach(applyLogFiltersToElement);
}

function applyLogFiltersToElement(log) {
    const category = window.currentLogCategory || 'all';
    const agentFilter = (document.getElementById('agent-filter-select')?.value || 'all').toLowerCase();
    const query = (document.getElementById('log-search-input')?.value || '').toLowerCase();

    const sender = (log.getAttribute('data-sender') || '').toLowerCase();
    const recipient = (log.getAttribute('data-recipient') || '').toLowerCase();
    const eventType = (log.getAttribute('data-event-type') || '').toLowerCase();
    const text = log.textContent.toLowerCase();

    let matchesCategory = true;
    if (category === 'tool_call') matchesCategory = (eventType === 'tool_call' || log.getAttribute('data-tool'));
    else if (category === 'supervisor') matchesCategory = (eventType === 'qa_eval' || sender.includes('supervisor'));
    else if (category === 'dispatch') matchesCategory = (eventType === 'task_dispatch' || recipient.includes('agent'));

    let matchesAgent = (agentFilter === 'all') || (sender === agentFilter) || (recipient === agentFilter);
    let matchesQuery = !query || text.includes(query);

    log.style.display = (matchesCategory && matchesAgent && matchesQuery) ? 'flex' : 'none';
}

// ---------------------------------------------------------------------------
// Concurrency Matrix & Execution Swimlanes
// ---------------------------------------------------------------------------
window.activeParallelAgents = {};
window.agentTimers = {};
window.agentInvocations = {};

const NON_WORKER_ENTITIES = new Set([
    'USER', 'WEB', 'SYSTEM', 'security-guardrail', 'SUBTASK', 'SCHEDULER', 'WARNING', 'UNKNOWN', 'planner-agent', 'triage-agent', 'supervisor-agent'
]);

window.updateParallelismSwimlane = function(sender, recipient, content, eventType, activeTool) {
    if (!sender && !recipient) return;
    if (sender === 'security-guardrail' || sender === 'SYSTEM' || (content && content.includes('System Warnings'))) return;

    const now = Date.now();

    // 1. Overall task completion / final outcome: mark ALL active agents done
    const isSwarmFinalOutcome = (eventType === 'TASK_COMPLETE') || (content && (content.includes('[Final Outcome]') || content.includes('Verdict: DONE')));
    if (isSwarmFinalOutcome) {
        Object.keys(window.activeParallelAgents).forEach(agentId => {
            const statusEl = document.getElementById('swimlane-status-' + agentId);
            const barEl = document.getElementById('swimlane-bar-' + agentId);
            if (statusEl) statusEl.textContent = 'DONE';
            if (barEl) {
                barEl.style.width = '100%';
                barEl.style.background = 'var(--status-emerald-dim)';
            }
            if (window.agentTimers[agentId]) {
                clearInterval(window.agentTimers[agentId]);
                delete window.agentTimers[agentId];
            }
        });
        window.activeParallelAgents = {};
        updateConcurrencyBadge();
        return;
    }

    // 2. Individual subtask / tool execution completion: sender completed its work
    const isSubtaskComplete = (eventType === 'SUBTASK_COMPLETE') || 
                              (recipient === 'SUBTASK') ||
                              (sender && !NON_WORKER_ENTITIES.has(sender) && recipient === 'planner-agent' && !activeTool && eventType !== 'TOOL_CALL');
    if (isSubtaskComplete && sender) {
        const agentId = sender;
        const statusEl = document.getElementById('swimlane-status-' + agentId);
        const barEl = document.getElementById('swimlane-bar-' + agentId);
        if (statusEl) statusEl.textContent = 'DONE';
        if (barEl) {
            barEl.style.width = '100%';
            barEl.style.background = 'var(--status-emerald-dim)';
        }
        if (window.agentTimers[agentId]) {
            clearInterval(window.agentTimers[agentId]);
            delete window.agentTimers[agentId];
        }
        delete window.activeParallelAgents[agentId];
        updateConcurrencyBadge();
        return;
    }

    // 3. Parallel worker agents actively running / dispatched
    let targetAgents = [];
    if (eventType === 'TASK_DISPATCH' || ((sender === 'triage-agent' || sender === 'planner-agent') && recipient)) {
        if (recipient && !NON_WORKER_ENTITIES.has(recipient)) {
            targetAgents = [recipient];
        }
    } else {
        targetAgents = [sender, recipient].filter(a => a && !NON_WORKER_ENTITIES.has(a));
    }

    targetAgents.forEach(agentId => {
        let swimlane = document.getElementById('swimlane-' + agentId);
        if (!swimlane) {
            const matrixContainer = document.getElementById('parallelism-matrix');
            if (matrixContainer) {
                const row = document.createElement('div');
                row.id = 'swimlane-' + agentId;
                row.className = 'swimlane-row';
                row.innerHTML = `
                    <div class="swimlane-agent-id" title="${escapeHtml(agentId)}">
                        <span>⚡</span> ${escapeHtml(agentId)}
                    </div>
                    <div class="swimlane-bar-track">
                        <div id="swimlane-bar-${escapeHtml(agentId)}" class="swimlane-bar-fill"></div>
                        <span id="swimlane-status-${escapeHtml(agentId)}" class="swimlane-status-text">IDLE</span>
                    </div>
                    <span id="swimlane-timer-${escapeHtml(agentId)}" class="swimlane-timer">0.0s</span>
                `;
                matrixContainer.appendChild(row);
                swimlane = row;
            }
        }

        window.agentInvocations[agentId] = (window.agentInvocations[agentId] || 0) + 1;
        const statusEl = document.getElementById('swimlane-status-' + agentId);
        const barEl = document.getElementById('swimlane-bar-' + agentId);
        const timerEl = document.getElementById('swimlane-timer-' + agentId);

        window.activeParallelAgents[agentId] = now;
        if (statusEl) statusEl.textContent = activeTool ? ('⚡ ' + activeTool) : 'RUNNING';
        if (barEl) {
            barEl.style.width = '70%';
            barEl.style.background = 'var(--status-cyan-dim)';
        }
        if (!window.agentTimers[agentId]) {
            const startTime = now;
            window.agentTimers[agentId] = setInterval(() => {
                if (timerEl) {
                    const elapsed = ((Date.now() - startTime) / 1000).toFixed(1);
                    timerEl.textContent = elapsed + 's';
                }
            }, 100);
        }
    });

    updateConcurrencyBadge();
};

function updateConcurrencyBadge() {
    const count = Object.keys(window.activeParallelAgents).length;
    const badge = document.getElementById('parallel-concurrency-badge');
    const headerPill = document.getElementById('chat-live-indicator');

    if (badge) badge.textContent = `Active Threads: ${count} Parallel`;
    if (headerPill) {
        if (count > 0) {
            headerPill.innerHTML = `<span class="status-dot" style="background: var(--status-cyan); box-shadow: 0 0 6px var(--status-cyan);"></span><span>${count} Active</span>`;
        } else {
            headerPill.innerHTML = `<span class="status-dot"></span><span>Swarm Ready</span>`;
        }
    }
}

function resetParallelismMatrix() {
    Object.values(window.agentTimers).forEach(clearInterval);
    window.agentTimers = {};
    window.activeParallelAgents = {};
    window.agentInvocations = {};
    const matrix = document.getElementById('parallelism-matrix');
    if (matrix) matrix.innerHTML = '';
    updateConcurrencyBadge();
}

// ---------------------------------------------------------------------------
// Interactive Conversational Steering Stream
// ---------------------------------------------------------------------------
window.activeChatTurn = null;

function setPromptTask(text) {
    const input = document.getElementById('chat-composer-input');
    if (input) {
        input.value = text;
        input.focus();
        input.style.height = 'auto';
        input.style.height = Math.min(input.scrollHeight, 180) + 'px';
    }
}

function dispatchQuickPrompt(text, mode) {
    const modeSelect = document.getElementById('chat-orchestration-mode');
    if (modeSelect && mode) modeSelect.value = mode;
    setPromptTask(text);
    submitChatComposer();
}

function scrollChatToBottom() {
    const scrollContainer = document.getElementById('dashboard-subview-chat');
    if (scrollContainer) {
        scrollContainer.scrollTop = scrollContainer.scrollHeight;
    }
}

function createAssistantTurn(mode) {
    const chatMessages = document.getElementById('chat-messages');
    if (!chatMessages) return null;

    const hero = document.getElementById('chat-hero');
    if (hero) hero.style.display = 'none';

    const turn = document.createElement('div');
    turn.className = 'chat-turn turn-assistant';
    const turnId = 'turn-' + Date.now();
    turn.id = turnId;

    turn.innerHTML = `
        <div class="turn-assistant-header">
            <div class="agent-badge-group">
                <div class="agent-avatar">✦</div>
                <span class="agent-name">Swarm Intelligence</span>
                <span style="font-family: var(--font-mono); font-size: 0.65rem; color: var(--text-dim); text-transform: uppercase;">(${escapeHtml(mode || 'autonomous')})</span>
            </div>
            <span class="turn-timer" style="font-family: var(--font-mono); font-size: 0.7rem; color: var(--text-dim);">thinking...</span>
        </div>
        <details class="swarm-thinking is-running" open>
            <summary class="swarm-thinking-summary">
                <span class="thinking-title"><span>⚡</span> <span>Swarm reasoning and tool orchestration</span></span>
                <span style="font-size: 0.68rem; color: var(--text-dim);">Details ▼</span>
            </summary>
            <div class="thinking-timeline"></div>
        </details>
        <div class="synthesized-answer hidden"></div>
    `;

    chatMessages.appendChild(turn);

    const turnObj = {
        id: turnId,
        container: turn,
        thinkingAccordion: turn.querySelector('.swarm-thinking'),
        thinkingTitle: turn.querySelector('.thinking-title'),
        thinkingTimeline: turn.querySelector('.thinking-timeline'),
        synthesizedAnswer: turn.querySelector('.synthesized-answer'),
        timerEl: turn.querySelector('.turn-timer'),
        startTime: Date.now(),
        steps: []
    };

    window.activeChatTurn = turnObj;
    scrollChatToBottom();
    return turnObj;
}

function submitChatComposer() {
    const textarea = document.getElementById('chat-composer-input');
    if (!textarea) return;

    const content = textarea.value.trim();
    if (!content) return;

    const modeSelect = document.getElementById('chat-orchestration-mode');
    const mode = modeSelect ? modeSelect.value : 'autonomous';
    const fileInput = document.getElementById('chat-file-input');
    const file = fileInput && fileInput.files && fileInput.files[0] ? fileInput.files[0] : null;

    // Render User Turn in Feed
    const chatMessages = document.getElementById('chat-messages');
    if (chatMessages) {
        const hero = document.getElementById('chat-hero');
        if (hero) hero.style.display = 'none';

        const userTurn = document.createElement('div');
        userTurn.className = 'chat-turn turn-user';
        userTurn.innerHTML = `
            <div class="turn-user-meta">
                <span>OPERATOR INSTRUCTION</span>
                <span>${new Date().toLocaleTimeString()}</span>
            </div>
            <div style="font-size: 0.88rem; line-height: 1.5; white-space: pre-wrap;">${escapeHtml(content)}</div>
            ${file ? `<div class="composer-file-chip" style="margin-top: 0.45rem;">📎 ${escapeHtml(file.name)}</div>` : ''}
        `;
        chatMessages.appendChild(userTurn);
    }

    // Reset Composer Input
    textarea.value = '';
    textarea.style.height = 'auto';
    clearComposerFile();

    // Prepare Assistant Turn
    createAssistantTurn(mode);

    // Build Form Data and Dispatch
    const formData = new FormData();
    formData.append('content', content);
    formData.append('orchestration_mode', mode);
    if (file) formData.append('file', file);

    const sendBtn = document.getElementById('chat-send-btn');
    if (sendBtn) sendBtn.disabled = true;

    fetch('/api/task', {
        method: 'POST',
        body: formData
    })
    .then(res => res.json())
    .catch(err => console.error('Dispatch task warning:', err))
    .finally(() => {
        if (sendBtn) sendBtn.disabled = false;
    });
}

function handleComposerFileSelected(input) {
    if (input && input.files && input.files[0]) {
        const chip = document.getElementById('composer-file-chip');
        const nameEl = document.getElementById('composer-file-name');
        if (chip && nameEl) {
            nameEl.textContent = input.files[0].name;
            chip.classList.remove('hidden');
        }
    }
}

function clearComposerFile() {
    const fileInput = document.getElementById('chat-file-input');
    const chip = document.getElementById('composer-file-chip');
    if (fileInput) fileInput.value = '';
    if (chip) chip.classList.add('hidden');
}

function clearChatThread() {
    fetch('/api/session/new', { method: 'POST' })
        .then(() => {
            const chatMessages = document.getElementById('chat-messages');
            if (chatMessages) chatMessages.innerHTML = '';
            const hero = document.getElementById('chat-hero');
            if (hero) hero.style.display = 'flex';
            resetParallelismMatrix();
            window.activeChatTurn = null;
        })
        .catch(err => console.error('Session reset failed:', err));
}

function dispatchLogToChatView(sender, recipient, eventType, tool, raw) {
    if (tool === 'schedule_task' || tool === 'cancel_schedule' || (raw && (raw.includes('schedule') || raw.includes('sched-')))) {
        if (typeof fetchSchedules === 'function') fetchSchedules();
    }

    if (!window.activeChatTurn) {
        if (recipient === 'USER' && raw && (raw.includes('[Final Outcome]') || eventType === 'TASK_COMPLETE' || raw.includes('Task Completion Report') || raw.includes('Comprehensive Report'))) {
            const turns = document.querySelectorAll('#chat-messages .turn-assistant');
            if (turns.length > 0) {
                const lastTurn = turns[turns.length - 1];
                const ans = lastTurn.querySelector('.synthesized-answer');
                if (ans) {
                    let outcomeText = cleanOutcomeText(raw);
                    ans.innerHTML = renderMarkdown(outcomeText);
                    enhanceCodeBlocks(ans);
                    ans.classList.remove('hidden');
                    const th = lastTurn.querySelector('.swarm-thinking');
                    if (th) th.open = false;
                    Object.keys(window.activeParallelAgents).forEach(agentId => {
                        const statusEl = document.getElementById('swimlane-status-' + agentId);
                        const barEl = document.getElementById('swimlane-bar-' + agentId);
                        if (statusEl) statusEl.textContent = 'DONE';
                        if (barEl) {
                            barEl.style.width = '100%';
                            barEl.style.background = 'var(--status-emerald-dim)';
                        }
                        if (window.agentTimers[agentId]) {
                            clearInterval(window.agentTimers[agentId]);
                            delete window.agentTimers[agentId];
                        }
                    });
                    window.activeParallelAgents = {};
                    updateConcurrencyBadge();
                    scrollChatToBottom();
                }
                return;
            } else {
                // If no turn exists yet (background job, page reload, or CLI dispatch), create assistant turn
                const newTurn = createAssistantTurn('autonomous');
                if (newTurn) {
                    let outcomeText = cleanOutcomeText(raw);
                    newTurn.synthesizedAnswer.innerHTML = renderMarkdown(outcomeText);
                    enhanceCodeBlocks(newTurn.synthesizedAnswer);
                    newTurn.synthesizedAnswer.classList.remove('hidden');
                    newTurn.thinkingTitle.innerHTML = `<span>✓</span> <span>Task Completed</span>`;
                    newTurn.thinkingAccordion.open = false;
                    Object.keys(window.activeParallelAgents).forEach(agentId => {
                        const statusEl = document.getElementById('swimlane-status-' + agentId);
                        const barEl = document.getElementById('swimlane-bar-' + agentId);
                        if (statusEl) statusEl.textContent = 'DONE';
                        if (barEl) {
                            barEl.style.width = '100%';
                            barEl.style.background = 'var(--status-emerald-dim)';
                        }
                        if (window.agentTimers[agentId]) {
                            clearInterval(window.agentTimers[agentId]);
                            delete window.agentTimers[agentId];
                        }
                    });
                    window.activeParallelAgents = {};
                    window.activeChatTurn = null;
                    updateConcurrencyBadge();
                    scrollChatToBottom();
                }
                return;
            }
        }
        if (sender && sender !== 'USER' && sender !== 'SYSTEM' && sender !== 'WEB') {
            createAssistantTurn('autonomous');
        }
    }

    const turn = window.activeChatTurn;
    if (!turn) return;

    const isDone = (recipient === 'USER' && (eventType === 'TASK_COMPLETE' || (raw && (raw.includes('[Final Outcome]') || raw.includes('Verdict: DONE') || raw.includes('Task Completion Report') || raw.includes('Final Report'))))) || (raw && raw.includes('[Final Outcome]'));

    if (isDone) {
        let outcomeText = cleanOutcomeText(raw);
        turn.synthesizedAnswer.innerHTML = renderMarkdown(outcomeText);
        enhanceCodeBlocks(turn.synthesizedAnswer);
        turn.synthesizedAnswer.classList.remove('hidden');

        const duration = ((Date.now() - turn.startTime) / 1000).toFixed(1);
        turn.thinkingTitle.innerHTML = `<span>✓</span> <span>Orchestrated in ${duration}s (${turn.steps.length} steps)</span>`;
        if (turn.timerEl) turn.timerEl.textContent = duration + 's';
        turn.thinkingAccordion.open = false;

        // Finalize all active swimlanes and clear active parallel threads
        Object.keys(window.activeParallelAgents).forEach(agentId => {
            const statusEl = document.getElementById('swimlane-status-' + agentId);
            const barEl = document.getElementById('swimlane-bar-' + agentId);
            if (statusEl) statusEl.textContent = 'DONE';
            if (barEl) {
                barEl.style.width = '100%';
                barEl.style.background = 'var(--status-emerald-dim)';
            }
            if (window.agentTimers[agentId]) {
                clearInterval(window.agentTimers[agentId]);
                delete window.agentTimers[agentId];
            }
        });
        window.activeParallelAgents = {};
        window.activeChatTurn = null;
        updateConcurrencyBadge();
        scrollChatToBottom();
    } else {
        let stepText = '';
        if (eventType === 'TOOL_CALL' || tool) {
            const toolName = tool || 'tool';
            stepText = `<strong>${escapeHtml(sender)}</strong> invoked <span class="tool-step-badge">⚡ ${escapeHtml(toolName)}</span>`;
        } else if (eventType === 'SUBTASK_COMPLETE') {
            stepText = `<strong>${escapeHtml(sender)}</strong> completed sub-task ✓`;
        } else if (eventType === 'QA_EVAL') {
            stepText = `<strong>${escapeHtml(sender)}</strong> evaluated QA quality check 🔍`;
        } else if (eventType === 'TASK_DISPATCH') {
            stepText = `<strong>${escapeHtml(sender)}</strong> routed sub-task to <strong>${escapeHtml(recipient)}</strong> ➔`;
        } else {
            const snippet = raw.length > 90 ? raw.slice(0, 90) + '...' : raw;
            stepText = `<strong>${escapeHtml(sender)}</strong>: ${escapeHtml(snippet)}`;
        }

        turn.steps.push(stepText);
        const stepDiv = document.createElement('div');
        stepDiv.className = 'timeline-step';
        stepDiv.innerHTML = `<span>➔</span> <span>${stepText}</span>`;
        turn.thinkingTimeline.appendChild(stepDiv);

        turn.thinkingTitle.innerHTML = `<span>⚡</span> <span>Swarm Active: <strong>${escapeHtml(sender)}</strong> ${tool ? '• ' + escapeHtml(tool) : ''}</span>`;
        if (turn.timerEl) {
            const duration = ((Date.now() - turn.startTime) / 1000).toFixed(1);
            turn.timerEl.textContent = duration + 's';
        }
        scrollChatToBottom();
    }
}

// ---------------------------------------------------------------------------
// Human-in-the-Loop Approval Handling
// ---------------------------------------------------------------------------
function toggleChatApprovalsList() {
    const list = document.getElementById('chat-approvals-list');
    const icon = document.getElementById('chat-approvals-toggle-icon');
    const text = document.getElementById('chat-approvals-toggle-text');
    if (!list) return;

    if (list.classList.contains('hidden')) {
        list.classList.remove('hidden');
        if (icon) icon.textContent = '▼';
        if (text) text.textContent = 'Collapse';
    } else {
        list.classList.add('hidden');
        if (icon) icon.textContent = '▶';
        if (text) text.textContent = 'Expand';
    }
}

async function submitApprovalDecision(corrId, decision) {
    const formData = new FormData();
    formData.append('correlation_id', corrId);
    formData.append('decision', decision);

    try {
        const res = await fetch('/api/hitl/respond', {
            method: 'POST',
            body: formData
        });
        const html = await res.text();
        const card = document.getElementById('approval-' + corrId);
        if (card) {
            card.outerHTML = html;
        }
        syncApprovalsToChat();
    } catch(err) {
        console.error('Error submitting approval:', err);
    }
}

function syncApprovalsToChat() {
    const list = document.getElementById('chat-approvals-list');
    const badge = document.getElementById('chat-approvals-badge');
    const section = document.getElementById('chat-approvals-section');
    const indicator = document.getElementById('subview-approval-indicator');

    if (!list) return;

    const cards = list.querySelectorAll('[id^="approval-"]:not([data-resolved="true"])');
    const pendingCount = cards.length;

    if (badge) badge.textContent = `${pendingCount} Pending`;
    if (indicator) {
        indicator.textContent = pendingCount;
        if (pendingCount > 0) indicator.classList.remove('hidden');
        else indicator.classList.add('hidden');
    }

    if (section) {
        if (pendingCount > 0) section.classList.remove('hidden');
        else section.classList.add('hidden');
    }
}

// ---------------------------------------------------------------------------
// Audit Explorer Data
// ---------------------------------------------------------------------------
function fetchAuditExplorerData() {
    fetch('/api/audit/summary')
        .then(res => res.json())
        .then(data => {
            const costEl = document.getElementById('audit-kpi-total-cost');
            const tokenEl = document.getElementById('audit-kpi-total-tokens');
            const durEl = document.getElementById('audit-kpi-avg-duration');
            const tasksEl = document.getElementById('audit-kpi-total-tasks');

            if (costEl && data.total_cost_usd !== undefined) costEl.textContent = '$' + data.total_cost_usd.toFixed(4);
            if (tokenEl && data.total_tokens !== undefined) tokenEl.textContent = data.total_tokens.toLocaleString() + ' Tokens';
            if (durEl && data.avg_duration_ms !== undefined) durEl.textContent = Math.round(data.avg_duration_ms) + ' ms';
            if (tasksEl && data.total_tasks !== undefined) tasksEl.textContent = data.total_tasks + ' Turns';
        })
        .catch(err => console.warn('Audit summary warning:', err));

    const q = document.getElementById('audit-search-input')?.value || '';
    fetch('/api/audit/records?q=' + encodeURIComponent(q))
        .then(res => res.json())
        .then(records => {
            const tbody = document.getElementById('audit-records-table-body');
            if (!tbody) return;
            if (!records || records.length === 0) {
                tbody.innerHTML = '<tr><td colspan="8" style="padding: 2rem; text-align: center; color: var(--text-dim); font-style: italic;">No audit records found.</td></tr>';
                return;
            }
            tbody.innerHTML = records.map(r => `
                <tr>
                    <td style="font-family: var(--font-mono); font-size: 0.7rem;">${escapeHtml(r.timestamp)}</td>
                    <td style="font-family: var(--font-mono); font-size: 0.7rem; color: var(--text-dim);">${escapeHtml(r.correlation_id)}</td>
                    <td style="font-weight: 700; color: var(--text-primary); font-family: var(--font-mono);">${escapeHtml(r.agent_id)}</td>
                    <td style="font-family: var(--font-mono); font-size: 0.7rem;">${escapeHtml(r.model_node)}</td>
                    <td style="font-family: var(--font-mono); font-size: 0.7rem;">${r.prompt_tokens} / ${r.completion_tokens}</td>
                    <td style="font-family: var(--font-mono); font-weight: 700; color: var(--status-emerald);">$${(r.estimated_cost_usd || 0).toFixed(4)}</td>
                    <td style="font-family: var(--font-mono); font-size: 0.7rem;">${r.duration_ms}ms</td>
                    <td><span style="font-family: var(--font-mono); font-size: 0.65rem; font-weight: 700; padding: 0.1rem 0.4rem; border-radius: 4px; background: var(--status-emerald-dim); color: var(--status-emerald);">${escapeHtml(r.status)}</span></td>
                </tr>
            `).join('');
        })
        .catch(err => console.warn('Audit records warning:', err));
}

function exportAuditCSV() { window.open('/api/audit/export', '_blank'); }
function pruneAuditRecords() {
    if (confirm('Prune audit records older than 30 days?')) {
        fetch('/api/audit/prune', { method: 'POST' })
            .then(() => fetchAuditExplorerData())
            .catch(err => alert('Prune failed: ' + err));
    }
}

// ---------------------------------------------------------------------------
// Lifecycle & Initializations
// ---------------------------------------------------------------------------
document.addEventListener('DOMContentLoaded', function() {
    const savedTheme = localStorage.getItem('theme_preference');
    if (savedTheme === 'light') {
        document.documentElement.classList.add('light-theme');
        document.documentElement.classList.remove('dark');
        updateThemeUI(true);
    } else {
        document.documentElement.classList.add('dark');
        document.documentElement.classList.remove('light-theme');
        updateThemeUI(false);
    }

    const composerTextarea = document.getElementById('chat-composer-input');
    if (composerTextarea) {
        composerTextarea.addEventListener('input', function() {
            this.style.height = 'auto';
            this.style.height = Math.min(this.scrollHeight, 180) + 'px';
        });
        composerTextarea.addEventListener('keydown', function(e) {
            if (e.key === 'Enter' && !e.shiftKey) {
                e.preventDefault();
                submitChatComposer();
            }
        });
    }

    syncApprovalsToChat();

    // Replay/swap event listener
    document.addEventListener('htmx:afterSwap', function(e) {
        if (e.target && (e.target.id === 'console-logs' || e.target.classList?.contains('log-entry'))) {
            processNewLogEntries();
        }
        if (e.target && (e.target.id === 'chat-approvals-list' || e.target.id?.startsWith('approval-'))) {
            syncApprovalsToChat();
        }
    });

    document.addEventListener('htmx:sseMessage', function() {
        processNewLogEntries();
    });

    if (window.location.hash) {
        const hash = window.location.hash.replace('#', '');
        if (['dashboard', 'studio', 'audit', 'network', 'config', 'scheduler'].includes(hash)) {
            switchTab(hash);
        }
    }
});

// Explicit Global Exports
window.toggleTheme = toggleTheme;
window.updateThemeUI = updateThemeUI;
window.switchTab = switchTab;
window.switchDrawerTab = switchDrawerTab;
window.submitChatComposer = submitChatComposer;
window.clearChatThread = clearChatThread;
window.dispatchQuickPrompt = dispatchQuickPrompt;
window.handleComposerFileSelected = handleComposerFileSelected;
window.clearComposerFile = clearComposerFile;
window.toggleChatApprovalsList = toggleChatApprovalsList;
window.submitApprovalDecision = submitApprovalDecision;
window.syncApprovalsToChat = syncApprovalsToChat;
window.fetchAuditExplorerData = fetchAuditExplorerData;
window.exportAuditCSV = exportAuditCSV;
window.pruneAuditRecords = pruneAuditRecords;
window.setLogCategoryFilter = setLogCategoryFilter;
window.applyLogFilters = applyLogFilters;
window.setPromptTask = setPromptTask;
window.renderMarkdown = renderMarkdown;
window.normalizeMarkdown = normalizeMarkdown;
window.escapeHtml = escapeHtml;
window.getRawContent = getRawContent;
window.fetchLatestArtifacts = fetchLatestArtifacts;