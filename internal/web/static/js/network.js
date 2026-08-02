function initNetworkVisualizer() {
            const canvas = document.getElementById('network-canvas');
            if (!canvas) return;

            const container = canvas.parentElement;
            const w = Math.max(300, canvas.clientWidth || (container && container.clientWidth > 0 ? container.clientWidth : 800));
            const h = Math.max(300, canvas.clientHeight || (container && container.clientHeight > 0 ? container.clientHeight : 520));

            const hasNaN = networkNodes.some(n => isNaN(n.x) || isNaN(n.y));

            if (!networkNodes || networkNodes.length === 0 || hasNaN) {
                setupGraphTopology([], [], w, h);
            }

            if (!isSimulating) {
                isSimulating = true;
                requestAnimationFrame(renderNetworkLoop);
            }

            fetch('/api/network/topology')
                .then(res => res.json())
                .then(data => {
                    if (data && data.nodes && data.nodes.length > 0) {
                        const curCanvas = document.getElementById('network-canvas');
                        const curContainer = curCanvas ? curCanvas.parentElement : null;
                        const curW = Math.max(300, curCanvas && curCanvas.clientWidth > 0 ? curCanvas.clientWidth : (curContainer ? curContainer.clientWidth : w));
                        const curH = Math.max(300, curCanvas && curCanvas.clientHeight > 0 ? curCanvas.clientHeight : (curContainer ? curContainer.clientHeight : h));
                        setupGraphTopology(data.nodes, data.edges || [], curW, curH);
                    }
                })
                .catch(err => {
                    console.warn('Network topology fetch warning:', err);
                });

            canvas.onmousedown = function(e) {
                const pos = getCanvasMousePos(canvas, e);
                const hit = findNodeAtPos(pos.x, pos.y);
                if (hit) {
                    dragNode = hit;
                    dragNode.fx = hit.x;
                    dragNode.fy = hit.y;
                    dragOffset = { x: hit.x - pos.x, y: hit.y - pos.y };
                    selectNetworkNode(hit);
                }
            };

            canvas.onmousemove = function(e) {
                if (dragNode) {
                    const pos = getCanvasMousePos(canvas, e);
                    dragNode.x = pos.x + dragOffset.x;
                    dragNode.y = pos.y + dragOffset.y;
                    dragNode.fx = dragNode.x;
                    dragNode.fy = dragNode.y;
                }
            };

            canvas.onmouseup = function() {
                if (dragNode) {
                    dragNode.fx = null;
                    dragNode.fy = null;
                    dragNode = null;
                }
            };
        }

        function getCanvasMousePos(canvas, evt) {
            const rect = canvas.getBoundingClientRect();
            return {
                x: evt.clientX - rect.left,
                y: evt.clientY - rect.top
            };
        }

        function findNodeAtPos(x, y) {
            for (let i = networkNodes.length - 1; i >= 0; i--) {
                const n = networkNodes[i];
                if (!n || isNaN(n.x) || isNaN(n.y)) continue;
                const dx = n.x - x;
                const dy = n.y - y;
                const dist = Math.sqrt(dx * dx + dy * dy);
                if (dist <= n.radius + 6) return n;
            }
            return null;
        }

        function setupGraphTopology(rawNodes, rawEdges, width, height) {
            const cx = (width && width > 100) ? width / 2 : 400;
            const cy = (height && height > 100) ? height / 2 : 260;

            if (!rawNodes || rawNodes.length === 0) {
                rawNodes = [
                    { id: "USER", label: "User / Client", category: "user", role: "Task Dispatcher" },
                    { id: "triage-agent", label: "Triage Agent", category: "orchestrator", role: "Intent Router & Gatekeeper" },
                    { id: "planner-agent", label: "Planner Agent", category: "orchestrator", role: "Task Decomposer & DAG Planner" },
                    { id: "supervisor-agent", label: "Supervisor Agent", category: "orchestrator", role: "Goal Verifier" },
                    { id: "developer-agent", label: "Developer Agent", category: "specialist", role: "Code Generation & Execution", tools: ["read_file", "write_file", "execute_python_docker"] },
                    { id: "researcher-agent", label: "Researcher Agent", category: "specialist", role: "Deep Web Search & PDF Synthesis", tools: ["web_search_and_extract", "generate_pdf_report"] },
                    { id: "quant-agent", label: "Quant Agent", category: "specialist", role: "Options Pricing & Forward Curves", tools: ["query_compute_prices", "query_options_chain"] },
                    { id: "writer-agent", label: "Writer Agent", category: "specialist", role: "Document Synthesis", tools: ["write_file"] }
                ];
                rawEdges = [
                    { source: "USER", target: "triage-agent", weight: 10 },
                    { source: "triage-agent", target: "planner-agent", weight: 5 },
                    { source: "triage-agent", target: "developer-agent", weight: 8 },
                    { source: "triage-agent", target: "researcher-agent", weight: 8 },
                    { source: "triage-agent", target: "quant-agent", weight: 6 },
                    { source: "triage-agent", target: "writer-agent", weight: 6 },
                    { source: "supervisor-agent", target: "triage-agent", weight: 3 }
                ];
            }

            // Exclude tool nodes - network topology focuses on agent & user interactions
            const agentNodesOnly = rawNodes.filter(n => n.category !== 'tool');

            const userNodes = agentNodesOnly.filter(n => n.category === 'user');
            const orchNodes = agentNodesOnly.filter(n => n.category === 'orchestrator');
            const specNodes = agentNodesOnly.filter(n => n.category === 'specialist');
            const otherNodes = agentNodesOnly.filter(n => !['user', 'orchestrator', 'specialist'].includes(n.category));

            const placeRing = (nodes, radius, baseAngleOffset = 0) => {
                const count = nodes.length;
                return nodes.map((rn, idx) => {
                    const angle = baseAngleOffset + (count > 0 ? (idx / count) * Math.PI * 2 : 0);
                    let nodeRadius = 18;
                    let color = '#818cf8';
                    if (rn.category === 'user') { nodeRadius = 22; color = '#f59e0b'; }
                    else if (rn.category === 'orchestrator') { nodeRadius = 20; color = '#6366f1'; }
                    else if (rn.category === 'specialist') { nodeRadius = 18; color = '#10b981'; }

                    const initX = cx + (radius === 0 ? 0 : Math.cos(angle) * radius) + (Math.random() - 0.5) * 10;
                    const initY = cy + (radius === 0 ? 0 : Math.sin(angle) * radius) + (Math.random() - 0.5) * 10;

                    return {
                        id: rn.id,
                        label: rn.label || rn.id,
                        category: rn.category || 'specialist',
                        role: rn.role || '',
                        tools: rn.tools || [],
                        radius: nodeRadius,
                        color: color,
                        x: isNaN(initX) ? cx : initX,
                        y: isNaN(initY) ? cy : initY,
                        vx: 0,
                        vy: 0,
                        fx: null,
                        fy: null
                    };
                });
            };

            const placedUser = placeRing(userNodes, 0);
            const placedOrch = placeRing(orchNodes, Math.min(cx, cy) * 0.40);
            const placedSpec = placeRing(specNodes, Math.min(cx, cy) * 0.75, Math.PI / 8);
            const placedOther = placeRing(otherNodes, Math.min(cx, cy) * 0.88);

            networkNodes = [...placedUser, ...placedOrch, ...placedSpec, ...placedOther];

            const nodeMap = {};
            networkNodes.forEach(n => { nodeMap[n.id] = n; });

            networkEdges = [];
            rawEdges.forEach(re => {
                const src = nodeMap[re.source];
                const tgt = nodeMap[re.target];
                if (src && tgt && src.category !== 'tool' && tgt.category !== 'tool') {
                    networkEdges.push({ source: src, target: tgt, weight: re.weight || 1 });
                }
            });
        }

        if (typeof window.radarSweepAngle === 'undefined') {
            window.radarSweepAngle = 0;
        }

        function renderNetworkLoop() {
            const canvas = document.getElementById('network-canvas');
            if (!canvas) {
                isSimulating = false;
                return;
            }
            try {
                const ctx = canvas.getContext('2d');
                const dpr = window.devicePixelRatio || 1;
                const container = canvas.parentElement;
                const width = Math.max(300, canvas.clientWidth || (container && container.clientWidth > 0 ? container.clientWidth : 800));
                const height = Math.max(300, canvas.clientHeight || (container && container.clientHeight > 0 ? container.clientHeight : 520));

                const targetW = Math.floor(width * dpr);
                const targetH = Math.floor(height * dpr);

                if (canvas.width !== targetW || canvas.height !== targetH) {
                    canvas.width = targetW;
                    canvas.height = targetH;
                }
                ctx.setTransform(dpr, 0, 0, dpr, 0, 0);

                if (container && container.clientWidth > 0) {
                    updatePhysics(width, height);
                }

                const isLight = document.body.classList.contains('light-theme');
                ctx.fillStyle = isLight ? '#f8fafc' : '#05070a';
                ctx.fillRect(0, 0, width, height);

                const cx = width / 2;
                const cy = height / 2;
                const maxRadius = Math.min(width, height) * 0.42;

                // --- MINIMALIST NETWORK CANVAS BACKGROUND ---
                [0.4, 0.75].forEach((ratio) => {
                    const r = maxRadius * ratio;
                    ctx.beginPath();
                    ctx.arc(cx, cy, r, 0, Math.PI * 2);
                    ctx.strokeStyle = isLight ? 'rgba(99, 102, 241, 0.08)' : 'rgba(255, 255, 255, 0.05)';
                    ctx.lineWidth = 1;
                    ctx.setLineDash([4, 6]);
                    ctx.stroke();
                    ctx.setLineDash([]);
                });

                // Telemetry Header Info Overlay
                const hudM = 16;
                ctx.font = '500 10px "JetBrains Mono", monospace';
                ctx.fillStyle = isLight ? '#4f46e5' : '#a1a1aa';
                ctx.textAlign = 'left';
                ctx.fillText('NETWORK TOPOLOGY', hudM, hudM + 12);
                ctx.font = '400 9px "JetBrains Mono", monospace';
                ctx.fillStyle = isLight ? '#64748b' : '#52525b';
                ctx.fillText(networkNodes.length + ' ACTIVE NODES', hudM, hudM + 26);

                // --- RENDER INTERACTION EDGES ---
                networkEdges.forEach(e => {
                    if (!e.source || !e.target) return;
                    if (isNaN(e.source.x) || isNaN(e.source.y) || isNaN(e.target.x) || isNaN(e.target.y)) return;
                    ctx.beginPath();
                    ctx.moveTo(e.source.x, e.source.y);
                    ctx.lineTo(e.target.x, e.target.y);
                    ctx.strokeStyle = isLight ? 'rgba(99, 102, 241, 0.2)' : 'rgba(63, 63, 70, 0.5)';
                    ctx.lineWidth = 1;
                    ctx.setLineDash([3, 3]);
                    ctx.stroke();
                    ctx.setLineDash([]);
                });

                // --- RENDER SIGNAL PULSES ---
                for (let i = networkPulses.length - 1; i >= 0; i--) {
                    const p = networkPulses[i];
                    p.progress += p.speed;
                    if (p.progress >= 1) {
                        networkPulses.splice(i, 1);
                        continue;
                    }
                    if (isNaN(p.source.x) || isNaN(p.source.y) || isNaN(p.target.x) || isNaN(p.target.y)) continue;
                    const px = p.source.x + (p.target.x - p.source.x) * p.progress;
                    const py = p.source.y + (p.target.y - p.source.y) * p.progress;

                    ctx.beginPath();
                    ctx.arc(px, py, 3.5, 0, Math.PI * 2);
                    ctx.fillStyle = isLight ? '#4f46e5' : '#818cf8';
                    ctx.shadowColor = isLight ? '#6366f1' : '#818cf8';
                    ctx.shadowBlur = 8;
                    ctx.fill();
                    ctx.shadowBlur = 0;
                }

                // --- RENDER RADAR NODES & TARGET LOCKS ---
                const glowDuration = 3500;
                networkNodes.forEach(n => {
                    if (!n || isNaN(n.x) || isNaN(n.y)) return;
                    const isSelected = selectedNode && selectedNode.id === n.id;
                    const isGlowing = n.glowUntil && n.glowUntil > Date.now();

                    // Tactical Target Lock Reticle around Selected / Firing Node
                    if (isSelected || isGlowing) {
                        const boxSize = (n.radius + 14);
                        ctx.strokeStyle = isGlowing ? '#34d399' : '#818cf8';
                        ctx.lineWidth = isGlowing ? 2 : 1.5;

                        // Target Bracket Corners
                        ctx.beginPath();
                        ctx.moveTo(n.x - boxSize, n.y - boxSize + 6); ctx.lineTo(n.x - boxSize, n.y - boxSize); ctx.lineTo(n.x - boxSize + 6, n.y - boxSize);
                        ctx.moveTo(n.x + boxSize - 6, n.y - boxSize); ctx.lineTo(n.x + boxSize, n.y - boxSize); ctx.lineTo(n.x + boxSize, n.y - boxSize + 6);
                        ctx.moveTo(n.x - boxSize, n.y + boxSize - 6); ctx.lineTo(n.x - boxSize, n.y + boxSize); ctx.lineTo(n.x - boxSize + 6, n.y + boxSize);
                        ctx.moveTo(n.x + boxSize - 6, n.y + boxSize); ctx.lineTo(n.x + boxSize, n.y + boxSize); ctx.lineTo(n.x + boxSize, n.y + boxSize - 6);
                        ctx.stroke();

                        if (isGlowing) {
                            const remaining = Math.max(0, n.glowUntil - Date.now());
                            const elapsed = (glowDuration - remaining) / 1000;
                            const pulseCycle = (elapsed % 0.8) / 0.8;
                            const rippleRadius = n.radius + 6 + pulseCycle * 28;
                            const alpha = Math.max(0, 1 - pulseCycle);

                            ctx.beginPath();
                            ctx.arc(n.x, n.y, rippleRadius, 0, Math.PI * 2);
                            ctx.strokeStyle = n.color || '#34d399';
                            ctx.lineWidth = 2.5;
                            ctx.globalAlpha = alpha;
                            ctx.stroke();
                            ctx.globalAlpha = 1.0;
                        }
                    }

                    // Node Radar Blip Core (Pulsing beat when glowing)
                    const pulseFactor = isGlowing ? Math.sin(Date.now() / 90) * 4 : 0;
                    const activeRadius = Math.max(2, n.radius + (isSelected ? 3 : 0) + pulseFactor);

                    ctx.beginPath();
                    ctx.arc(n.x, n.y, activeRadius, 0, Math.PI * 2);
                    ctx.fillStyle = isGlowing ? '#34d399' : (n.color || '#818cf8');
                    if (isSelected || isGlowing) {
                        ctx.shadowColor = isGlowing ? '#34d399' : (n.color || '#818cf8');
                        ctx.shadowBlur = isGlowing ? 32 : 16;
                    }
                    ctx.fill();
                    ctx.shadowBlur = 0;

                    ctx.strokeStyle = isGlowing ? '#ffffff' : (isLight ? '#ffffff' : '#05070a');
                    ctx.lineWidth = isGlowing ? 2.5 : 2;
                    ctx.stroke();

                    // Node Label with Radar Tag
                    ctx.font = isGlowing ? '700 11px "JetBrains Mono", monospace' : '10px "JetBrains Mono", monospace, sans-serif';
                    ctx.fillStyle = isGlowing ? '#34d399' : (isLight ? '#0f172a' : '#f4f4f5');
                    ctx.textAlign = 'center';
                    ctx.fillText(n.label || n.id, n.x, n.y + n.radius + 16);
                });
            } catch(renderErr) {
                console.warn('Canvas render frame error:', renderErr);
            }

            requestAnimationFrame(renderNetworkLoop);
        }

        function updatePhysics(width, height) {
            if (!networkNodes || networkNodes.length === 0) return;

            const cx = width / 2;
            const cy = height / 2;
            const nodeCount = networkNodes.length;
            const k = 0.04;
            const rep = Math.min(1000, Math.max(150, 12000 / Math.max(1, nodeCount)));
            const maxSpeed = 10;

            for (let i = 0; i < nodeCount; i++) {
                const na = networkNodes[i];
                if (!na || isNaN(na.x) || isNaN(na.y)) {
                    if (na) { na.x = cx; na.y = cy; na.vx = 0; na.vy = 0; }
                    continue;
                }
                for (let j = i + 1; j < nodeCount; j++) {
                    const nb = networkNodes[j];
                    if (!nb || isNaN(nb.x) || isNaN(nb.y)) {
                        if (nb) { nb.x = cx; nb.y = cy; nb.vx = 0; nb.vy = 0; }
                        continue;
                    }
                    let dx = nb.x - na.x;
                    let dy = nb.y - na.y;
                    if (Math.abs(dx) < 0.1 && Math.abs(dy) < 0.1) {
                        dx = (Math.random() - 0.5) * 4;
                        dy = (Math.random() - 0.5) * 4;
                    }
                    const distSq = dx * dx + dy * dy + 1.0;
                    const dist = Math.sqrt(distSq);
                    const force = rep / distSq;

                    const fx = (dx / dist) * force;
                    const fy = (dy / dist) * force;

                    if (na.fx === null) { na.vx -= fx; na.vy -= fy; }
                    if (nb.fx === null) { nb.vx += fx; nb.vy += fy; }
                }
            }

            networkEdges.forEach(e => {
                if (!e.source || !e.target) return;
                if (isNaN(e.source.x) || isNaN(e.source.y) || isNaN(e.target.x) || isNaN(e.target.y)) return;

                let dx = e.target.x - e.source.x;
                let dy = e.target.y - e.source.y;
                let dist = Math.sqrt(dx * dx + dy * dy) + 0.1;
                const desired = e.target.category === 'tool' ? 70 : 130;
                const delta = dist - desired;
                const force = delta * k;

                const fx = (dx / dist) * force;
                const fy = (dy / dist) * force;

                if (e.source.fx === null) { e.source.vx += fx; e.source.vy += fy; }
                if (e.target.fx === null) { e.target.vx -= fx; e.target.vy -= fy; }
            });

            networkNodes.forEach(n => {
                if (n.fx !== null) {
                    n.x = n.fx;
                    n.y = n.fy;
                    return;
                }
                n.vx += (cx - n.x) * 0.002;
                n.vy += (cy - n.y) * 0.002;

                n.vx *= 0.82;
                n.vy *= 0.82;

                const speed = Math.sqrt(n.vx * n.vx + n.vy * n.vy);
                if (speed > maxSpeed) {
                    n.vx = (n.vx / speed) * maxSpeed;
                    n.vy = (n.vy / speed) * maxSpeed;
                }

                n.x += n.vx;
                n.y += n.vy;

                const minX = n.radius + 12;
                const maxX = width - n.radius - 12;
                const minY = n.radius + 12;
                const maxY = height - n.radius - 12;

                if (n.x < minX) { n.x = minX; n.vx = 0; }
                if (n.x > maxX) { n.x = maxX; n.vx = 0; }
                if (n.y < minY) { n.y = minY; n.vy = 0; }
                if (n.y > maxY) { n.y = maxY; n.vy = 0; }
            });
        }

        function selectNetworkNode(node) {
            selectedNode = node;
            const badge = document.getElementById('inspector-badge');
            const content = document.getElementById('inspector-content');
            if (!badge || !content) return;

            badge.textContent = node.category.toUpperCase();
            badge.className = 'text-[9px] font-mono px-2 py-0.5 rounded font-bold uppercase ' +
                (node.category === 'user' ? 'bg-amber-500/20 text-amber-300 border border-amber-500/30' :
                (node.category === 'orchestrator' ? 'bg-indigo-500/20 text-indigo-300 border border-indigo-500/30' :
                (node.category === 'specialist' ? 'bg-emerald-500/20 text-emerald-300 border border-emerald-500/30' : 'bg-sky-500/20 text-sky-300 border border-sky-500/30')));

            let toolsHtml = '';
            if (node.tools && node.tools.length > 0) {
                toolsHtml = '<div class="space-y-1"><span class="text-[10px] uppercase font-bold text-zinc-400">Assigned Tools:</span><div class="flex flex-wrap gap-1">' +
                    node.tools.map(t => '<span class="px-2 py-0.5 rounded bg-zinc-900 border border-zinc-800 text-[10px] text-indigo-300">' + t + '</span>').join('') +
                    '</div></div>';
            }

            content.innerHTML = 
                '<div class="space-y-2">' +
                    '<div class="text-sm font-extrabold text-zinc-100">' + node.label + '</div>' +
                    '<div class="text-[10px] text-zinc-400 font-mono">ID: <span class="text-zinc-200">' + node.id + '</span></div>' +
                '</div>' +
                '<div class="p-3 bg-zinc-950 rounded-xl border border-zinc-800 leading-relaxed text-zinc-300 text-xs">' +
                    (node.role || 'Active workspace node') +
                '</div>' +
                toolsHtml +
                '<div class="pt-2 border-t border-zinc-800 flex items-center justify-between text-[10px] text-zinc-400">' +
                    '<span>Status: <strong class="text-emerald-400">ONLINE</strong></span>' +
                    '<button onclick="triggerPulseFromNode(\'' + node.id + '\')" class="text-indigo-400 hover:underline cursor-pointer">Emit Signal</button>' +
                '</div>';
        }

        function triggerPulseFromNode(sourceId) {
            const srcNode = networkNodes.find(n => n.id === sourceId);
            if (!srcNode) return;
            const connectedEdges = networkEdges.filter(e => e.source.id === sourceId || e.target.id === sourceId);
            connectedEdges.forEach(e => {
                const targetNode = e.source.id === sourceId ? e.target : e.source;
                networkPulses.push({
                    source: srcNode,
                    target: targetNode,
                    progress: 0,
                    speed: 0.02 + Math.random() * 0.01,
                    color: srcNode.color
                });
            });
        }

        function triggerSamplePulse() {
            if (networkNodes.length === 0) return;
            const userNode = networkNodes.find(n => n.id === 'USER') || networkNodes[0];
            const triageNode = networkNodes.find(n => n.id === 'triage-agent') || networkNodes[1];
            if (userNode && triageNode) {
                networkPulses.push({ source: userNode, target: triageNode, progress: 0, speed: 0.02, color: '#f59e0b' });
                setTimeout(() => {
                    const devNode = networkNodes.find(n => n.id === 'developer-agent' || n.category === 'specialist');
                    if (devNode) {
                        networkPulses.push({ source: triageNode, target: devNode, progress: 0, speed: 0.02, color: '#6366f1' });
                    }
                }, 400);
            }
        }

        function resetNetworkLayout() {
            initNetworkVisualizer();
        }

        window.emitLiveNetworkPulse = function(sender, recipient) {
            if (!networkNodes || networkNodes.length === 0) return;

            let srcId = sender || 'USER';
            let tgtId = recipient || 'triage-agent';

            let srcNode = networkNodes.find(n => n.id === srcId || n.id.toLowerCase() === srcId.toLowerCase());
            let tgtNode = networkNodes.find(n => n.id === tgtId || n.id.toLowerCase() === tgtId.toLowerCase());

            if (!srcNode) {
                srcNode = networkNodes.find(n => n.category === 'user' || n.id === 'USER') || networkNodes[0];
            }
            if (!tgtNode) {
                tgtNode = networkNodes.find(n => n.category === 'orchestrator' || n.id === 'triage-agent') || networkNodes[1] || networkNodes[0];
            }

            if (srcNode && tgtNode) {
                srcNode.glowUntil = Date.now() + 3500;
                tgtNode.glowUntil = Date.now() + 3500;

                // Spawn 2 traveling energy pulses along the edge
                networkPulses.push({
                    source: srcNode,
                    target: tgtNode,
                    progress: 0,
                    speed: 0.022 + Math.random() * 0.008,
                    color: '#34d399'
                });
                setTimeout(() => {
                    networkPulses.push({
                        source: srcNode,
                        target: tgtNode,
                        progress: 0,
                        speed: 0.025 + Math.random() * 0.008,
                        color: '#6366f1'
                    });
                }, 150);
            }
        };
        const emitLiveNetworkPulse = window.emitLiveNetworkPulse;