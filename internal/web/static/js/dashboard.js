function getCsrfToken() {
            const match = document.cookie.match(/(?:^|;\s*)_csrf_token=([^;]+)/);
            return match ? decodeURIComponent(match[1]) : '';
        }

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
                const raw = mdContainer ? (mdContainer.getAttribute('data-raw') || '') : '';
                if (typeof updateParallelismSwimlane === 'function') {
                    updateParallelismSwimlane(sender, recipient, raw, eventType, tool);
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
            const raw = mdContainer.getAttribute('data-raw') || '';
            let renderedHTML = '';
            try {
                if (typeof marked !== 'undefined' && typeof marked.parse === 'function') {
                    renderedHTML = marked.parse(raw);
                } else {
                    renderedHTML = escapeHtml(raw).replace(/\n/g, '<br>');
                }
            } catch (err) {
                console.warn('Markdown fallback:', err);
                renderedHTML = escapeHtml(raw).replace(/\n/g, '<br>');
            }
            if (!renderedHTML || renderedHTML.trim() === '') {
                renderedHTML = escapeHtml(raw).replace(/\n/g, '<br>');
            }
            if (tool || eventType === 'TOOL_CALL') {
                const toolName = tool || 'tool_call';
                mdContainer.innerHTML = `
                    <details class="my-1 rounded-lg border border-purple-900/60 bg-purple-950/20 overflow-hidden shadow-inner group/details">
                        <summary class="px-3 py-1.5 text-[10px] font-mono font-bold text-purple-300 cursor-pointer bg-purple-950/40 hover:bg-purple-950/60 flex items-center justify-between select-none">
                            <span class="flex items-center gap-1.5">
                                <span class="text-purple-400">⚡ Tool Call:</span>
                                <span class="px-1.5 py-0.5 rounded bg-purple-900/70 border border-purple-700/60 text-purple-200">${toolName}</span>
                            </span>
                            <span class="text-[9px] text-purple-400 font-normal group-open/details:hidden">Expand Payload ➔</span>
                            <span class="text-[9px] text-purple-400 font-normal hidden group-open/details:inline">Collapse ⯅</span>
                        </summary>
                        <div class="p-3 text-[10px] font-mono text-zinc-300 overflow-x-auto bg-zinc-950/80 border-t border-purple-900/40 leading-relaxed">${renderedHTML}</div>
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
                        barColor = 'bg-purple-500/30 border-purple-500/60 text-purple-300';
                        statusColor = 'text-purple-300 font-mono font-bold animate-pulse';
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
                            ? "px-2.5 py-1 text-[9px] font-mono font-bold uppercase tracking-wider bg-indigo-600 text-white rounded-md cursor-pointer transition-all active:scale-95 shadow-sm"
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
                        btnEl.className = "border-b-2 border-indigo-500 py-3.5 px-2 text-xs font-bold uppercase tracking-wider text-indigo-400 flex items-center gap-2 transition-all cursor-pointer";
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
                                        '<span class="text-indigo-400 font-bold">$' + cost.toFixed(4) + '</span>' +
                                    '</div>' +
                                    '<div class="w-full bg-zinc-950 rounded-full h-2 overflow-hidden border border-zinc-800">' +
                                        '<div class="bg-gradient-to-r from-indigo-500 to-purple-400 h-2 rounded-full" style="width: ' + pct + '%"></div>' +
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
                            '<td class="py-2.5 px-3 text-indigo-300 text-[11px] font-bold">' + rec.agent_id + '</td>' +
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
                el.focus();
            }
        }