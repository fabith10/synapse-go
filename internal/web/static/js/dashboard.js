function getCsrfToken() {
            const match = document.cookie.match(/(?:^|;\s*)_csrf_token=([^;]+)/);
            return match ? decodeURIComponent(match[1]) : '';
        }

        // Global fetch interceptor to inject X-CSRF-Token on all mutating requests
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

function toggleTheme() {
            const isLight = document.body.classList.toggle('light-theme');
            if (isLight) {
                document.documentElement.classList.remove('dark');
                document.documentElement.classList.add('light-theme');
            } else {
                document.documentElement.classList.add('dark');
                document.documentElement.classList.remove('light-theme');
            }
            localStorage.setItem('theme_preference', isLight ? 'light' : 'dark');
            updateThemeUI(isLight);
        }

        function updateThemeUI(isLight) {
            const iconEl = document.getElementById('theme-toggle-icon');
            const textEl = document.getElementById('theme-toggle-text');
            if (iconEl) iconEl.textContent = isLight ? '🌙' : '☀️';
            if (textEl) textEl.textContent = isLight ? 'Dark Mode' : 'Light Mode';
        }

        function escapeHtml(str) {
            if (!str) return '';
            return str
                .replace(/&/g, "&amp;")
                .replace(/</g, "&lt;")
                .replace(/>/g, "&gt;")
                .replace(/"/g, "&quot;")
                .replace(/'/g, "&#039;");
        }

        if (typeof marked !== 'undefined' && typeof marked.setOptions === 'function') {
            marked.setOptions({
                breaks: true,
                gfm: true
            });
        }

        function getRawContent(el) {
            if (!el) return '';
            const b64 = el.getAttribute('data-b64');
            if (b64) {
                try {
                    return decodeURIComponent(escape(atob(b64)));
                } catch(e) {
                    try {
                        return atob(b64);
                    } catch(e2) {}
                }
            }
            return el.getAttribute('data-raw') || el.textContent || '';
        }

        function normalizeMarkdown(str) {
            if (!str) return '';
            let text = str;

            // 1. Separate inline headers: e.g. "something. ## Header" -> "something.\n\n## Header"
            text = text.replace(/([^\n])\s*(#{1,6}\s+)/g, '$1\n\n$2');

            // 2. Separate text that was squashed directly after a header on the same line:
            // e.g. "## Executive Summary The primary objective" -> "## Executive Summary\n\nThe primary objective"
            text = text.replace(/(^|\n)(#{1,6}\s+[^\n#]+?)\s+([A-Z][a-z]+|\d+\.|\*|-)/g, function(match, prefix, heading, nextWord) {
                return prefix + heading + '\n\n' + nextWord;
            });

            // 3. Separate bullet points that were squashed into the previous line:
            // e.g. "operational: * Data & Analysis: ... * Content & Research:" -> "operational:\n* Data & Analysis:\n* Content & Research:"
            text = text.replace(/([^\n])\s*([*•-]\s+)/g, '$1\n$2');

            // 4. Separate numbered lists that were squashed into the previous line:
            // e.g. "tasks: 1. First 2. Second" -> "tasks:\n1. First\n2. Second"
            text = text.replace(/([^\n])\s*(\b\d+\.\s+)/g, '$1\n$2');

            return text;
        }

        function renderMarkdown(str) {
            if (!str) return '';
            const normalized = normalizeMarkdown(str);
            if (typeof marked !== 'undefined' && typeof marked.parse === 'function') {
                try {
                    return marked.parse(normalized, { breaks: true, gfm: true });
                } catch (e) {
                    console.warn('marked.parse failed, falling back:', e);
                    return escapeHtml(normalized).replace(/\n/g, '<br>');
                }
            }
            return escapeHtml(normalized).replace(/\n/g, '<br>');
        }

        document.addEventListener('DOMContentLoaded', function() {
            // Restore theme preference
            const saved = localStorage.getItem('theme_preference');
            if (saved === 'light') {
                document.body.classList.add('light-theme');
                document.documentElement.classList.remove('dark');
                document.documentElement.classList.add('light-theme');
                updateThemeUI(true);
            }

            // Seed known agents from the live registry so custom Studio agents
            // appear in the filter dropdown immediately, not only after telemetry.
            fetch('/api/agents')
                .then(r => r.json())
                .then(data => {
                    const select = document.getElementById('agent-filter-select');
                    Object.keys(data.agents || {}).forEach(id => {
                        if (!window.knownAgents.has(id)) {
                            window.knownAgents.add(id);
                            if (select) {
                                const opt = document.createElement('option');
                                opt.value = id;
                                opt.textContent = id;
                                select.appendChild(opt);
                            }
                        }
                    });
                })
                .catch(() => {}); // non-critical; live telemetry will still populate the list

            // Process any log entries present in the DOM on initial page load / refresh
            processNewLogEntries();

            // Set up MutationObserver on #console-logs to instantly catch any new elements
            const consoleLogsEl = document.getElementById('console-logs');
            if (consoleLogsEl) {
                const observer = new MutationObserver(function() {
                    processNewLogEntries();
                });
                observer.observe(consoleLogsEl, { childList: true, subtree: true });
            }
        });

        document.body.addEventListener('htmx:afterRequest', function(evt) {
            if (evt.detail.successful) {
                var form = document.getElementById('task-form');
                if (form && evt.target.id === 'task-form') {
                    form.reset();
                    var chosen = document.getElementById('file-chosen');
                    if (chosen) {
                        chosen.textContent = "No file chosen";
                    }
                }
                var schedForm = document.getElementById('schedule-form');
                if (schedForm && evt.target.id === 'schedule-form') {
                    schedForm.reset();
                }
            }
        });


        // Bug fix: clear orphaned setInterval timers, but ONLY on the "New Session" button swap
        // (htmx:afterSwap fires on #console-logs for every SSE log message too — don't reset on those)
        document.body.addEventListener('htmx:afterSwap', function(evt) {
            if (evt.target && evt.target.id === 'console-logs') {
                const cfg = evt.detail && evt.detail.requestConfig;
                if (cfg && cfg.path === '/api/session/new' && typeof resetParallelismMatrix === 'function') {
                    resetParallelismMatrix();
                }
            }
        });

        document.addEventListener('htmx:sseBeforeMessage', function(e) {
            setTimeout(function() {
                var consoleDiv = document.getElementById('console-logs');
                if (consoleDiv) {
                    consoleDiv.scrollTop = consoleDiv.scrollHeight;
                }
            }, 50);
        });

        window.currentLogCategory = 'all';
        window.knownAgents = new Set(['triage-agent', 'planner-agent', 'supervisor-agent', 'developer-agent', 'researcher-agent', 'browser-agent', 'quant-agent', 'etl-agent', 'writer-agent', 'sales-agent', 'email-agent', 'excel-agent', 'generalist-agent']);

        function processNewLogEntries() {
            const logs = document.querySelectorAll('#console-logs .log-entry');
            logs.forEach(log => {
                if (log.dataset.rendered === 'true') return;
                log.dataset.rendered = 'true';

                const sender = log.getAttribute('data-sender') || '';
                const recipient = log.getAttribute('data-recipient') || '';
                const eventType = log.getAttribute('data-event-type') || 'INFO';
                const tool = log.getAttribute('data-tool') || '';

                // Add sender and recipient to agent filter dropdown
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

        document.addEventListener('htmx:afterSwap', function(e) {
            if (e.target && (e.target.id === 'console-logs' || (e.target.classList && e.target.classList.contains('log-entry')))) {
                processNewLogEntries();
            }
        });

        document.addEventListener('htmx:afterSettle', function(e) {
            if (e.target && (e.target.id === 'console-logs' || (e.target.classList && e.target.classList.contains('log-entry')))) {
                processNewLogEntries();
            }
        });

        document.addEventListener('htmx:sseMessage', function(e) {
            processNewLogEntries();
        });

        function processLogEntry(log, tool, eventType) {
            const mdContainer = log.querySelector('.md-content');
            if (!mdContainer) return;
            const raw = getRawContent(mdContainer);
            const renderedHTML = renderMarkdown(raw);
            if (tool || eventType === 'TOOL_CALL') {
                const toolName = tool || 'tool_call';
                mdContainer.innerHTML = `
                    <details class="my-1 rounded-lg border border-zinc-800 bg-zinc-900/40 overflow-hidden shadow-inner group/details">
                        <summary class="px-3 py-1.5 text-[10px] font-mono font-bold text-zinc-200 cursor-pointer bg-zinc-900/80 hover:bg-zinc-800/80 flex items-center justify-between select-none">
                            <span class="flex items-center gap-1.5">
                                <span class="text-zinc-400">⚡ Tool Call:</span>
                                <span class="px-1.5 py-0.5 rounded bg-zinc-800 border border-zinc-700 text-zinc-100">${toolName}</span>
                            </span>
                            <span class="text-[9px] text-zinc-400 font-normal group-open/details:hidden">Expand Payload ➔</span>
                            <span class="text-[9px] text-zinc-400 font-normal hidden group-open/details:inline">Collapse ⯅</span>
                        </summary>
                        <div class="p-3 text-[10px] font-mono text-zinc-300 overflow-x-auto bg-zinc-950/80 border-t border-zinc-800 leading-relaxed">${renderedHTML}</div>
                    </details>
                `;
            } else {
                mdContainer.innerHTML = renderedHTML;
            }

            // Network pulse
            if (typeof emitLiveNetworkPulse === 'function') {
                const sender = log.getAttribute('data-sender') || '';
                const recipient = log.getAttribute('data-recipient') || '';
                emitLiveNetworkPulse(sender, recipient);
            }
        }

        window.activeParallelAgents = {};
        window.agentTimers = {};
        window.agentInvocations = {};

        window.updateParallelismSwimlane = function(sender, recipient, content, eventType, activeTool) {
            if (!sender && !recipient) return;

            if (sender === 'security-guardrail' || sender === 'SYSTEM' || (content && (content.includes('WARNING') || content.includes('System Warnings')))) {
                return;
            }

            const targetAgents = [sender, recipient].filter(a => a && a !== 'USER' && a !== 'WEB' && a !== 'SYSTEM' && a !== 'security-guardrail');
            const now = Date.now();

            targetAgents.forEach(agentId => {
                let swimlane = document.getElementById('swimlane-' + agentId);
                if (!swimlane) {
                    const matrixContainer = document.getElementById('parallelism-matrix');
                    if (matrixContainer) {
                        const row = document.createElement('div');
                        row.id = 'swimlane-' + agentId;
                        row.className = 'swimlane-row flex items-center gap-3 p-2.5 rounded-xl bg-zinc-950/60 border border-zinc-800/80 transition-all';
                        row.innerHTML = `
                            <div class="w-36 shrink-0 flex items-center justify-between pr-2 select-none">
                                <div class="flex items-center gap-2">
                                    <span class="w-2 h-2 rounded-full bg-emerald-500"></span>
                                    <span class="font-bold text-zinc-200 truncate">${agentId}</span>
                                </div>
                                <span id="swimlane-count-${agentId}" class="text-[9px] font-mono font-bold bg-zinc-900 border border-zinc-800 text-zinc-400 px-1.5 py-0.5 rounded hidden">×1</span>
                            </div>
                            <div class="flex-1 bg-zinc-900/90 rounded-lg h-7 p-1 relative overflow-hidden flex items-center border border-zinc-800">
                                <div id="swimlane-bar-${agentId}" class="h-full rounded bg-emerald-500/20 border border-emerald-500/40 w-0 transition-all duration-300 flex items-center px-2 text-[10px] text-emerald-300 font-semibold truncate"></div>
                                <span id="swimlane-status-${agentId}" class="absolute inset-0 flex items-center justify-center text-[10px] text-zinc-500 font-mono">IDLE</span>
                            </div>
                            <span id="swimlane-timer-${agentId}" class="w-16 text-right text-[10px] text-zinc-500 font-mono select-none">0.0s</span>
                        `;
                        matrixContainer.appendChild(row);
                        swimlane = row;
                    }
                }

                window.agentInvocations[agentId] = (window.agentInvocations[agentId] || 0) + 1;
                const invCount = window.agentInvocations[agentId];

                const countBadge = document.getElementById('swimlane-count-' + agentId);
                if (countBadge) {
                    countBadge.textContent = '×' + invCount;
                    countBadge.classList.remove('hidden');
                }

                const bar = document.getElementById('swimlane-bar-' + agentId);
                const status = document.getElementById('swimlane-status-' + agentId);

                const isDone = eventType === 'TASK_COMPLETE' || (content && (content.includes('Verdict: DONE') || content.includes('action":"done')));

                if (!isDone) {
                    if (!activeParallelAgents[agentId]) {
                        activeParallelAgents[agentId] = { startTime: now, state: 'RUNNING', invocations: invCount };
                    }

                    let label = 'RUNNING ⚡';
                    let barColor = 'bg-emerald-500/30 border-emerald-500/60 text-emerald-300';
                    let statusColor = 'text-emerald-400 font-mono font-bold animate-pulse';
                    let width = '70%';

                    if (eventType === 'TOOL_CALL' || activeTool) {
                        const toolName = activeTool || 'tool';
                        label = `TOOL: ${toolName} ⚡`;
                        barColor = 'bg-zinc-800 border-zinc-600 text-zinc-200';
                        statusColor = 'text-zinc-200 font-mono font-bold animate-pulse';
                        width = '85%';
                    } else if (eventType === 'QA_EVAL') {
                        label = 'QA CHECK 🔍';
                        barColor = 'bg-amber-500/30 border-amber-500/60 text-amber-300';
                        statusColor = 'text-amber-300 font-mono font-bold animate-pulse';
                        width = '90%';
                    }

                    if (status) {
                        status.textContent = label;
                        status.className = 'absolute inset-0 flex items-center justify-center text-[10px] ' + statusColor;
                    }
                    if (bar) {
                        bar.style.width = width;
                        bar.className = 'h-full rounded transition-all duration-300 flex items-center px-2 text-[10px] font-semibold truncate animate-pulse ' + barColor;
                    }
                    if (swimlane) {
                        swimlane.className = 'swimlane-row flex items-center gap-3 p-2.5 rounded-xl bg-zinc-900 border border-emerald-500/50 shadow-lg shadow-emerald-500/10 transition-all';
                    }

                    if (!agentTimers[agentId]) {
                        agentTimers[agentId] = setInterval(function() {
                            const timerEl = document.getElementById('swimlane-timer-' + agentId);
                            if (timerEl && activeParallelAgents[agentId]) {
                                const elapsed = ((Date.now() - activeParallelAgents[agentId].startTime) / 1000).toFixed(1);
                                timerEl.textContent = elapsed + 's';
                                timerEl.className = 'w-16 text-right text-[10px] text-emerald-400 font-mono font-bold';
                            }
                        }, 100);
                    }
                } else {
                    if (activeParallelAgents[agentId]) {
                        activeParallelAgents[agentId].state = 'COMPLETED';
                    }
                    if (status) {
                        status.textContent = `COMPLETED (${invCount}x) ✅`;
                        status.className = 'absolute inset-0 flex items-center justify-center text-[10px] text-cyan-400 font-mono font-bold';
                    }
                    if (bar) {
                        bar.style.width = '100%';
                        bar.className = 'h-full rounded bg-cyan-500/30 border border-cyan-500/60 transition-all duration-300 flex items-center px-2 text-[10px] text-cyan-300 font-semibold truncate';
                    }
                    if (swimlane) {
                        swimlane.className = 'swimlane-row flex items-center gap-3 p-2.5 rounded-xl bg-zinc-950/60 border border-cyan-500/30 transition-all';
                    }
                    if (agentTimers[agentId]) {
                        clearInterval(agentTimers[agentId]);
                        delete agentTimers[agentId];
                    }
                    setTimeout(function() {
                        delete activeParallelAgents[agentId];
                        updateConcurrencyBadge();
                    }, 8000);
                }
            });

            updateConcurrencyBadge();
        };

        window.setLogCategoryFilter = function(cat) {
            window.currentLogCategory = cat;
            const isLight = document.body.classList.contains('light-theme');
            ['all', 'tool_call', 'supervisor', 'dispatch'].forEach(c => {
                const btn = document.getElementById('filter-btn-' + c);
                if (btn) {
                    if (c === cat) {
                        btn.className = isLight 
                            ? "px-2.5 py-1 text-[9px] font-mono font-bold uppercase tracking-wider bg-zinc-950 text-white rounded-md cursor-pointer transition-all active:scale-95 shadow-sm"
                            : "px-2.5 py-1 text-[9px] font-mono font-bold uppercase tracking-wider bg-white text-zinc-950 rounded-md cursor-pointer transition-all active:scale-95 shadow-sm";
                    } else {
                        btn.className = isLight
                            ? "px-2.5 py-1 text-[9px] font-mono font-medium uppercase tracking-wider bg-slate-200 border border-slate-300 text-slate-700 hover:text-slate-900 rounded-md cursor-pointer transition-all active:scale-95"
                            : "px-2.5 py-1 text-[9px] font-mono font-medium uppercase tracking-wider bg-zinc-900 border border-zinc-800 text-zinc-400 hover:text-zinc-200 rounded-md cursor-pointer transition-all active:scale-95";
                    }
                }
            });
            applyLogFilters();
        };

        window.applyLogFilters = function() {
            const entries = document.querySelectorAll('.log-entry');
            entries.forEach(entry => applyLogFiltersToElement(entry));
        };

        window.applyLogFiltersToElement = function(el) {
            const sender = (el.getAttribute('data-sender') || '').toLowerCase();
            const recipient = (el.getAttribute('data-recipient') || '').toLowerCase();
            const category = (el.getAttribute('data-category') || 'general').toLowerCase();
            const eventType = (el.getAttribute('data-event-type') || 'INFO').toLowerCase();
            const activeTool = (el.getAttribute('data-tool') || '').toLowerCase();
            const textContent = (el.textContent || '').toLowerCase();

            const agentSelect = document.getElementById('agent-filter-select');
            const targetAgent = agentSelect ? agentSelect.value.toLowerCase() : 'all';

            const searchInput = document.getElementById('log-search-input');
            const searchQuery = searchInput ? searchInput.value.toLowerCase().trim() : '';

            let matchesAgent = targetAgent === 'all' || sender === targetAgent || recipient === targetAgent;

            let matchesCategory = false;
            const filterCat = window.currentLogCategory;
            if (!filterCat || filterCat === 'all') {
                matchesCategory = true;
            } else if (filterCat === 'tool_call') {
                matchesCategory = category === 'tool_call' || eventType === 'tool_call' || activeTool !== '' || el.querySelector('.tool-badge') !== null || el.querySelector('details') !== null || textContent.includes('tool call');
            } else if (filterCat === 'supervisor') {
                matchesCategory = category === 'supervisor' || eventType === 'qa_eval' || sender.includes('supervisor') || recipient.includes('supervisor') || textContent.includes('supervisor') || textContent.includes('qa check');
            } else if (filterCat === 'dispatch') {
                matchesCategory = category === 'dispatch' || eventType === 'task_dispatch' || textContent.includes('dispatch') || textContent.includes('sub-task') || textContent.includes('task:');
            } else {
                matchesCategory = category === filterCat || eventType.includes(filterCat);
            }

            let matchesSearch = !searchQuery || textContent.includes(searchQuery);

            if (matchesAgent && matchesCategory && matchesSearch) {
                el.style.display = 'flex';
            } else {
                el.style.display = 'none';
            }
        };

        function updateConcurrencyBadge() {
            const badge = document.getElementById('parallel-concurrency-badge');
            const count = Object.keys(activeParallelAgents).length;
            if (badge) {
                badge.textContent = `Active Threads: ${count} Parallel`;
                if (count > 0) {
                    badge.className = 'text-[9px] font-mono text-emerald-400 bg-emerald-950/60 border border-emerald-800 px-2.5 py-1 rounded-md font-bold animate-pulse';
                } else {
                    badge.className = 'text-[9px] font-mono text-zinc-400 bg-zinc-950 border border-zinc-800 px-2.5 py-1 rounded-md';
                }
            }
        }

        window.resetParallelismMatrix = function() {
            Object.keys(agentTimers).forEach(id => clearInterval(agentTimers[id]));
            window.agentTimers = {};
            window.activeParallelAgents = {};
            const statuses = document.querySelectorAll('[id^="swimlane-status-"]');
            statuses.forEach(s => {
                s.textContent = 'IDLE';
                s.className = 'absolute inset-0 flex items-center justify-center text-[10px] text-zinc-500 font-mono';
            });
            const bars = document.querySelectorAll('[id^="swimlane-bar-"]');
            bars.forEach(b => b.style.width = '0%');
            const timers = document.querySelectorAll('[id^="swimlane-timer-"]');
            timers.forEach(t => {
                t.textContent = '0.0s';
                t.className = 'w-16 text-right text-[10px] text-zinc-500 font-mono';
            });
            updateConcurrencyBadge();
        };


        function switchTab(tabId) {
            const panes = ['dashboard', 'studio', 'audit', 'network', 'config', 'scheduler'];
            panes.forEach(p => {
                const paneEl = document.getElementById('tab-content-' + p);
                const btnEl = document.getElementById('tab-btn-' + p);
                if (paneEl) {
                    if (p === tabId) {
                        paneEl.classList.remove('hidden');
                    } else {
                        paneEl.classList.add('hidden');
                    }
                }
                if (btnEl) {
                    if (p === tabId) {
                        btnEl.className = "border-b-2 border-white py-3.5 px-2 text-xs font-bold uppercase tracking-wider text-white flex items-center gap-2 transition-all cursor-pointer";
                    } else {
                        btnEl.className = "border-b-2 border-transparent py-3.5 px-2 text-xs font-bold uppercase tracking-wider text-zinc-400 hover:text-zinc-200 flex items-center gap-2 transition-all cursor-pointer";
                    }
                }
            });
            if (tabId === 'studio') {
                fetchStudioTools();
            } else if (tabId === 'audit') {
                fetchAuditExplorerData();
            } else if (tabId === 'network') {
                initNetworkVisualizer();
            }
            try { history.replaceState(null, null, '#' + tabId); } catch(e) {}
        }

        function fetchAuditExplorerData() {
            fetch('/api/audit/summary')
                .then(res => res.json())
                .then(data => {
                    const costEl = document.getElementById('audit-kpi-total-cost');
                    const tokensEl = document.getElementById('audit-kpi-total-tokens');
                    const durEl = document.getElementById('audit-kpi-avg-duration');
                    const tasksEl = document.getElementById('audit-kpi-total-tasks');

                    if (costEl) costEl.textContent = '$' + (data.total_cost_usd || 0).toFixed(4);
                    if (tokensEl) tokensEl.textContent = (data.total_tokens || 0).toLocaleString() + ' Tokens';
                    if (durEl) durEl.textContent = (data.avg_duration_ms || 0) + ' ms';
                    if (tasksEl) tasksEl.textContent = (data.total_tasks || 0) + ' Turns';

                    const agentBarsEl = document.getElementById('audit-agent-cost-bars');
                    if (agentBarsEl) {
                        agentBarsEl.innerHTML = '';
                        const agentMap = data.cost_by_agent || {};
                        const keys = Object.keys(agentMap);
                        if (keys.length === 0) {
                            agentBarsEl.innerHTML = '<div class="text-xs text-zinc-500 italic">No agent execution costs logged yet.</div>';
                        } else {
                            const maxCost = Math.max(...Object.values(agentMap), 0.0001);
                            keys.forEach(agentId => {
                                const cost = agentMap[agentId];
                                const pct = Math.min(100, Math.max(8, (cost / maxCost) * 100));
                                const row = document.createElement('div');
                                row.className = 'space-y-1';
                                row.innerHTML = 
                                    '<div class="flex justify-between text-xs font-mono">' +
                                        '<span class="text-zinc-200 font-bold">' + agentId + '</span>' +
                                        '<span class="text-emerald-400 font-bold">$' + cost.toFixed(4) + '</span>' +
                                    '</div>' +
                                    '<div class="w-full bg-zinc-950 rounded-full h-2 overflow-hidden border border-zinc-800">' +
                                        '<div class="bg-gradient-to-r from-emerald-500 to-teal-400 h-2 rounded-full" style="width: ' + pct + '%"></div>' +
                                    '</div>';
                                agentBarsEl.appendChild(row);
                            });
                        }
                    }

                    const modelBarsEl = document.getElementById('audit-model-cost-bars');
                    if (modelBarsEl) {
                        modelBarsEl.innerHTML = '';
                        const modelMap = data.cost_by_model || {};
                        const keys = Object.keys(modelMap);
                        if (keys.length === 0) {
                            modelBarsEl.innerHTML = '<div class="text-xs text-zinc-500 italic">No model node costs logged yet.</div>';
                        } else {
                            const maxCost = Math.max(...Object.values(modelMap), 0.0001);
                            keys.forEach(modelName => {
                                const cost = modelMap[modelName];
                                const pct = Math.min(100, Math.max(8, (cost / maxCost) * 100));
                                const row = document.createElement('div');
                                row.className = 'space-y-1';
                                row.innerHTML = 
                                    '<div class="flex justify-between text-xs font-mono">' +
                                        '<span class="text-zinc-200 font-bold">' + modelName + '</span>' +
                                        '<span class="text-zinc-100 font-bold">$' + cost.toFixed(4) + '</span>' +
                                    '</div>' +
                                    '<div class="w-full bg-zinc-950 rounded-full h-2 overflow-hidden border border-zinc-800">' +
                                        '<div class="bg-white h-2 rounded-full" style="width: ' + pct + '%"></div>' +
                                    '</div>';
                                modelBarsEl.appendChild(row);
                            });
                        }
                    }
                })
                .catch(err => console.error('Failed to fetch audit summary:', err));

            const searchVal = document.getElementById('audit-search-input')?.value || '';
            fetch('/api/audit/records?search=' + encodeURIComponent(searchVal))
                .then(res => res.json())
                .then(data => {
                    const tbody = document.getElementById('audit-records-table-body');
                    if (!tbody) return;
                    tbody.innerHTML = '';

                    const records = data.records || [];
                    if (records.length === 0) {
                        tbody.innerHTML = '<tr><td colspan="8" class="py-6 text-center text-zinc-500 italic">No audit records found matching your filter.</td></tr>';
                        return;
                    }

                    records.forEach(rec => {
                        const tr = document.createElement('tr');
                        tr.className = 'hover:bg-zinc-900/60 transition-all font-mono';
                        const dateStr = rec.created_at ? new Date(rec.created_at).toLocaleTimeString() : 'N/A';
                        const statusBadge = rec.status === 'COMPLETED' ? 
                            '<span class="px-2 py-0.5 rounded-full bg-emerald-500/20 text-emerald-400 border border-emerald-500/30 text-[9px] font-bold">COMPLETED</span>' :
                            '<span class="px-2 py-0.5 rounded-full bg-amber-500/20 text-amber-400 border border-amber-500/30 text-[9px] font-bold">' + rec.status + '</span>';

                        tr.innerHTML = 
                            '<td class="py-2.5 px-3 text-zinc-400 text-[11px]">' + dateStr + '</td>' +
                            '<td class="py-2.5 px-3 font-bold text-zinc-200 text-[11px] truncate max-w-[120px]" title="' + rec.correlation_id + '">' + rec.correlation_id + '</td>' +
                            '<td class="py-2.5 px-3 text-zinc-100 text-[11px] font-bold">' + rec.agent_id + '</td>' +
                            '<td class="py-2.5 px-3 text-zinc-300 text-[11px]">' + rec.model_name + '</td>' +
                            '<td class="py-2.5 px-3 text-zinc-400 text-[11px]">' + rec.prompt_tokens + ' / ' + rec.completion_tokens + '</td>' +
                            '<td class="py-2.5 px-3 text-emerald-400 font-bold text-[11px]">$' + rec.total_cost_usd.toFixed(4) + '</td>' +
                            '<td class="py-2.5 px-3 text-amber-300 text-[11px]">' + rec.execution_duration_ms + ' ms</td>' +
                            '<td class="py-2.5 px-3">' + statusBadge + '</td>';
                        tbody.appendChild(tr);
                    });
                })
                .catch(err => console.error('Failed to fetch audit records:', err));
        }

        function exportAuditCSV() {
            window.location.href = '/api/audit/export';
        }

        function pruneAuditRecords() {
            if (!confirm('Are you sure you want to prune audit records older than 30 days?')) return;
            fetch('/api/audit/prune', { method: 'POST' })
                .then(res => res.json())
                .then(data => {
                    if (data.success) {
                        alert('✅ ' + data.message);
                        fetchAuditExplorerData();
                    } else {
                        alert('❌ Prune failed: ' + data.error);
                    }
                })
                .catch(err => alert('❌ Prune failed: ' + err));
        }

        window.addEventListener('DOMContentLoaded', function() {
            if (window.location.hash) {
                const hash = window.location.hash.replace('#', '');
                if (['dashboard', 'studio', 'audit', 'network', 'config', 'scheduler'].includes(hash)) {
                    switchTab(hash);
                }
            }
        });

        function setPromptTask(text) {
            const el = document.getElementById('content');
            if (el) {
                el.value = text;
            }
            const chatInput = document.getElementById('chat-composer-input');
            if (chatInput) {
                chatInput.value = text;
                chatInput.style.height = 'auto';
                chatInput.style.height = Math.min(chatInput.scrollHeight, 180) + 'px';
                chatInput.focus();
            }
        }

        /* ----------------------------------------------------------------- */
        /* Conversational Interface (Claude & Gemini Paradigm)               */
        /* ----------------------------------------------------------------- */
        window.activeChatTurn = null;

        function switchDashboardSubview(subview) {
            const chatView = document.getElementById('dashboard-subview-chat');
            const matrixView = document.getElementById('dashboard-subview-matrix');
            const chatBtn = document.getElementById('subview-btn-chat');
            const matrixBtn = document.getElementById('subview-btn-matrix');

            if (subview === 'matrix') {
                if (chatView) chatView.classList.add('hidden');
                if (matrixView) matrixView.classList.remove('hidden');
                if (chatBtn) {
                    chatBtn.className = "px-3.5 py-1.5 rounded-lg text-xs font-medium flex items-center gap-2 text-zinc-400 hover:text-zinc-200 transition-all cursor-pointer";
                }
                if (matrixBtn) {
                    matrixBtn.className = "px-3.5 py-1.5 rounded-lg text-xs font-semibold flex items-center gap-2 bg-white text-zinc-950 shadow-sm transition-all cursor-pointer";
                }
            } else {
                if (chatView) chatView.classList.remove('hidden');
                if (matrixView) matrixView.classList.add('hidden');
                if (chatBtn) {
                    chatBtn.className = "px-3.5 py-1.5 rounded-lg text-xs font-semibold flex items-center gap-2 bg-white text-zinc-950 shadow-sm transition-all cursor-pointer";
                }
                if (matrixBtn) {
                    matrixBtn.className = "px-3.5 py-1.5 rounded-lg text-xs font-medium flex items-center gap-2 text-zinc-400 hover:text-zinc-200 transition-all cursor-pointer";
                }
            }
        }

        function enhanceCodeBlocks(container) {
            if (!container) return;
            const codeBlocks = container.querySelectorAll('pre code');
            codeBlocks.forEach(codeEl => {
                if (codeEl.dataset.enhanced === 'true') return;
                codeEl.dataset.enhanced = 'true';

                if (typeof hljs !== 'undefined') {
                    try {
                        hljs.highlightElement(codeEl);
                    } catch(e) {}
                }

                const pre = codeEl.parentElement;
                if (!pre || (pre.parentElement && pre.parentElement.classList.contains('code-container'))) return;

                let lang = 'code';
                codeEl.classList.forEach(cls => {
                    if (cls.startsWith('language-')) {
                        lang = cls.replace('language-', '');
                    }
                });

                const wrapper = document.createElement('div');
                wrapper.className = 'code-container';

                const header = document.createElement('div');
                header.className = 'code-header';
                header.innerHTML = `
                    <span class="font-bold text-zinc-400 font-mono text-[10px] uppercase">${lang}</span>
                    <button type="button" class="code-copy-btn">Copy</button>
                `;

                const copyBtn = header.querySelector('.code-copy-btn');
                copyBtn.addEventListener('click', function(e) {
                    e.stopPropagation();
                    navigator.clipboard.writeText(codeEl.textContent).then(() => {
                        copyBtn.textContent = 'Copied! ✓';
                        copyBtn.style.color = '#34d399';
                        setTimeout(() => {
                            copyBtn.textContent = 'Copy';
                            copyBtn.style.color = '';
                        }, 2000);
                    });
                });

                pre.parentNode.insertBefore(wrapper, pre);
                wrapper.appendChild(header);
                wrapper.appendChild(pre);
            });
        }

        function handleComposerFileSelected(input) {
            if (input && input.files && input.files[0]) {
                const file = input.files[0];
                const chip = document.getElementById('composer-file-chip');
                const nameEl = document.getElementById('composer-file-name');
                if (chip && nameEl) {
                    nameEl.textContent = file.name;
                    chip.classList.remove('hidden');
                }
            }
        }

        function clearComposerFile() {
            const input = document.getElementById('chat-file-input');
            if (input) input.value = '';
            const chip = document.getElementById('composer-file-chip');
            if (chip) chip.classList.add('hidden');
            const taskFileInput = document.getElementById('task-file');
            if (taskFileInput) taskFileInput.value = '';
        }

        function dispatchQuickPrompt(text, mode) {
            const input = document.getElementById('chat-composer-input');
            const modeSelect = document.getElementById('chat-orchestration-mode');
            if (input) input.value = text;
            if (modeSelect && mode) modeSelect.value = mode;
            submitChatComposer();
        }

        function createAssistantTurn(mode) {
            const messages = document.getElementById('chat-messages');
            if (!messages) return null;

            const turnId = 'turn-' + Date.now();
            const assistantTurn = document.createElement('div');
            assistantTurn.id = turnId;
            assistantTurn.className = 'assistant-turn flex gap-3 sm:gap-4 animate-in fade-in slide-in-from-bottom-2';

            assistantTurn.innerHTML = `
                <div class="w-8 h-8 rounded-xl bg-zinc-800 border border-zinc-700 p-[1px] shadow-sm shrink-0 mt-1">
                    <div class="w-full h-full bg-zinc-950 rounded-[11px] flex items-center justify-center text-xs font-bold text-zinc-100">
                        ✦
                    </div>
                </div>
                <div class="flex-1 space-y-4 min-w-0 max-w-full">
                    <!-- Swarm Thinking Accordion (Claude/Gemini Style) -->
                    <details class="swarm-thinking is-running group" open>
                        <summary class="px-4 py-2.5 text-xs font-mono font-medium cursor-pointer flex items-center justify-between select-none">
                            <span class="thinking-title flex items-center gap-2">
                                <span class="w-2 h-2 rounded-full bg-white animate-ping"></span>
                                <span class="font-semibold">Swarm Orchestrating...</span>
                            </span>
                            <span class="text-[10px] font-mono group-open:rotate-180 transition-transform">▾</span>
                        </summary>
                        <div class="thinking-timeline p-3.5 font-mono text-[11px] space-y-2 border-t">
                            <div class="timeline-step flex items-center gap-2">
                                <span class="step-icon">➔</span>
                                <span>Evaluating requirements &amp; routing to specialists...</span>
                            </div>
                        </div>
                    </details>

                    <!-- Final Synthesized Outcome Container -->
                    <div class="synthesized-answer assistant-bubble md-content hidden space-y-3">
                    </div>

                    <!-- Deliverables / Artifacts Container -->
                    <div class="turn-artifacts space-y-2 hidden">
                    </div>
                </div>
            `;

            messages.appendChild(assistantTurn);

            window.activeChatTurn = {
                id: turnId,
                startTime: Date.now(),
                steps: ['Evaluating requirements & routing to specialists...'],
                turnEl: assistantTurn,
                thinkingAccordion: assistantTurn.querySelector('.swarm-thinking'),
                thinkingTitle: assistantTurn.querySelector('.thinking-title'),
                thinkingTimeline: assistantTurn.querySelector('.thinking-timeline'),
                synthesizedAnswer: assistantTurn.querySelector('.synthesized-answer'),
                turnArtifacts: assistantTurn.querySelector('.turn-artifacts')
            };

            return window.activeChatTurn;
        }

        function submitChatComposer() {
            const input = document.getElementById('chat-composer-input');
            if (!input) return;
            const text = input.value.trim();
            const fileInput = document.getElementById('chat-file-input');
            const hasFile = fileInput && fileInput.files && fileInput.files.length > 0;

            if (!text && !hasFile) return;

            // Check if there is an active pending approval and the user typed an approval intent
            const trimmedLower = text.toLowerCase().trim();
            const isApprove = ['approve', 'approved', 'yes', 'proceed', 'go ahead', 'ok', 'sure', 'confirm', 'confirmed', 'continue', 'y'].includes(trimmedLower);
            const isReject = ['deny', 'denied', 'reject', 'rejected', 'cancel', 'no', 'stop', 'abort', 'n'].includes(trimmedLower);

            const pendingCard = document.querySelector('#chat-approvals-list [id^="chat-approval-"]:not([data-resolved="true"])') || document.querySelector('#chat-messages [id^="chat-msg-approval-"]:not([data-resolved="true"])');
            if ((isApprove || isReject) && pendingCard) {
                const corrId = pendingCard.id.replace('chat-approval-', '').replace('chat-msg-approval-', '');
                if (corrId) {
                    const hero = document.getElementById('chat-hero');
                    if (hero) hero.style.display = 'none';

                    const messages = document.getElementById('chat-messages');
                    const userTurn = document.createElement('div');
                    userTurn.className = 'flex justify-end gap-3 animate-in fade-in slide-in-from-bottom-2';
                    const timeStr = new Date().toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
                    userTurn.innerHTML = `
                        <div class="user-bubble max-w-[85%] px-5 py-3.5 space-y-1.5 text-sm leading-relaxed">
                            <div class="whitespace-pre-wrap font-medium">${escapeHtml(text)}</div>
                            <div class="bubble-timestamp text-[10px] font-mono text-right">${timeStr}</div>
                        </div>
                        <div class="w-8 h-8 rounded-full bg-zinc-800 border border-zinc-700 flex items-center justify-center text-xs font-bold text-zinc-200 shrink-0">
                            U
                        </div>
                    `;
                    messages.appendChild(userTurn);

                    submitApprovalDecision(corrId, isApprove ? 'APPROVED' : 'REJECTED');
                    input.value = '';
                    input.style.height = 'auto';
                    clearComposerFile();
                    return;
                }
            }

            const modeSelect = document.getElementById('chat-orchestration-mode');
            const mode = modeSelect ? modeSelect.value : 'autonomous';

            // Hide welcome hero
            const hero = document.getElementById('chat-hero');
            if (hero) hero.style.display = 'none';

            // Append user message bubble to chat
            const messages = document.getElementById('chat-messages');
            const userTurn = document.createElement('div');
            userTurn.className = 'flex justify-end gap-3 animate-in fade-in slide-in-from-bottom-2';

            let fileBadge = '';
            if (hasFile) {
                fileBadge = `<div class="bubble-file-chip inline-flex items-center gap-1.5 px-2.5 py-0.5 rounded-full text-[10px] font-mono mb-1">📎 ${escapeHtml(fileInput.files[0].name)}</div>`;
            }

            const timeStr = new Date().toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
            userTurn.innerHTML = `
                <div class="user-bubble max-w-[85%] px-5 py-3.5 space-y-1.5 text-sm leading-relaxed">
                    ${fileBadge}
                    <div class="whitespace-pre-wrap">${escapeHtml(text)}</div>
                    <div class="bubble-timestamp text-[10px] font-mono text-right">${timeStr}</div>
                </div>
                <div class="w-8 h-8 rounded-full bg-zinc-800 border border-zinc-700 flex items-center justify-center text-xs font-bold text-zinc-200 shrink-0">
                    U
                </div>
            `;
            messages.appendChild(userTurn);

            // Create assistant turn container with thinking accordion
            createAssistantTurn(mode);

            // Sync with backend form and dispatch
            const taskForm = document.getElementById('task-form');
            const contentInput = document.getElementById('content');
            const formMode = document.getElementById('orchestration_mode');
            const taskFileInput = document.getElementById('task-file');

            if (contentInput) contentInput.value = text;
            if (formMode) formMode.value = mode;

            if (hasFile && taskFileInput && fileInput.files[0]) {
                try {
                    const dataTransfer = new DataTransfer();
                    dataTransfer.items.add(fileInput.files[0]);
                    taskFileInput.files = dataTransfer.files;
                } catch(e) {}
            }

            // Reset input
            input.value = '';
            input.style.height = 'auto';
            clearComposerFile();

            // Submit task
            if (taskForm) {
                htmx.trigger(taskForm, 'submit');
            }

            window.scrollTo({ top: document.body.scrollHeight, behavior: 'smooth' });
        }

        function clearChatThread() {
            if (!confirm('Start a new session? This will reset the conversation thread.')) return;
            fetch('/api/session/new', { method: 'POST' })
                .then(() => {
                    const messages = document.getElementById('chat-messages');
                    if (messages) messages.innerHTML = '';
                    const hero = document.getElementById('chat-hero');
                    if (hero) hero.style.display = 'flex';
                    const logs = document.getElementById('console-logs');
                    if (logs) logs.innerHTML = '<div class="text-zinc-500 italic font-mono text-xs text-center py-8">[Ready to process autonomous agent telemetry...]</div>';
                    if (typeof resetParallelismMatrix === 'function') resetParallelismMatrix();
                    window.activeChatTurn = null;
                })
                .catch(err => console.error('Failed to reset session:', err));
        }

        function dispatchLogToChatView(sender, recipient, eventType, tool, raw) {
            if (!window.activeChatTurn) {
                if (recipient === 'USER' && raw && (raw.includes('[Final Outcome]') || eventType === 'TASK_COMPLETE')) {
                    const turns = document.querySelectorAll('#chat-messages .assistant-turn');
                    if (turns.length > 0) {
                        const lastTurn = turns[turns.length - 1];
                        const ans = lastTurn.querySelector('.synthesized-answer');
                        if (ans) {
                            let outcomeText = raw.replace(/^\[Final Outcome\]\s*/i, '').trim();
                            ans.innerHTML = renderMarkdown(outcomeText);
                            enhanceCodeBlocks(ans);
                            ans.classList.remove('hidden');
                            const th = lastTurn.querySelector('.swarm-thinking');
                            if (th) {
                                th.classList.remove('is-running');
                                th.open = false;
                            }
                        }
                    }
                    return;
                }
                if (sender && sender !== 'USER' && sender !== 'SYSTEM' && sender !== 'WEB') {
                    createAssistantTurn('autonomous');
                }
            }

            const turn = window.activeChatTurn;
            if (!turn) return;

            const isDone = (recipient === 'USER') || (eventType === 'TASK_COMPLETE') || (raw && (raw.includes('[Final Outcome]') || raw.includes('Verdict: DONE') || raw.includes('action":"done')));

            if (isDone) {
                let outcomeText = raw.replace(/^\[Final Outcome\]\s*/i, '').trim();
                turn.synthesizedAnswer.innerHTML = renderMarkdown(outcomeText);
                enhanceCodeBlocks(turn.synthesizedAnswer);
                turn.synthesizedAnswer.classList.remove('hidden');

                const duration = ((Date.now() - turn.startTime) / 1000).toFixed(1);
                turn.thinkingTitle.innerHTML = `<span class="flex items-center gap-2"><span class="orchestration-check">✓</span><span>Orchestrated in ${duration}s (${turn.steps.length} steps)</span></span>`;
                turn.thinkingAccordion.classList.remove('is-running');
                turn.thinkingAccordion.open = false; // collapse thought like Claude & Gemini

                window.activeChatTurn = null;

                const livePill = document.getElementById('chat-live-indicator');
                if (livePill) {
                    livePill.innerHTML = '<span class="w-1.5 h-1.5 rounded-full bg-emerald-400"></span><span class="hidden sm:inline">Swarm Ready</span>';
                }

                window.scrollTo({ top: document.body.scrollHeight, behavior: 'smooth' });
            } else {
                let stepText = '';
                if (eventType === 'TOOL_CALL' || tool) {
                    const toolName = tool || 'tool';
                    stepText = `<strong>${escapeHtml(sender)}</strong> invoked <span class="tool-step-badge px-1.5 py-0.5 rounded text-[10px]">⚡ ${escapeHtml(toolName)}</span>`;
                } else if (eventType === 'QA_EVAL') {
                    stepText = `<strong>${escapeHtml(sender)}</strong> completed quality assurance check 🔍`;
                } else if (eventType === 'TASK_DISPATCH') {
                    stepText = `<strong>${escapeHtml(sender)}</strong> routed sub-task to <strong>${escapeHtml(recipient)}</strong> ➔`;
                } else {
                    const snippet = raw.length > 100 ? raw.slice(0, 100) + '...' : raw;
                    stepText = `<strong>${escapeHtml(sender)}</strong>: ${escapeHtml(snippet)}`;
                }

                turn.steps.push(stepText);

                const stepDiv = document.createElement('div');
                stepDiv.className = 'timeline-step flex items-center gap-2 py-0.5';
                stepDiv.innerHTML = `<span class="step-icon">➔</span><span>${stepText}</span>`;
                turn.thinkingTimeline.appendChild(stepDiv);

                turn.thinkingTitle.innerHTML = `<span class="flex items-center gap-2"><span class="w-2 h-2 rounded-full bg-white animate-ping"></span><span>Swarm Active: <strong>${escapeHtml(sender)}</strong> ${tool ? '⚡ ' + escapeHtml(tool) : ''}</span></span>`;

                const livePill = document.getElementById('chat-live-indicator');
                if (livePill) {
                    livePill.innerHTML = `<span class="w-1.5 h-1.5 rounded-full bg-white animate-pulse"></span><span class="hidden sm:inline">${escapeHtml(sender)} Active</span>`;
                }
            }
        }

        // Toggle Conversational Approvals List
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

        // Submit Approval Decision via API and synchronize both views
        async function submitApprovalDecision(corrId, decision) {
            if (!corrId || !decision) return;

            const token = getCsrfToken();
            const formData = new FormData();
            formData.append('correlation_id', corrId);
            formData.append('decision', decision);
            formData.append('_csrf_token', token);

            try {
                const resp = await fetch('/api/hitl/respond', {
                    method: 'POST',
                    headers: {
                        'X-CSRF-Token': token
                    },
                    body: formData
                });
                if (resp.ok) {
                    const html = await resp.text();

                    // Update Conversational approval card in queue
                    const chatCard = document.getElementById('chat-approval-' + corrId);
                    if (chatCard) {
                        chatCard.dataset.resolved = 'true';
                        chatCard.className = 'chat-approval-card p-4 bg-zinc-950/80 border border-zinc-800 rounded-xl';
                        chatCard.innerHTML = html;
                    }

                    // Update Inline approval card in chat messages
                    const chatMsgCard = document.getElementById('chat-msg-approval-' + corrId);
                    if (chatMsgCard) {
                        chatMsgCard.dataset.resolved = 'true';
                        chatMsgCard.className = 'chat-approval-card p-4 bg-zinc-950/80 border border-zinc-800 rounded-xl my-3';
                        chatMsgCard.innerHTML = html;
                    }

                    // Update Matrix view approval card
                    const matrixCard = document.getElementById('approval-' + corrId);
                    if (matrixCard) {
                        matrixCard.dataset.resolved = 'true';
                        matrixCard.outerHTML = html;
                    }

                    // Append resolution notice to chat thread
                    appendChatApprovalResolution(corrId, decision);

                    // Re-sync approvals state
                    setTimeout(syncApprovalsToChat, 100);
                } else {
                    console.error('Failed to submit approval decision:', await resp.text());
                }
            } catch (err) {
                console.error('Approval submission error:', err);
            }
        }

        // Append inline resolution pill to the conversational stream
        function appendChatApprovalResolution(corrId, decision) {
            const chatMessages = document.getElementById('chat-messages');
            if (!chatMessages) return;

            const notice = document.createElement('div');
            notice.className = 'w-full flex justify-center my-3 animate-in fade-in duration-300';

            let badgeText = 'APPROVED';
            let badgeClass = 'bg-emerald-950/60 text-emerald-300 border-emerald-800/60';
            let icon = '✓';
            if (decision === 'REJECTED') {
                badgeText = 'DENIED';
                badgeClass = 'bg-rose-950/60 text-rose-300 border-rose-800/60';
                icon = '✕';
            } else if (decision === 'APPROVE_ALL') {
                badgeText = 'AUTO-APPROVED (Auto-Pilot ON)';
                badgeClass = 'bg-zinc-800 text-zinc-200 border-zinc-700';
                icon = '⚡';
            }

            notice.innerHTML = `
                <div class="inline-flex items-center gap-2 px-3.5 py-1.5 rounded-full border text-xs font-mono shadow-sm ${badgeClass}">
                    <span class="font-bold">${icon}</span>
                    <span>Human-in-the-Loop Decision: <strong>${badgeText}</strong></span>
                    <span class="text-[10px] opacity-70">${new Date().toLocaleTimeString()}</span>
                </div>
            `;
            chatMessages.appendChild(notice);
            window.scrollTo({ top: document.body.scrollHeight, behavior: 'smooth' });
        }

        // Sync pending approvals between matrix view and conversational view
        function syncApprovalsToChat() {
            const matrixApprovals = document.getElementById('approvals-list');
            const chatApprovalsList = document.getElementById('chat-approvals-list');
            const chatApprovalsBadge = document.getElementById('chat-approvals-badge');
            const chatApprovalsPing = document.getElementById('chat-approvals-ping');
            const chatApprovalsDot = document.getElementById('chat-approvals-dot');
            const chatApprovalsEmpty = document.getElementById('chat-approvals-empty');
            const chatApprovalPanel = document.getElementById('chat-approval-panel');
            const subviewIndicator = document.getElementById('subview-approval-indicator');

            if (!matrixApprovals || !chatApprovalsList) return;

            const cards = matrixApprovals.querySelectorAll('[id^="approval-"]');
            let pendingCount = 0;

            cards.forEach(card => {
                const corrId = card.id.replace('approval-', '');
                const isResolved = card.dataset.resolved === 'true' || card.textContent.includes('RESOLVED // OPERATOR DECISION') || !card.querySelector('form');

                if (isResolved) {
                    // If marked resolved in matrix, also ensure resolved in chat queue and chat thread
                    const chatCard = document.getElementById('chat-approval-' + corrId);
                    if (chatCard && !chatCard.dataset.resolved) {
                        chatCard.dataset.resolved = 'true';
                        chatCard.className = 'chat-approval-card p-4 bg-zinc-950/80 border border-zinc-800 rounded-xl';
                        chatCard.innerHTML = card.innerHTML;
                    }
                    const chatMsgCard = document.getElementById('chat-msg-approval-' + corrId);
                    if (chatMsgCard && !chatMsgCard.dataset.resolved) {
                        chatMsgCard.dataset.resolved = 'true';
                        chatMsgCard.className = 'chat-approval-card p-4 bg-zinc-950/80 border border-zinc-800 rounded-xl my-3';
                        chatMsgCard.innerHTML = card.innerHTML;
                    }
                    return;
                }

                pendingCount++;

                // Extract metadata from matrix card
                const senderEl = card.querySelector('.font-mono.font-bold');
                const senderText = senderEl ? senderEl.textContent : 'Agent';
                const riskEl = card.querySelector('.uppercase.font-bold');
                const riskText = riskEl ? riskEl.textContent.trim() : 'HIGH RISK';
                const contentEl = card.querySelector('.approval-content');
                const rawContent = contentEl ? getRawContent(contentEl) : '';
                let formatted = renderMarkdown(rawContent);

                // Check if card already mirrored into chat approvals queue
                let chatCard = document.getElementById('chat-approval-' + corrId);
                if (!chatCard) {
                    chatCard = document.createElement('div');
                    chatCard.id = 'chat-approval-' + corrId;
                    chatCard.className = 'chat-approval-card p-4 sm:p-5 bg-zinc-950/90 border border-zinc-800 border-l-4 border-l-amber-500 rounded-xl flex flex-col gap-3.5 shadow-lg animate-in fade-in duration-300';
                    chatCard.innerHTML = `
                        <div class="flex justify-between items-start gap-2">
                            <div class="flex flex-col">
                                <span class="text-[9px] text-amber-400 font-bold font-mono uppercase tracking-widest flex items-center gap-1.5">
                                    <span class="w-2 h-2 rounded-full bg-amber-400 animate-ping"></span> Approval Requested (HITL)
                                </span>
                                <span class="chat-approval-sender font-bold text-zinc-100 text-xs mt-0.5 font-mono">${escapeHtml(senderText)}</span>
                            </div>
                            <span class="text-[9px] font-mono bg-amber-500/10 text-amber-300 border border-amber-500/20 px-2.5 py-0.5 rounded-full uppercase font-bold shrink-0">
                                ${escapeHtml(riskText)}
                            </span>
                        </div>

                        <div class="chat-approval-content text-xs text-zinc-300 font-mono bg-zinc-900/90 p-3.5 rounded-xl border border-zinc-800/90 leading-relaxed shadow-inner overflow-x-auto max-h-[300px]">
                            ${formatted}
                        </div>

                        <div class="flex gap-2.5 justify-end flex-wrap pt-1">
                            <button type="button" onclick="submitApprovalDecision('${corrId}', 'REJECTED')" class="chat-approval-btn-deny px-3.5 py-2 bg-zinc-900 hover:bg-rose-950/60 text-zinc-300 hover:text-rose-200 border border-zinc-700 hover:border-rose-800 rounded-lg text-[10px] font-bold uppercase tracking-wider transition-all active:scale-95 cursor-pointer">
                                Deny Execution
                            </button>
                            <button type="button" onclick="submitApprovalDecision('${corrId}', 'APPROVED')" class="px-3.5 py-2 bg-gradient-to-r from-emerald-600 to-teal-600 hover:from-emerald-500 hover:to-teal-500 text-white rounded-lg text-[10px] font-bold uppercase tracking-wider transition-all shadow-md shadow-emerald-500/20 active:scale-95 cursor-pointer">
                                Approve &amp; Continue
                            </button>
                            <button type="button" onclick="submitApprovalDecision('${corrId}', 'APPROVE_ALL')" class="px-3.5 py-2 bg-white hover:bg-zinc-200 text-zinc-950 rounded-lg text-[10px] font-bold uppercase tracking-wider transition-all shadow-sm active:scale-95 cursor-pointer flex items-center gap-1.5" title="Approve this request and enable Auto-Pilot for all future actions">
                                <span>⚡</span> Auto-Approve All
                            </button>
                        </div>
                    `;

                    chatApprovalsList.appendChild(chatCard);
                }

                // Also post the full interactive approval card directly into the chat stream!
                let chatMsgCard = document.getElementById('chat-msg-approval-' + corrId);
                if (!chatMsgCard) {
                    const chatMessages = document.getElementById('chat-messages');
                    if (chatMessages) {
                        const hero = document.getElementById('chat-hero');
                        if (hero) hero.style.display = 'none';

                        chatMsgCard = document.createElement('div');
                        chatMsgCard.id = 'chat-msg-approval-' + corrId;
                        chatMsgCard.className = 'chat-approval-card p-4 sm:p-5 bg-zinc-950/90 border border-zinc-800 border-l-4 border-l-amber-500 rounded-xl flex flex-col gap-3.5 shadow-lg animate-in fade-in duration-300 my-3 w-full';
                        chatMsgCard.innerHTML = `
                            <div class="flex justify-between items-start gap-2">
                                <div class="flex flex-col">
                                    <span class="text-[9px] text-amber-400 font-bold font-mono uppercase tracking-widest flex items-center gap-1.5">
                                        <span class="w-2 h-2 rounded-full bg-amber-400 animate-ping"></span> Approval Requested (Human-in-the-Loop)
                                    </span>
                                    <span class="chat-approval-sender font-bold text-zinc-100 text-xs mt-0.5 font-mono">${escapeHtml(senderText)}</span>
                                </div>
                                <span class="text-[9px] font-mono bg-amber-500/10 text-amber-300 border border-amber-500/20 px-2.5 py-0.5 rounded-full uppercase font-bold shrink-0">
                                    ${escapeHtml(riskText)}
                                </span>
                            </div>

                            <div class="chat-approval-content text-xs text-zinc-300 font-mono bg-zinc-900/90 p-3.5 rounded-xl border border-zinc-800/90 leading-relaxed shadow-inner overflow-x-auto max-h-[300px]">
                                ${formatted}
                            </div>

                            <div class="flex gap-2.5 justify-end flex-wrap pt-1">
                                <button type="button" onclick="submitApprovalDecision('${corrId}', 'REJECTED')" class="chat-approval-btn-deny px-3.5 py-2 bg-zinc-900 hover:bg-rose-950/60 text-zinc-300 hover:text-rose-200 border border-zinc-700 hover:border-rose-800 rounded-lg text-[10px] font-bold uppercase tracking-wider transition-all active:scale-95 cursor-pointer">
                                    Deny Execution
                                </button>
                                <button type="button" onclick="submitApprovalDecision('${corrId}', 'APPROVED')" class="px-3.5 py-2 bg-gradient-to-r from-emerald-600 to-teal-600 hover:from-emerald-500 hover:to-teal-500 text-white rounded-lg text-[10px] font-bold uppercase tracking-wider transition-all shadow-md shadow-emerald-500/20 active:scale-95 cursor-pointer">
                                    Approve &amp; Continue
                                </button>
                                <button type="button" onclick="submitApprovalDecision('${corrId}', 'APPROVE_ALL')" class="px-3.5 py-2 bg-white hover:bg-zinc-200 text-zinc-950 rounded-lg text-[10px] font-bold uppercase tracking-wider transition-all shadow-sm active:scale-95 cursor-pointer flex items-center gap-1.5" title="Approve this request and enable Auto-Pilot for all future actions">
                                    <span>⚡</span> Auto-Approve All
                                </button>
                            </div>
                        `;
                        chatMessages.appendChild(chatMsgCard);
                        window.scrollTo({ top: document.body.scrollHeight, behavior: 'smooth' });
                    }
                }
            });

            // Update badge counters and visibility states
            if (chatApprovalsBadge) {
                chatApprovalsBadge.textContent = `${pendingCount} Pending`;
                if (pendingCount > 0) {
                    chatApprovalsBadge.className = 'px-2.5 py-0.5 rounded-full text-[10px] font-mono font-bold bg-amber-500/20 text-amber-300 border border-amber-500/40 transition-colors';
                } else {
                    chatApprovalsBadge.className = 'px-2.5 py-0.5 rounded-full text-[10px] font-mono font-bold bg-zinc-800 text-zinc-400 border border-zinc-700 transition-colors';
                }
            }

            if (chatApprovalsPing) {
                if (pendingCount > 0) {
                    chatApprovalsPing.classList.remove('hidden');
                } else {
                    chatApprovalsPing.classList.add('hidden');
                }
            }

            if (chatApprovalsDot) {
                if (pendingCount > 0) {
                    chatApprovalsDot.classList.remove('bg-zinc-500');
                    chatApprovalsDot.classList.add('bg-amber-400');
                } else {
                    chatApprovalsDot.classList.remove('bg-amber-400');
                    chatApprovalsDot.classList.add('bg-zinc-500');
                }
            }

            if (chatApprovalPanel) {
                if (pendingCount > 0) {
                    chatApprovalPanel.classList.add('has-pending');
                } else {
                    chatApprovalPanel.classList.remove('has-pending');
                }
            }

            if (chatApprovalsEmpty) {
                if (pendingCount > 0) {
                    chatApprovalsEmpty.classList.add('hidden');
                } else {
                    chatApprovalsEmpty.classList.remove('hidden');
                }
            }

            if (subviewIndicator) {
                subviewIndicator.textContent = pendingCount;
                if (pendingCount > 0) {
                    subviewIndicator.classList.remove('hidden');
                } else {
                    subviewIndicator.classList.add('hidden');
                }
            }

            // Ensure approvals list is visible if there are pending actions
            if (pendingCount > 0 && chatApprovalsList.classList.contains('hidden')) {
                toggleChatApprovalsList();
            }
        }

        // Initialize Chat Composer Event Listeners
        document.addEventListener('DOMContentLoaded', function() {
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

            // Sync initial state of approvals
            syncApprovalsToChat();

            // Synchronize HITL approval requests & artifacts into active chat thread
            document.addEventListener('htmx:afterSwap', function(e) {
                if (e.target && (e.target.id === 'approvals-list' || (e.target.id && e.target.id.startsWith('approval-')))) {
                    syncApprovalsToChat();
                }
                if (e.target && e.target.id === 'artifacts-list') {
                    const lastArt = e.target.firstElementChild;
                    if (lastArt && !lastArt.dataset.chatMirrored) {
                        lastArt.dataset.chatMirrored = 'true';
                        const chatMessages = document.getElementById('chat-messages');
                        if (chatMessages) {
                            const clone = lastArt.cloneNode(true);
                            clone.classList.add('max-w-md', 'my-2');
                            chatMessages.appendChild(clone);
                            window.scrollTo({ top: document.body.scrollHeight, behavior: 'smooth' });
                        }
                    }
                }
            });
        });

        // Explicit Global Window Exports for Inline HTML Handlers
        window.toggleTheme = toggleTheme;
        window.updateThemeUI = updateThemeUI;
        window.switchTab = switchTab;
        window.switchDashboardSubview = switchDashboardSubview;
        window.submitChatComposer = submitChatComposer;
        window.clearChatThread = clearChatThread;
        window.toggleChatApprovalsList = toggleChatApprovalsList;
        window.submitApprovalDecision = submitApprovalDecision;
        window.fetchAuditExplorerData = fetchAuditExplorerData;
        window.exportAuditCSV = exportAuditCSV;
        window.pruneAuditRecords = pruneAuditRecords;
        window.setPromptTask = setPromptTask;
        window.renderMarkdown = renderMarkdown;
        window.normalizeMarkdown = normalizeMarkdown;