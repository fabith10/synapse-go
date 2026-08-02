function insertPromptTemplate(type) {
            const promptEl = document.getElementById('studio_system_prompt');
            if (!promptEl) return;
            if (type === 'analyst') {
                promptEl.value = 'You are a Quantitative Analyst specialist. Your job is to parse financial data, perform modeling, and extract key performance indicators.\n\nCRITICAL RULES:\n1. Execute calculations via script tools (\'execute_python_docker\' or native tools).\n2. Deliver clear markdown tables summarizing key metrics.';
            } else if (type === 'researcher') {
                promptEl.value = 'You are a Deep Web Researcher. Your job is to query live web search endpoints, clean markdown content, and synthesize structured reports.\n\nCRITICAL RULES:\n1. Use \'web_search_and_extract\' tool for context.\n2. Summarize findings into executive PDF reports via \'generate_pdf_report\'.';
            }
        }

        let studioToolsList = [];

        function fetchStudioTools() {
            fetch('/api/agents')
                .then(res => res.json())
                .then(data => {
                    const checklist = document.getElementById('studio-tools-checklist');
                    const dockerCheckbox = document.getElementById('docker-policy-checkbox');
                    const dockerLabel = document.getElementById('docker-policy-status-label');

                    if (dockerCheckbox && typeof data.docker_tools_enabled === 'boolean') {
                        dockerCheckbox.checked = data.docker_tools_enabled;
                        if (dockerLabel) {
                            dockerLabel.textContent = data.docker_tools_enabled ? "Unlocked by Admin Policy" : "Gated by Admin Policy";
                            dockerLabel.className = data.docker_tools_enabled ? "text-[9px] font-mono text-emerald-400 font-bold" : "text-[9px] font-mono text-amber-400 font-bold";
                        }
                    }

                    if (checklist && data.available_tools) {
                        studioToolsList = data.available_tools;
                        checklist.innerHTML = '';
                        data.available_tools.forEach(t => {
                            const isGatedAndDisabled = t.is_gated && !data.docker_tools_enabled;
                            const wrapper = document.createElement('label');
                            wrapper.className = "flex items-start gap-2.5 p-2.5 bg-zinc-900/80 rounded-lg border border-zinc-800 hover:border-zinc-700 cursor-pointer transition-all";
                            wrapper.innerHTML = 
                                '<input type="checkbox" name="studio_tools" value="' + t.name + '" ' + (isGatedAndDisabled ? 'disabled' : '') + ' class="mt-0.5 rounded text-indigo-600 border-zinc-700 bg-zinc-950 focus:ring-indigo-500" />' +
                                '<div class="flex flex-col gap-0.5 min-w-0">' +
                                    '<div class="flex items-center gap-1.5 flex-wrap">' +
                                        '<span class="text-xs font-mono font-bold text-zinc-100">' + t.name + '</span>' +
                                        '<span class="text-[8px] font-mono font-bold uppercase px-1.5 py-0.5 rounded ' + (t.is_gated ? 'bg-amber-500/10 text-amber-400 border border-amber-500/20' : 'bg-indigo-500/10 text-indigo-400 border border-indigo-500/20') + '">' + t.category + '</span>' +
                                    '</div>' +
                                    '<span class="text-[10px] text-zinc-400 leading-tight">' + t.description + (isGatedAndDisabled ? ' (Requires Tier 3 Admin Unlock)' : '') + '</span>' +
                                '</div>';
                            checklist.appendChild(wrapper);
                        });
                    }

                    const listEl = document.getElementById('studio-registered-agents-list');
                    if (listEl && data.agents) {
                        listEl.innerHTML = '';
                        Object.keys(data.agents).forEach(id => {
                            const agentCfg = data.agents[id];
                            const card = document.createElement('div');
                            card.className = "p-3.5 bg-zinc-900/80 rounded-xl border border-zinc-800 flex items-center justify-between gap-2 shadow-sm";
                            card.innerHTML = 
                                '<div class="flex flex-col min-w-0">' +
                                    '<span class="font-bold text-xs text-zinc-100 font-mono truncate">' + id + '</span>' +
                                    '<span class="text-[10px] text-zinc-400 font-mono truncate">' + (agentCfg.description || 'Specialist Agent') + '</span>' +
                                '</div>' +
                                '<button onclick="exportAgentBlueprint(\'' + id + '\')" class="px-2.5 py-1.5 bg-zinc-800 hover:bg-zinc-700 border border-zinc-700 text-zinc-200 rounded-lg text-[9px] font-bold font-mono uppercase tracking-wider transition-all active:scale-95 shrink-0 cursor-pointer">' +
                                    '<span>📥 Export</span>' +
                                '</button>';
                            listEl.appendChild(card);
                        });
                    }
                })
                .catch(err => console.error("Error fetching studio tools:", err));
        }

        function toggleDockerAdminPolicy(enabled) {
            fetch('/api/settings/docker-toggle', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ enabled: enabled })
            })
            .then(res => res.json())
            .then(data => {
                fetchStudioTools();
            })
            .catch(err => alert("Error toggling policy: " + err));
        }

        function createStudioAgent(e) {
            e.preventDefault();
            const id = document.getElementById('studio_agent_id').value.trim();
            const desc = document.getElementById('studio_description').value.trim();
            const prompt = document.getElementById('studio_system_prompt').value.trim();
            const alertEl = document.getElementById('studio-alert');

            const selectedTools = [];
            document.querySelectorAll('input[name="studio_tools"]:checked').forEach(cb => {
                selectedTools.push(cb.value);
            });

            const payload = {
                id: id,
                description: desc,
                system_prompt: prompt,
                tools: selectedTools
            };

            fetch('/api/agents', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify(payload)
            })
            .then(res => res.json().then(data => ({ status: res.status, body: data })))
            .then(res => {
                alertEl.classList.remove('hidden', 'bg-rose-950/60', 'text-rose-200', 'border-rose-800', 'bg-emerald-950/60', 'text-emerald-200', 'border-emerald-800');
                if (res.body.success) {
                    alertEl.className = "p-4 rounded-xl text-xs font-mono bg-emerald-950/60 text-emerald-200 border border-emerald-800 shadow-lg";
                    alertEl.textContent = "✅ " + res.body.message;
                    document.getElementById('studio-agent-form').reset();
                    fetchStudioTools();
                } else {
                    alertEl.className = "p-4 rounded-xl text-xs font-mono bg-rose-950/60 text-rose-200 border border-rose-800 shadow-lg";
                    alertEl.textContent = "❌ " + res.body.error;
                }
            })
            .catch(err => {
                alertEl.classList.remove('hidden');
                alertEl.className = "p-4 rounded-xl text-xs font-mono bg-rose-950/60 text-rose-200 border border-rose-800 shadow-lg";
                alertEl.textContent = "❌ Network error: " + err;
            });
        }

        function exportAgentBlueprint(id) {
            window.open('/api/agents/export?id=' + encodeURIComponent(id), '_blank');
        }

        function importAgentBlueprint(e) {
            const file = e.target.files[0];
            if (!file) return;
            const reader = new FileReader();
            reader.onload = function(evt) {
                fetch('/api/agents/import', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: evt.target.result
                })
                .then(res => res.json())
                .then(data => {
                    if (data.success) {
                        alert("✅ " + data.message);
                        fetchStudioTools();
                    } else {
                        alert("❌ Import failed: " + data.error);
                    }
                })
                .catch(err => alert("❌ Import failed: " + err));
            };
            reader.readAsText(file);
        }

        window.addEventListener('DOMContentLoaded', function() {
            setTimeout(initNetworkVisualizer, 50);
            if (window.location.hash) {
                const hash = window.location.hash.replace('#', '');
                if (['dashboard', 'studio', 'audit', 'network', 'config', 'scheduler'].includes(hash)) {
                    switchTab(hash);
                }
            }
        });

        // -------------------------------------------------------------------
        // Agent Network Topology Visualizer Engine
        // -------------------------------------------------------------------
        let networkNodes = [];
        let networkEdges = [];
        let networkPulses = [];
        let selectedNode = null;
        let isSimulating = false;
        let dragNode = null;
        let dragOffset = { x: 0, y: 0 };