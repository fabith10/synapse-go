package web

import (
	"html/template"
)

// DashboardPage is the HTML template for the main status page in ultra-premium dark glassmorphic design.
var DashboardPage = template.Must(template.New("dashboard").Parse(`
<!DOCTYPE html>
<html lang="en" class="h-full bg-zinc-950 text-zinc-100 dark">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Agentic Control Center // Multi-Agent Autonomous Framework</title>
    <!-- Tailwind CSS CDN -->
    <script src="https://cdn.tailwindcss.com" defer></script>
    <!-- Marked.js for markdown rendering in the event stream -->
    <script src="https://cdn.jsdelivr.net/npm/marked/marked.min.js"></script>
    <style>
        @import url('https://fonts.googleapis.com/css2?family=Plus+Jakarta+Sans:wght@300;400;500;600;700;800&family=JetBrains+Mono:wght@400;500;600;700&display=swap');
        
        body { 
            font-family: 'Plus Jakarta Sans', -apple-system, BlinkMacSystemFont, sans-serif; 
            background-color: #09090b; 
            color: #f4f4f5;
            background-image: 
                radial-gradient(at 0% 0%, rgba(99, 102, 241, 0.08) 0px, transparent 50%),
                radial-gradient(at 100% 0%, rgba(168, 85, 247, 0.05) 0px, transparent 50%),
                radial-gradient(at 50% 100%, rgba(16, 185, 129, 0.04) 0px, transparent 50%);
            background-attachment: fixed;
        }
        
        .font-mono { font-family: 'JetBrains Mono', monospace; }
        
        /* Custom Scrollbar Styling */
        ::-webkit-scrollbar { width: 6px; height: 6px; }
        ::-webkit-scrollbar-track { background: rgba(24, 24, 27, 0.6); }
        ::-webkit-scrollbar-thumb { background: rgba(63, 63, 70, 0.8); border-radius: 9999px; }
        ::-webkit-scrollbar-thumb:hover { background: rgba(113, 113, 122, 1); }

        /* Glassmorphism utility */
        .glass-card {
            background: rgba(24, 24, 27, 0.65);
            backdrop-filter: blur(16px);
            -webkit-backdrop-filter: blur(16px);
            border: 1px solid rgba(63, 63, 70, 0.4);
        }
        
        .glass-card-hover {
            transition: all 0.25s cubic-bezier(0.4, 0, 0.2, 1);
        }
        .glass-card-hover:hover {
            border-color: rgba(99, 102, 241, 0.4);
            box-shadow: 0 10px 30px -10px rgba(99, 102, 241, 0.15);
            transform: translateY(-1px);
        }

        /* Markdown rendered inside log entries */
        .md-content h1, .md-content h2, .md-content h3, .md-content h4 {
            font-weight: 700; margin-top: 0.6em; margin-bottom: 0.3em; color: #fafafa;
        }
        .md-content h1 { font-size: 1.05em; border-bottom: 1px solid rgba(63, 63, 70, 0.5); padding-bottom: 0.2em; }
        .md-content h2 { font-size: 0.98em; }
        .md-content h3, .md-content h4 { font-size: 0.92em; }
        .md-content p { margin: 0.3em 0; line-height: 1.6; }
        .md-content strong { font-weight: 700; color: #ffffff; }
        .md-content em { font-style: italic; color: #e4e4e7; }
        .md-content code {
            font-family: 'JetBrains Mono', monospace;
            background: rgba(39, 39, 42, 0.9); 
            border: 1px solid rgba(63, 63, 70, 0.6);
            border-radius: 4px; padding: 1px 5px; font-size: 0.88em; color: #a5f3fc;
        }
        .md-content pre code { background: none; border: none; padding: 0; color: #e4e4e7; }
        .approval-content code {
            font-family: 'JetBrains Mono', monospace;
            background: rgba(24, 24, 27, 0.95); 
            border: 1px solid rgba(63, 63, 70, 0.7);
            border-radius: 4px; padding: 1px 5px; font-size: 0.88em; color: #a5f3fc;
        }
        .approval-content pre {
            background: #05070a; border: 1px solid rgba(63, 63, 70, 0.8);
            border-radius: 8px; padding: 12px 16px; overflow-x: auto;
            margin: 0.5em 0; font-size: 0.86em; line-height: 1.55;
            box-shadow: inset 0 2px 4px rgba(0,0,0,0.6);
        }
        .approval-content pre code { background: none; border: none; padding: 0; color: #34d399; }
        .md-content ul { list-style: disc; padding-left: 1.3em; margin: 0.3em 0; }
        .md-content ol { list-style: decimal; padding-left: 1.3em; margin: 0.3em 0; }
        .md-content li { margin: 0.15em 0; }
        .md-content a { color: #818cf8; text-decoration: underline; text-underline-offset: 2px; }
        .md-content blockquote {
            border-left: 3px solid #6366f1; padding-left: 10px;
            color: #a1a1aa; margin: 0.4em 0; font-style: italic; background: rgba(99, 102, 241, 0.05);
            border-radius: 0 4px 4px 0; padding-top: 4px; padding-bottom: 4px;
        }
        .md-content hr { border-color: rgba(63, 63, 70, 0.5); margin: 0.6em 0; }
        .md-content table { border-collapse: collapse; width: 100%; font-size: 0.86em; margin: 0.4em 0; }
        .md-content th { background: rgba(39, 39, 42, 0.8); font-weight: 700; color: #f4f4f5; }
        .md-content th, .md-content td { border: 1px solid rgba(63, 63, 70, 0.5); padding: 5px 8px; text-align: left; }

        /* ----------------------------------------------------------------- */
        /* Complete Premium Light Theme Overrides                            */
        /* ----------------------------------------------------------------- */
        html.light-theme,
        body.light-theme,
        html.light-theme body {
            background-color: #f8fafc !important;
            color: #0f172a !important;
            background-image: 
                radial-gradient(at 0% 0%, rgba(99, 102, 241, 0.04) 0px, transparent 50%),
                radial-gradient(at 100% 0%, rgba(168, 85, 247, 0.03) 0px, transparent 50%),
                radial-gradient(at 50% 100%, rgba(16, 185, 129, 0.03) 0px, transparent 50%) !important;
        }

        body.light-theme header,
        body.light-theme footer,
        body.light-theme nav,
        body.light-theme section,
        body.light-theme .backdrop-blur-md,
        body.light-theme .backdrop-blur-xl {
            background-color: rgba(255, 255, 255, 0.95) !important;
            border-color: #cbd5e1 !important;
            color: #0f172a !important;
        }

        body.light-theme .glass-card,
        body.light-theme .log-entry,
        body.light-theme [id^="approval-"],
        body.light-theme #artifact-viewer-modal > div,
        body.light-theme #console-logs {
            background-color: #ffffff !important;
            backdrop-filter: blur(16px);
            -webkit-backdrop-filter: blur(16px);
            border-color: #cbd5e1 !important;
            color: #0f172a !important;
            box-shadow: 0 4px 16px -2px rgba(0, 0, 0, 0.05) !important;
        }

        body.light-theme .glass-card-hover:hover {
            border-color: #6366f1 !important;
            box-shadow: 0 10px 30px -5px rgba(99, 102, 241, 0.15) !important;
        }

        /* High Contrast Typography */
        body.light-theme .text-zinc-100,
        body.light-theme .text-zinc-200,
        body.light-theme .text-zinc-300,
        body.light-theme .text-white { color: #0f172a !important; }
        body.light-theme .text-zinc-400 { color: #334155 !important; }
        body.light-theme .text-zinc-500 { color: #64748b !important; }

        /* Sender & Recipient Badges in Light Theme */
        body.light-theme .sender-badge[data-sender="SYSTEM"],
        body.light-theme [data-sender="SYSTEM"] .sender-badge {
            background-color: #fef3c7 !important;
            color: #92400e !important;
            border-color: #fcd34d !important;
        }
        body.light-theme .sender-badge[data-sender="USER"],
        body.light-theme [data-sender="USER"] .sender-badge {
            background-color: #e0e7ff !important;
            color: #3730a3 !important;
            border-color: #c7d2fe !important;
        }
        body.light-theme .sender-badge[data-sender="ADMIN"],
        body.light-theme [data-sender="ADMIN"] .sender-badge {
            background-color: #ffe4e6 !important;
            color: #9f1239 !important;
            border-color: #fecdd3 !important;
        }
        body.light-theme .sender-badge {
            background-color: #e2e8f0 !important;
            color: #1e293b !important;
            border-color: #cbd5e1 !important;
        }
        body.light-theme .recipient-badge {
            background-color: #f1f5f9 !important;
            color: #475569 !important;
            border-color: #cbd5e1 !important;
        }

        /* Solid Action Buttons: Preserve White Text */
        body.light-theme .bg-indigo-600,
        body.light-theme .bg-indigo-700,
        body.light-theme .bg-emerald-600,
        body.light-theme .bg-rose-600,
        body.light-theme button.bg-indigo-600,
        body.light-theme button.bg-emerald-600,
        body.light-theme a.bg-indigo-600 {
            color: #ffffff !important;
        }

        /* Universal Light Background & Border Overrides */
        body.light-theme .bg-zinc-950,
        body.light-theme .bg-zinc-900,
        body.light-theme .bg-zinc-850,
        body.light-theme .bg-zinc-800,
        body.light-theme .bg-zinc-700,
        body.light-theme .bg-slate-950,
        body.light-theme .bg-slate-900,
        body.light-theme .bg-slate-800,
        body.light-theme .bg-black,
        body.light-theme .bg-black\/80,
        body.light-theme .bg-zinc-950\/80,
        body.light-theme .bg-zinc-950\/60,
        body.light-theme .bg-zinc-950\/40,
        body.light-theme .bg-zinc-900\/90,
        body.light-theme .bg-zinc-900\/80,
        body.light-theme .bg-zinc-900\/60,
        body.light-theme .bg-zinc-900\/50,
        body.light-theme .bg-zinc-900\/40,
        body.light-theme #artifact-modal-content-wrapper {
            background-color: #ffffff !important;
            border-color: #cbd5e1 !important;
            color: #0f172a !important;
        }

        body.light-theme .border-zinc-800,
        body.light-theme .border-zinc-800\/90,
        body.light-theme .border-zinc-800\/80,
        body.light-theme .border-zinc-800\/60,
        body.light-theme .border-zinc-800\/40,
        body.light-theme .border-zinc-700,
        body.light-theme .border-zinc-900 {
            border-color: #cbd5e1 !important;
        }

        /* Form Controls & Inputs */
        body.light-theme input[type="text"],
        body.light-theme textarea,
        body.light-theme select {
            background-color: #ffffff !important;
            color: #0f172a !important;
            border-color: #cbd5e1 !important;
        }
        body.light-theme input::placeholder,
        body.light-theme textarea::placeholder {
            color: #94a3b8 !important;
        }

        /* Preset Task Buttons in Light Theme */
        body.light-theme .bg-indigo-950\/60 {
            background-color: #e0e7ff !important;
            color: #3730a3 !important;
            border-color: #c7d2fe !important;
        }
        body.light-theme .bg-emerald-950\/60 {
            background-color: #d1fae5 !important;
            color: #065f46 !important;
            border-color: #a7f3d0 !important;
        }
        body.light-theme .bg-cyan-950\/60 {
            background-color: #cffaff !important;
            color: #164e63 !important;
            border-color: #a5f3fc !important;
        }

        /* High Visibility Warning & Amber Styling in Light Theme */
        body.light-theme .warning-banner,
        html.light-theme .warning-banner {
            background-color: #fffbeb !important; /* Soft warm amber-50 background */
            border-color: #fcd34d !important; /* Gold border */
            color: #78350f !important;
        }

        body.light-theme .warning-title-badge,
        html.light-theme .warning-title-badge {
            background-color: #b45309 !important; /* Rich dark amber badge fill */
            color: #ffffff !important; /* CRISP WHITE TEXT */
        }

        body.light-theme .warning-subtitle,
        html.light-theme .warning-subtitle {
            color: #92400e !important; /* Deep amber-800 text */
            font-weight: 700 !important;
        }

        body.light-theme .warning-list,
        body.light-theme .warning-list li,
        html.light-theme .warning-list li {
            color: #78350f !important; /* Deep amber-900 text */
            font-weight: 600 !important;
        }

        body.light-theme .text-amber-400,
        body.light-theme .text-amber-300,
        body.light-theme .text-amber-200,
        body.light-theme .text-yellow-400,
        body.light-theme .text-yellow-300,
        body.light-theme .text-yellow-500 {
            color: #92400e !important; /* Dark amber-800 for high readability */
            font-weight: 700 !important;
        }

        body.light-theme .bg-amber-500\/10,
        body.light-theme .bg-amber-500\/20,
        body.light-theme .bg-amber-950\/60,
        body.light-theme .bg-amber-900\/40,
        body.light-theme .bg-amber-950\/40 {
            background-color: #fef3c7 !important; /* Rich amber-100 tint */
            color: #92400e !important;
        }

        body.light-theme .border-amber-500\/20,
        body.light-theme .border-amber-500\/30,
        body.light-theme .border-amber-800,
        body.light-theme .border-l-amber-500 {
            border-color: #d97706 !important;
        }

        /* Emerald / Success Overrides */
        body.light-theme .text-emerald-400,
        body.light-theme .text-emerald-300 {
            color: #047857 !important;
            font-weight: 700 !important;
        }
        body.light-theme .bg-emerald-500\/10,
        body.light-theme .bg-emerald-500\/20,
        body.light-theme .bg-emerald-950\/60 {
            background-color: #d1fae5 !important;
            color: #065f46 !important;
        }

        /* Indigo / Primary Overrides */
        body.light-theme .text-indigo-400,
        body.light-theme .text-indigo-300 {
            color: #4338ca !important;
            font-weight: 700 !important;
        }
        body.light-theme .bg-indigo-500\/10,
        body.light-theme .bg-indigo-500\/20,
        body.light-theme .bg-indigo-950\/60 {
            background-color: #e0e7ff !important;
            color: #3730a3 !important;
        }

        /* Rose / Danger Overrides */
        body.light-theme .text-rose-400,
        body.light-theme .text-rose-300 {
            color: #be123c !important;
            font-weight: 700 !important;
        }
        body.light-theme .bg-rose-500\/10,
        body.light-theme .bg-rose-500\/20,
        body.light-theme .bg-rose-950\/60 {
            background-color: #ffe4e6 !important;
            color: #9f1239 !important;
        }

        /* Markdown Overrides */
        body.light-theme .md-content h1,
        body.light-theme .md-content h2,
        body.light-theme .md-content h3,
        body.light-theme .md-content strong {
            color: #0f172a !important;
        }
        body.light-theme .md-content p,
        body.light-theme .md-content li {
            color: #1e293b !important;
        }
        body.light-theme .md-content code {
            background: #e2e8f0 !important;
            border-color: #cbd5e1 !important;
            color: #0369a1 !important;
        }
        body.light-theme .md-content pre {
            background: #0f172a !important;
            color: #f8fafc !important;
            border-color: #334155 !important;
        }
    </style>
</head>
<body hx-ext="sse" sse-connect="/stream/logs" class="h-full flex flex-col selection:bg-indigo-500 selection:text-white">

    <!-- Top Header -->
    <header class="border-b border-zinc-800/80 bg-zinc-950/80 backdrop-blur-xl sticky top-0 z-50">
        <div class="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8 h-16 flex items-center justify-between">
            <div class="flex items-center gap-3">
                <div class="w-8 h-8 rounded-lg bg-gradient-to-tr from-indigo-600 via-purple-600 to-indigo-400 p-[1px] shadow-lg shadow-indigo-500/20">
                    <div class="w-full h-full bg-zinc-950 rounded-[7px] flex items-center justify-center font-bold text-transparent bg-clip-text bg-gradient-to-r from-indigo-400 to-purple-300 text-sm">
                        ⚡
                    </div>
                </div>
                <div>
                    <h1 class="text-sm font-extrabold tracking-wider text-zinc-100 uppercase flex items-center gap-2">
                        <span>Agentic Control Center</span>
                        <span class="text-[9px] font-mono font-bold bg-indigo-500/10 text-indigo-400 border border-indigo-500/20 px-2 py-0.5 rounded-full lowercase">v2.4.0</span>
                    </h1>
                    <p class="text-[10px] text-zinc-400 font-mono">Go Orchestration Tier // Ephemeral WASM &amp; Docker Sandboxing</p>
                </div>
            </div>

            <!-- Live Status & System Indicators -->
            <div class="flex items-center gap-4">
                <button id="theme-toggle-btn" onclick="toggleTheme()" class="flex items-center gap-1.5 px-3 py-1.5 bg-zinc-900 border border-zinc-800 hover:border-zinc-700 text-zinc-300 rounded-full text-xs font-mono font-bold transition-all cursor-pointer shadow-sm">
                    <span id="theme-toggle-icon">☀️</span> <span id="theme-toggle-text" class="hidden sm:inline">Light Mode</span>
                </button>

                {{ if .Warnings }}
                <div class="hidden sm:flex items-center gap-1.5 px-3 py-1 bg-amber-500/10 text-amber-400 border border-amber-500/30 rounded-full text-[10px] font-mono font-bold uppercase tracking-wider" title="System warnings or mock settings connected">
                    <span>⚠️</span> <span>Mock/Warnings Active</span>
                </div>
                {{ end }}

                <div class="hidden md:flex items-center gap-3 border-r border-zinc-800 pr-4">
                    <div class="flex items-center gap-1.5 text-[10px] font-mono text-zinc-400">
                        <span class="w-2 h-2 rounded-full bg-emerald-500"></span>
                        <span>Docker: Connected</span>
                    </div>
                    <div class="flex items-center gap-1.5 text-[10px] font-mono text-zinc-400">
                        <span class="w-2 h-2 rounded-full bg-indigo-500"></span>
                        <span>Wasm: Active</span>
                    </div>
                </div>

                <div class="flex items-center gap-2 bg-zinc-900/80 border border-zinc-800 px-3 py-1.5 rounded-full shadow-inner">
                    <span class="relative flex h-2.5 w-2.5">
                      <span class="animate-ping absolute inline-flex h-full w-full rounded-full bg-emerald-400 opacity-75"></span>
                      <span class="relative inline-flex rounded-full h-2.5 w-2.5 bg-emerald-500"></span>
                    </span>
                    <span class="text-[10px] font-bold text-zinc-300 font-mono tracking-wider uppercase">SSE Stream Live</span>
                </div>
            </div>
        </div>
    </header>

    <!-- Navigation Tabs -->
    <div class="border-b border-zinc-800/60 bg-zinc-950/60 backdrop-blur-md">
        <div class="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8">
            <nav class="flex space-x-6" aria-label="Tabs">
                <button id="tab-btn-dashboard" onclick="switchTab('dashboard')" class="border-b-2 border-indigo-500 py-3.5 px-2 text-xs font-bold uppercase tracking-wider text-indigo-400 flex items-center gap-2 transition-all cursor-pointer">
                    <span>⚡ Control Center</span>
                </button>
                <button id="tab-btn-studio" onclick="switchTab('studio')" class="border-b-2 border-transparent py-3.5 px-2 text-xs font-bold uppercase tracking-wider text-zinc-400 hover:text-zinc-200 flex items-center gap-2 transition-all cursor-pointer">
                    <span>🎨 Agent Studio</span>
                </button>
                <button id="tab-btn-audit" onclick="switchTab('audit')" class="border-b-2 border-transparent py-3.5 px-2 text-xs font-bold uppercase tracking-wider text-zinc-400 hover:text-zinc-200 flex items-center gap-2 transition-all cursor-pointer">
                    <span>📊 Cost &amp; Audit</span>
                </button>
                <button id="tab-btn-network" onclick="switchTab('network')" class="border-b-2 border-transparent py-3.5 px-2 text-xs font-bold uppercase tracking-wider text-zinc-400 hover:text-zinc-200 flex items-center gap-2 transition-all cursor-pointer">
                    <span>🕸️ Network Graph</span>
                </button>
                <button id="tab-btn-config" onclick="switchTab('config')" class="border-b-2 border-transparent py-3.5 px-2 text-xs font-bold uppercase tracking-wider text-zinc-400 hover:text-zinc-200 flex items-center gap-2 transition-all cursor-pointer">
                    <span>⚙️ System Settings</span>
                </button>
                <button id="tab-btn-scheduler" onclick="switchTab('scheduler')" class="border-b-2 border-transparent py-3.5 px-2 text-xs font-bold uppercase tracking-wider text-zinc-400 hover:text-zinc-200 flex items-center gap-2 transition-all cursor-pointer">
                    <span>⏱️ Cron Scheduler</span>
                </button>
            </nav>
        </div>
    </div>

    <!-- System Config Warnings Banner -->
    {{ if .Warnings }}
    <div class="bg-amber-950/40 border-b border-amber-500/30 px-4 py-3 text-amber-200 backdrop-blur-md warning-banner">
        <div class="max-w-7xl mx-auto flex flex-col gap-2">
            <div class="flex items-center gap-2">
                <span class="text-[10px] font-bold uppercase tracking-wider bg-amber-500 text-zinc-950 px-2 py-0.5 rounded font-mono shadow-sm warning-title-badge">System Warnings</span>
                <span class="text-xs text-amber-300 font-mono warning-subtitle">Environment setup recommendations:</span>
            </div>
            <ul class="list-disc list-inside text-xs space-y-1 text-amber-200/90 font-mono warning-list">
                {{ range .Warnings }}
                <li>{{ . }}</li>
                {{ end }}
            </ul>
        </div>
    </div>
    {{ end }}

    <!-- Control Center Tab Content -->
    <div id="tab-content-dashboard" class="tab-pane flex-1 flex flex-col">
        <main class="flex-1 max-w-7xl w-full mx-auto px-4 sm:px-6 lg:px-8 py-8 grid grid-cols-1 lg:grid-cols-3 gap-8">
            
            <!-- Left Column: Tasks, Controls & Event Stream (Taking 2/3 width) -->
            <div class="space-y-8 lg:col-span-2">
                
                <!-- Task Dispatcher Card -->
                <section class="glass-card rounded-2xl p-6 shadow-2xl relative overflow-hidden">
                    <div class="absolute top-0 right-0 w-64 h-64 bg-indigo-500/5 rounded-full filter blur-3xl pointer-events-none"></div>
                    <div class="flex items-center justify-between mb-4 border-b border-zinc-800/80 pb-3">
                        <div class="flex items-center gap-2">
                            <span class="w-2.5 h-2.5 rounded-full bg-indigo-500 shadow-sm shadow-indigo-500/50"></span>
                            <h2 class="text-xs font-extrabold uppercase tracking-widest text-zinc-100">Task Dispatcher</h2>
                        </div>
                        <span class="text-[9px] font-mono bg-indigo-500/10 text-indigo-400 border border-indigo-500/20 px-2.5 py-0.5 rounded-full uppercase">Dynamic Routing</span>
                    </div>

                    <form id="task-form" hx-post="/api/task" hx-swap="none" enctype="multipart/form-data" class="space-y-4">
                        <p class="text-xs text-zinc-400 leading-relaxed">
                            Specify your task goal below. The system triage-agent will evaluate requirements, select the optimal specialist agent, and execute sandboxed actions.
                        </p>
                        
                        <div>
                            <div class="flex items-center gap-2 flex-wrap mb-2.5">
                                <span class="text-[10px] font-mono text-zinc-400 font-bold uppercase">Preset Task Launchers:</span>
                                <button type="button" onclick="setPromptTask('Search the web for AI market news and compile an executive PDF report')" class="text-[10px] font-mono bg-indigo-950/60 hover:bg-indigo-900/80 border border-indigo-800/80 text-indigo-300 px-2.5 py-1 rounded-lg transition-all active:scale-95 cursor-pointer shadow-sm">⚡ Web Search &amp; PDF Report</button>
                                <button type="button" onclick="setPromptTask('Inspect financial workbook in chicago_offsite_budget.xlsx and audit expense metrics')" class="text-[10px] font-mono bg-emerald-950/60 hover:bg-emerald-900/80 border border-emerald-800/80 text-emerald-300 px-2.5 py-1 rounded-lg transition-all active:scale-95 cursor-pointer shadow-sm">📊 Excel Model Audit</button>
                                <button type="button" onclick="setPromptTask('Inspect host system hardware and query spot GPU rental prices')" class="text-[10px] font-mono bg-cyan-950/60 hover:bg-cyan-900/80 border border-cyan-800/80 text-cyan-300 px-2.5 py-1 rounded-lg transition-all active:scale-95 cursor-pointer shadow-sm">🖥️ Host Hardware Audit</button>
                            </div>
                            <textarea name="content" id="content" rows="3" required 
                                placeholder="e.g. Search web for latest tech news and compile a PDF summary report" 
                                class="w-full bg-zinc-950/80 border border-zinc-800 rounded-xl px-4 py-3 text-sm focus:outline-none focus:border-indigo-500 focus:ring-2 focus:ring-indigo-500/20 text-zinc-100 placeholder-zinc-500 resize-none transition-all shadow-inner"></textarea>
                        </div>

                        <!-- File Attachment & Orchestration Settings Grid -->
                        <div class="grid grid-cols-1 sm:grid-cols-2 gap-4 pt-1">
                            <div class="space-y-1.5">
                                <label class="block text-[10px] font-bold text-zinc-400 uppercase tracking-wider font-mono">Context Attachment (Optional)</label>
                                <div class="flex items-center gap-3">
                                    <input type="file" name="file" id="task-file" class="hidden" onchange="document.getElementById('file-chosen').textContent = this.files[0] ? this.files[0].name : 'No file chosen'" />
                                    <label for="task-file" class="px-4 py-2 border border-zinc-700 hover:border-zinc-500 rounded-lg text-[10px] font-bold uppercase tracking-wider transition-all cursor-pointer bg-zinc-900 hover:bg-zinc-800 text-zinc-200 active:scale-95 shadow-sm inline-flex items-center gap-2">
                                        <span>📁</span> <span>Choose File</span>
                                    </label>
                                    <span id="file-chosen" class="text-[10px] text-zinc-400 font-mono italic truncate max-w-[140px]">No file chosen</span>
                                </div>
                            </div>

                            <div class="space-y-1.5">
                                <label class="block text-[10px] font-bold text-zinc-400 uppercase tracking-wider font-mono">Orchestration Mode</label>
                                <select name="orchestration_mode" id="orchestration_mode" class="w-full bg-zinc-950 border border-zinc-800 rounded-lg px-3 py-2 text-xs text-zinc-200 focus:outline-none focus:border-indigo-500 focus:ring-2 focus:ring-indigo-500/20 cursor-pointer shadow-sm">
                                    <option value="autonomous" selected>Fully Autonomous (Execute immediately)</option>
                                    <option value="steered">Steered (Review &amp; Approve Plan first)</option>
                                    <option value="deferred">Off-Peak Deferred (Oracle schedules optimal low-cost time)</option>
                                </select>
                            </div>
                        </div>

                        <button type="submit" class="w-full py-3.5 bg-gradient-to-r from-indigo-600 via-indigo-500 to-purple-600 hover:from-indigo-500 hover:to-purple-500 text-white rounded-xl text-xs font-bold uppercase tracking-wider transition-all shadow-lg shadow-indigo-500/20 active:scale-[0.98] cursor-pointer flex items-center justify-center gap-2">
                            <span>Dispatch Autonomous Task</span>
                            <span class="text-sm">➔</span>
                        </button>
                    </form>
                </section>

                <!-- Steering Controls (HITL Approval Queue) -->
                <section class="glass-card rounded-2xl p-6 shadow-2xl">
                    <div class="flex items-center justify-between mb-4 border-b border-zinc-800/80 pb-3">
                        <h2 class="text-xs font-extrabold uppercase tracking-widest text-zinc-100 flex items-center gap-2">
                            <span class="w-2.5 h-2.5 rounded-full bg-amber-500 animate-pulse"></span>
                            Human-in-the-Loop Steering Queue
                        </h2>
                        <span class="text-[9px] font-mono text-zinc-400">Security Gateways Active</span>
                    </div>
                    
                    <div id="approvals-list" sse-swap="hitl-request" hx-swap="beforeend" class="space-y-4">
                        <div class="empty-state text-[11px] text-zinc-400 italic p-6 border border-dashed border-zinc-800 rounded-xl text-center font-mono bg-zinc-950/40">
                            AWAITING INSTRUCTIONS. STEERING CONTROLS WILL POPULATE HERE UPON CRITICAL ACTIONS.
                        </div>
                    </div>
                </section>

                <!-- Real-time Console Log Stream -->
                <section class="glass-card rounded-2xl p-6 shadow-2xl flex flex-col h-[520px]">
                    <div class="flex items-center justify-between mb-4 border-b border-zinc-800/80 pb-3 flex-wrap gap-2">
                        <div class="flex items-center gap-2" id="log-filter-bar">
                            <span class="text-xs font-extrabold uppercase tracking-widest text-zinc-300 flex items-center gap-2 mr-2">
                                <span class="w-2 h-2 rounded-full bg-emerald-500 animate-pulse"></span>
                                Event Stream
                            </span>
                            <button onclick="setLogFilter('all')" id="filter-btn-all" class="px-3 py-1 text-[9px] font-mono font-bold uppercase tracking-wider bg-indigo-600 text-white rounded-md cursor-pointer transition-all active:scale-95 shadow-sm">All</button>
                            <button onclick="setLogFilter('user')" id="filter-btn-user" class="px-3 py-1 text-[9px] font-mono font-bold uppercase tracking-wider bg-zinc-900 border border-zinc-800 text-zinc-400 hover:text-zinc-200 rounded-md cursor-pointer transition-all active:scale-95">User Chat</button>
                            <button onclick="setLogFilter('orchestration')" id="filter-btn-orchestration" class="px-3 py-1 text-[9px] font-mono font-bold uppercase tracking-wider bg-zinc-900 border border-zinc-800 text-zinc-400 hover:text-zinc-200 rounded-md cursor-pointer transition-all active:scale-95">Orchestration</button>
                            <button onclick="setLogFilter('specialists')" id="filter-btn-specialists" class="px-3 py-1 text-[9px] font-mono font-bold uppercase tracking-wider bg-zinc-900 border border-zinc-800 text-zinc-400 hover:text-zinc-200 rounded-md cursor-pointer transition-all active:scale-95">Specialists</button>
                        </div>
                        <span class="text-[9px] font-mono text-zinc-400 bg-zinc-900 border border-zinc-800 px-2 py-0.5 rounded">SSE Live Feed</span>
                    </div>

                    <div id="console-logs" sse-swap="log-message" hx-swap="beforeend"
                         hx-on::after-settle="this.scrollTop = this.scrollHeight"
                         class="flex-1 font-mono text-[11px] overflow-y-auto bg-zinc-950 text-zinc-200 p-4 rounded-xl border border-zinc-800/90 space-y-3 scroll-smooth shadow-inner">
                        <div class="text-zinc-500 italic font-mono text-xs text-center py-8">[Ready to process autonomous agent telemetry...]</div>
                    </div>
                </section>

                <!-- Agent Artifacts Panel -->
                <section class="glass-card rounded-2xl p-6 shadow-2xl">
                    <div class="flex items-center justify-between mb-4 border-b border-zinc-800/80 pb-3">
                        <h2 class="text-xs font-extrabold uppercase tracking-widest text-zinc-100 flex items-center gap-2">
                            <span>📎</span>
                            Agent Artifacts &amp; Generated Deliverables
                        </h2>
                        <div class="flex items-center gap-3 font-mono">
                            <span class="text-[9px] text-zinc-400">Reports, Code, PDF &amp; Workbooks</span>
                            <button hx-post="/api/artifact/delete-all" hx-target="#artifacts-list" hx-swap="innerHTML" hx-confirm="Are you sure you want to delete ALL generated artifacts from disk?"
                                    class="px-2.5 py-1 bg-zinc-900 hover:bg-rose-950/80 text-zinc-400 hover:text-rose-300 border border-zinc-800 hover:border-rose-800 rounded-lg text-[9px] font-mono font-bold uppercase tracking-wider transition-all active:scale-95 cursor-pointer flex items-center gap-1">
                                <span>🗑️</span> <span>Clear All</span>
                            </button>
                        </div>
                    </div>
                    <div id="artifacts-list" sse-swap="artifact-ready" hx-swap="afterbegin" class="space-y-3">
                        {{if .ArtifactsHTML}}
                            {{.ArtifactsHTML}}
                        {{else}}
                            <div id="artifacts-empty" class="text-[11px] text-zinc-400 italic p-6 border border-dashed border-zinc-800 rounded-xl text-center font-mono bg-zinc-950/40">
                                NO ARTIFACTS PRODUCED YET. GENERATED DELIVERABLES WILL BE CAPTURED HERE.
                            </div>
                        {{end}}
                    </div>
                </section>
            </div>

            <!-- Right Column: Agent Registry Sidebar (Taking 1/3 width) -->
            <div class="space-y-8 lg:col-span-1">
                <section class="glass-card rounded-2xl p-6 shadow-2xl overflow-y-auto max-h-[920px]">
                    <div class="flex items-center justify-between mb-4 border-b border-zinc-800/80 pb-3">
                        <h2 class="text-xs font-extrabold uppercase tracking-widest text-zinc-200">Registered Blueprints</h2>
                        <span class="text-[9px] font-mono bg-emerald-500/10 text-emerald-400 border border-emerald-500/20 px-2 py-0.5 rounded-full uppercase">Dynamic Active</span>
                    </div>
                    <div class="space-y-4">
                        
                        <div class="p-4 rounded-xl bg-zinc-900/80 border border-zinc-800 flex flex-col gap-2 shadow-sm glass-card-hover">
                            <div class="flex justify-between items-center">
                                <span class="font-bold text-zinc-100 text-xs font-mono">triage-agent</span>
                                <span class="text-[9px] font-mono bg-cyan-500/10 text-cyan-400 border border-cyan-500/20 px-2 py-0.5 rounded uppercase">Gatekeeper</span>
                            </div>
                            <p class="text-xs text-zinc-400 leading-relaxed">Analyzes prompt requirements, resolves target capabilities, and routes compute efficiently.</p>
                            <div class="flex flex-wrap gap-1 mt-1">
                                <span class="text-[9px] font-mono bg-zinc-950 text-zinc-400 border border-zinc-800 px-2 py-0.5 rounded">query_pricing_oracle</span>
                            </div>
                        </div>

                        <div class="p-4 rounded-xl bg-zinc-900/80 border border-zinc-800 flex flex-col gap-2 shadow-sm glass-card-hover">
                            <div class="flex justify-between items-center">
                                <span class="font-bold text-zinc-100 text-xs font-mono">developer-agent</span>
                                <span class="text-[9px] font-mono bg-emerald-500/10 text-emerald-400 border border-emerald-500/20 px-2 py-0.5 rounded uppercase">Developer</span>
                            </div>
                            <p class="text-xs text-zinc-400 leading-relaxed">Executes code, inspects workspace directories, refactors codebases, and performs git operations.</p>
                            <div class="flex flex-wrap gap-1.5 mt-1">
                                <span class="text-[9px] font-mono bg-zinc-950 text-zinc-400 border border-zinc-800 px-2 py-0.5 rounded">execute_bash_docker</span>
                                <span class="text-[9px] font-mono bg-zinc-950 text-zinc-400 border border-zinc-800 px-2 py-0.5 rounded">replace_file_content</span>
                            </div>
                        </div>

                        <div class="p-4 rounded-xl bg-zinc-900/80 border border-zinc-800 flex flex-col gap-2 shadow-sm glass-card-hover">
                            <div class="flex justify-between items-center">
                                <span class="font-bold text-zinc-100 text-xs font-mono">quant-agent</span>
                                <span class="text-[9px] font-mono bg-purple-500/10 text-purple-400 border border-purple-500/20 px-2 py-0.5 rounded uppercase">Quant Sandboxer</span>
                            </div>
                            <p class="text-xs text-zinc-400 leading-relaxed">Writes Python code to perform financial modeling and Monte Carlo simulations inside Docker containers.</p>
                            <div class="flex flex-wrap gap-1 mt-1">
                                <span class="text-[9px] font-mono bg-zinc-950 text-zinc-400 border border-zinc-800 px-2 py-0.5 rounded">execute_python_docker</span>
                            </div>
                        </div>

                        <div class="p-4 rounded-xl bg-zinc-900/80 border border-zinc-800 flex flex-col gap-2 shadow-sm glass-card-hover">
                            <div class="flex justify-between items-center">
                                <span class="font-bold text-zinc-100 text-xs font-mono">excel-agent</span>
                                <span class="text-[9px] font-mono bg-green-500/10 text-green-400 border border-green-500/20 px-2 py-0.5 rounded uppercase">Spreadsheet Hero</span>
                            </div>
                            <p class="text-xs text-zinc-400 leading-relaxed">Modifies financial spreadsheets, updates cell formulas, and extracts model assumptions natively.</p>
                            <div class="flex flex-wrap gap-1 mt-1">
                                <span class="text-[9px] font-mono bg-zinc-950 text-zinc-400 border border-zinc-800 px-2 py-0.5 rounded">modify_excel_workbook</span>
                            </div>
                        </div>

                        <div class="p-4 rounded-xl bg-zinc-900/80 border border-zinc-800 flex flex-col gap-2 shadow-sm glass-card-hover">
                            <div class="flex justify-between items-center">
                                <span class="font-bold text-zinc-100 text-xs font-mono">researcher-agent</span>
                                <span class="text-[9px] font-mono bg-blue-500/10 text-blue-400 border border-blue-500/20 px-2 py-0.5 rounded uppercase">Deep Researcher</span>
                            </div>
                            <p class="text-xs text-zinc-400 leading-relaxed">Searches live web context, parses search extracts, and compiles PDF research reports natively.</p>
                            <div class="flex flex-wrap gap-1.5 mt-1">
                                <span class="text-[9px] font-mono bg-zinc-950 text-zinc-400 border border-zinc-800 px-2 py-0.5 rounded">web_search_and_extract</span>
                                <span class="text-[9px] font-mono bg-zinc-950 text-zinc-400 border border-zinc-800 px-2 py-0.5 rounded">generate_pdf_report</span>
                            </div>
                        </div>

                        <div class="p-4 rounded-xl bg-zinc-900/80 border border-zinc-800 flex flex-col gap-2 shadow-sm glass-card-hover">
                            <div class="flex justify-between items-center">
                                <span class="font-bold text-zinc-100 text-xs font-mono">email-agent</span>
                                <span class="text-[9px] font-mono bg-amber-500/10 text-amber-400 border border-amber-500/20 px-2 py-0.5 rounded uppercase">Email Assistant</span>
                            </div>
                            <p class="text-xs text-zinc-400 leading-relaxed">Drafts formatted emails with automatic recipient extraction and security checks for external domains.</p>
                            <div class="flex flex-wrap gap-1 mt-1">
                                <span class="text-[9px] font-mono bg-zinc-950 text-zinc-400 border border-zinc-800 px-2 py-0.5 rounded">write_email</span>
                            </div>
                        </div>

                        <div class="p-4 rounded-xl bg-zinc-900/80 border border-zinc-800 flex flex-col gap-2 shadow-sm glass-card-hover">
                            <div class="flex justify-between items-center">
                                <span class="font-bold text-zinc-100 text-xs font-mono">browser-agent</span>
                                <span class="text-[9px] font-mono bg-pink-500/10 text-pink-400 border border-pink-500/20 px-2 py-0.5 rounded uppercase">Web Browser Agent</span>
                            </div>
                            <p class="text-xs text-zinc-400 leading-relaxed">Navigates websites statefully, enters input values, and submits forms step-by-step.</p>
                            <div class="flex flex-wrap gap-1.5 mt-1">
                                <span class="text-[9px] font-mono bg-zinc-950 text-zinc-400 border border-zinc-800 px-2 py-0.5 rounded">browser_navigate</span>
                                <span class="text-[9px] font-mono bg-zinc-950 text-zinc-400 border border-zinc-800 px-2 py-0.5 rounded">browser_input</span>
                            </div>
                        </div>

                        <div class="p-4 rounded-xl bg-zinc-900/80 border border-zinc-800 flex flex-col gap-2 shadow-sm glass-card-hover">
                            <div class="flex justify-between items-center">
                                <span class="font-bold text-zinc-100 text-xs font-mono">planner-agent</span>
                                <span class="text-[9px] font-mono bg-indigo-500/10 text-indigo-400 border border-indigo-500/20 px-2 py-0.5 rounded uppercase">Task Planner</span>
                            </div>
                            <p class="text-xs text-zinc-400 leading-relaxed">Dissects complex instructions into sub-tasks with dependencies and executes them in parallel.</p>
                            <div class="flex flex-wrap gap-1 mt-1">
                                <span class="text-[9px] font-mono bg-zinc-950 text-zinc-400 border border-zinc-800 px-2 py-0.5 rounded">dag scheduling</span>
                            </div>
                        </div>

                    </div>
                </section>
            </div>
        </main>
    </div>

    <!-- Configurations Tab Content -->
    <div id="tab-content-config" class="tab-pane hidden flex-1 flex flex-col">
        <main class="flex-1 max-w-7xl w-full mx-auto px-4 sm:px-6 lg:px-8 py-8">
            <div class="glass-card rounded-2xl p-8 shadow-2xl">
                
                <div class="flex flex-col sm:flex-row sm:items-center sm:justify-between mb-8 border-b border-zinc-800 pb-5 gap-4">
                    <div>
                        <h2 class="text-sm font-extrabold uppercase tracking-wider text-zinc-100">System Configurations</h2>
                        <p class="text-xs text-zinc-400 mt-1">Configure agent system prompts, tool blocklists, and core LLM provider models.</p>
                    </div>
                    <div>
                        <button type="submit" form="config-form" class="px-6 py-2.5 bg-gradient-to-r from-indigo-600 to-purple-600 hover:from-indigo-500 hover:to-purple-500 text-white rounded-lg text-xs font-bold uppercase tracking-wider transition-all shadow-lg shadow-indigo-500/20 active:scale-95 cursor-pointer">
                            Save Configurations
                        </button>
                    </div>
                </div>

                <form id="config-form" hx-post="/api/config/save" hx-target="#save-status" class="space-y-8">
                    <!-- Status Box -->
                    <div id="save-status"></div>

                    <!-- Dynamic Agent Prompts Grid -->
                    <div>
                        <h3 class="text-xs font-extrabold uppercase tracking-widest text-indigo-400 mb-4 border-b border-zinc-800 pb-2">Agent System Prompts</h3>
                        <div class="grid grid-cols-1 md:grid-cols-2 gap-6">
                            
                            {{ range $id, $prompt := .AgentsConfig }}
                            <div class="space-y-1.5">
                                <label for="{{$id}}_prompt" class="block text-[10px] font-bold text-zinc-400 uppercase font-mono">{{$id}} Prompt</label>
                                <textarea name="{{$id}}_prompt" id="{{$id}}_prompt" rows="7" 
                                    class="w-full bg-zinc-950 border border-zinc-800 rounded-xl px-3 py-2.5 text-xs font-mono text-zinc-200 focus:outline-none focus:border-indigo-500 focus:ring-2 focus:ring-indigo-500/20 resize-y shadow-inner transition-all">{{$prompt}}</textarea>
                            </div>
                            {{ end }}

                        </div>
                    </div>

                    <!-- Human-in-the-Loop Command & Tool Approvals -->
                    <div class="pt-6 border-t border-zinc-800">
                        <div class="flex items-center justify-between mb-4 border-b border-zinc-800 pb-3 flex-wrap gap-2">
                            <div>
                                <h3 class="text-xs font-extrabold uppercase tracking-widest text-indigo-400">Human-in-the-Loop &amp; Security Controls</h3>
                                <p class="text-[10px] text-zinc-400 font-mono mt-0.5">Configure command blocklists, risky Python syscalls, and tool approval triggers (.agents/critical_actions.json).</p>
                            </div>
                            <label class="flex items-center space-x-2 cursor-pointer bg-zinc-900 border border-zinc-800 px-3.5 py-2 rounded-lg hover:bg-zinc-800 transition-colors">
                                <input type="checkbox" name="auto_approve_all" {{if .CriticalActions.AutoApproveAll}}checked{{end}} class="w-4 h-4 text-indigo-600 border-zinc-700 rounded focus:ring-indigo-500">
                                <span class="text-[10px] font-bold font-mono text-zinc-300 uppercase">Auto-Approve All (Auto-Pilot Mode)</span>
                            </label>
                        </div>

                        <div class="grid grid-cols-1 md:grid-cols-3 gap-6">
                            <div class="space-y-1.5">
                                <label for="risky_bash_keywords" class="block text-[10px] font-bold text-zinc-400 uppercase font-mono">Risky Bash Commands (comma/newline separated)</label>
                                <textarea name="risky_bash_keywords" id="risky_bash_keywords" rows="5" class="w-full bg-zinc-950 border border-zinc-800 rounded-xl px-3 py-2.5 text-xs font-mono text-zinc-200 focus:outline-none focus:border-indigo-500 focus:ring-2 focus:ring-indigo-500/20 resize-y shadow-inner" placeholder="rm, rmdir, sudo, chmod, curl, pkill">{{.RiskyBashKeywordsStr}}</textarea>
                            </div>

                            <div class="space-y-1.5">
                                <label for="risky_python_keywords" class="block text-[10px] font-bold text-zinc-400 uppercase font-mono">Risky Python Syscalls (comma/newline separated)</label>
                                <textarea name="risky_python_keywords" id="risky_python_keywords" rows="5" class="w-full bg-zinc-950 border border-zinc-800 rounded-xl px-3 py-2.5 text-xs font-mono text-zinc-200 focus:outline-none focus:border-indigo-500 focus:ring-2 focus:ring-indigo-500/20 resize-y shadow-inner" placeholder="shutil.rmtree, os.remove, os.system, subprocess, eval">{{.RiskyPythonKeywordsStr}}</textarea>
                            </div>

                            <div class="space-y-1.5">
                                <label for="require_approval_tools" class="block text-[10px] font-bold text-zinc-400 uppercase font-mono">Tools Requiring HITL Approval (comma/newline separated)</label>
                                <textarea name="require_approval_tools" id="require_approval_tools" rows="5" class="w-full bg-zinc-950 border border-zinc-800 rounded-xl px-3 py-2.5 text-xs font-mono text-zinc-200 focus:outline-none focus:border-indigo-500 focus:ring-2 focus:ring-indigo-500/20 resize-y shadow-inner" placeholder="execute_bash_docker, write_email">{{.RequireApprovalToolsStr}}</textarea>
                            </div>
                        </div>
                    </div>

                    <!-- LLM Providers raw JSON editing -->
                    <div class="pt-6 border-t border-zinc-800">
                        <h3 class="text-xs font-extrabold uppercase tracking-widest text-indigo-400 mb-2 border-b border-zinc-800 pb-2">LLM Providers (models.json)</h3>
                        <p class="text-[10px] text-zinc-400 font-mono mb-4">Edit the active provider endpoints below. Updates require a server restart to take effect.</p>
                        <div class="space-y-1.5">
                            <textarea name="models_json" id="models_json" rows="10" class="w-full bg-zinc-950 border border-zinc-800 rounded-xl px-3 py-2.5 text-xs font-mono text-zinc-200 focus:outline-none focus:border-indigo-500 focus:ring-2 focus:ring-indigo-500/20 resize-y shadow-inner">{{.ModelsConfig}}</textarea>
                        </div>
                    </div>
                </form>
            </div>
        </main>
    </div>

    <!-- Agent Studio Tab Content (No-Code Builder & Blueprint Studio) -->
    <div id="tab-content-studio" class="tab-pane hidden flex-1 flex flex-col">
        <main class="flex-1 max-w-7xl w-full mx-auto px-4 sm:px-6 lg:px-8 py-8 space-y-8">
            
            <!-- Header & Admin Controls -->
            <div class="flex flex-col md:flex-row md:items-center md:justify-between border-b border-zinc-800 pb-5 gap-4">
                <div>
                    <h2 class="text-sm font-extrabold uppercase tracking-wider text-zinc-100 flex items-center gap-2">
                        <span>🎨 Agent Studio &amp; No-Code Builder</span>
                        <span class="text-[9px] font-mono font-bold bg-indigo-500/10 text-indigo-400 border border-indigo-500/20 px-2.5 py-0.5 rounded-full uppercase">Dynamic Registry</span>
                    </h2>
                    <p class="text-xs text-zinc-400 mt-1">Create custom specialized agents, assign tool permissions visually, and import/export team blueprints without modifying Go source code.</p>
                </div>
                
                <!-- Admin Policy Toggle for Docker Tier 3 Tools -->
                <div class="glass-card px-4 py-3 rounded-xl border border-zinc-800 flex items-center gap-4 shadow-md">
                    <div class="flex flex-col">
                        <span class="text-[10px] font-bold uppercase tracking-wider text-zinc-200 font-mono">Tier 3 Docker Tools</span>
                        <span id="docker-policy-status-label" class="text-[9px] font-mono text-amber-400 font-bold">Gated by Admin Policy</span>
                    </div>
                    <label class="relative inline-flex items-center cursor-pointer">
                        <input type="checkbox" id="docker-policy-checkbox" onchange="toggleDockerAdminPolicy(this.checked)" class="sr-only peer">
                        <div class="w-11 h-6 bg-zinc-800 peer-focus:outline-none rounded-full peer peer-checked:after:translate-x-full peer-checked:after:border-white after:content-[''] after:absolute after:top-[2px] after:left-[2px] after:bg-white after:border-zinc-300 after:border after:rounded-full after:h-5 after:w-5 after:transition-all peer-checked:bg-emerald-600"></div>
                    </label>
                </div>
            </div>

            <!-- Two Column Studio Layout -->
            <div class="grid grid-cols-1 lg:grid-cols-3 gap-8">
                
                <!-- Left 2 Columns: Visual Agent Creator Form -->
                <div class="lg:col-span-2 space-y-6">
                    <section class="glass-card rounded-2xl p-6 shadow-2xl space-y-6">
                        <div class="flex items-center justify-between border-b border-zinc-800 pb-3">
                            <h3 class="text-xs font-extrabold uppercase tracking-widest text-zinc-100 flex items-center gap-2">
                                <span class="w-2.5 h-2.5 rounded-full bg-indigo-500"></span>
                                Visual Agent Creator
                            </h3>
                            <span class="text-[9px] font-mono text-zinc-400">Instant Orchestrator Binding</span>
                        </div>

                        <form id="studio-agent-form" onsubmit="createStudioAgent(event)" class="space-y-6">
                            <div id="studio-alert" class="hidden p-4 rounded-xl text-xs font-mono"></div>

                            <div class="grid grid-cols-1 sm:grid-cols-2 gap-4">
                                <div class="space-y-1.5">
                                    <label for="studio_agent_id" class="block text-[10px] font-bold text-zinc-400 uppercase font-mono">Agent Unique ID</label>
                                    <input type="text" id="studio_agent_id" required placeholder="e.g. market-analyst-agent" 
                                        class="w-full bg-zinc-950 border border-zinc-800 rounded-xl px-3.5 py-2.5 text-xs font-mono text-zinc-100 focus:outline-none focus:border-indigo-500 focus:ring-2 focus:ring-indigo-500/20" />
                                </div>
                                <div class="space-y-1.5">
                                    <label for="studio_description" class="block text-[10px] font-bold text-zinc-400 uppercase font-mono">Role / Capability Summary</label>
                                    <input type="text" id="studio_description" required placeholder="e.g. Evaluates market research and builds financial forecasts" 
                                        class="w-full bg-zinc-950 border border-zinc-800 rounded-xl px-3.5 py-2.5 text-xs font-mono text-zinc-100 focus:outline-none focus:border-indigo-500 focus:ring-2 focus:ring-indigo-500/20" />
                                </div>
                            </div>

                            <!-- System Prompt Textarea with Templates -->
                            <div class="space-y-2">
                                <div class="flex items-center justify-between">
                                    <label for="studio_system_prompt" class="block text-[10px] font-bold text-zinc-400 uppercase font-mono">System Prompt Instructions</label>
                                    <div class="flex gap-2">
                                        <button type="button" onclick="insertPromptTemplate('analyst')" class="text-[9px] font-mono text-indigo-400 hover:underline cursor-pointer">Insert Analyst Template</button>
                                        <button type="button" onclick="insertPromptTemplate('researcher')" class="text-[9px] font-mono text-indigo-400 hover:underline cursor-pointer">Insert Researcher Template</button>
                                    </div>
                                </div>
                                <textarea id="studio_system_prompt" rows="6" required 
                                    placeholder="You are a specialized agent responsible for processing..." 
                                    class="w-full bg-zinc-950 border border-zinc-800 rounded-xl px-4 py-3 text-xs font-mono text-zinc-100 focus:outline-none focus:border-indigo-500 focus:ring-2 focus:ring-indigo-500/20 resize-y shadow-inner"></textarea>
                            </div>

                            <!-- Tool Permissions Checklist -->
                            <div class="space-y-3">
                                <label class="block text-[10px] font-bold text-zinc-400 uppercase font-mono">Assign Tool Capabilities</label>
                                <div id="studio-tools-checklist" class="grid grid-cols-1 sm:grid-cols-2 gap-3 max-h-64 overflow-y-auto p-3 bg-zinc-950 rounded-xl border border-zinc-800/90 shadow-inner">
                                    <div class="text-xs text-zinc-500 italic">Loading framework tools...</div>
                                </div>
                            </div>

                            <button type="submit" class="w-full py-3.5 bg-gradient-to-r from-indigo-600 via-purple-600 to-indigo-500 hover:from-indigo-500 hover:to-purple-500 text-white rounded-xl text-xs font-bold uppercase tracking-wider transition-all shadow-lg shadow-indigo-500/20 active:scale-95 cursor-pointer flex items-center justify-center gap-2">
                                <span>✨ Register &amp; Deploy Agent</span>
                                <span>➔</span>
                            </button>
                        </form>
                    </section>
                </div>

                <!-- Right 1 Column: Blueprint Import/Export & Active Blueprints -->
                <div class="space-y-6 lg:col-span-1">
                    
                    <!-- Blueprint Import/Export Card -->
                    <section class="glass-card rounded-2xl p-6 shadow-2xl space-y-4">
                        <h3 class="text-xs font-extrabold uppercase tracking-widest text-zinc-100 border-b border-zinc-800 pb-3 flex items-center gap-2">
                            <span>📦</span> Blueprint Import &amp; Export
                        </h3>
                        <p class="text-xs text-zinc-400 leading-relaxed">Share agent blueprints across teams using standard .json configuration files.</p>
                        
                        <!-- Import Blueprint Form -->
                        <div class="p-4 bg-zinc-950 rounded-xl border border-zinc-800/90 space-y-3">
                            <span class="block text-[10px] font-bold text-zinc-300 uppercase font-mono">Import Blueprint File</span>
                            <input type="file" id="blueprint-file-input" accept=".json" class="hidden" onchange="importAgentBlueprint(event)" />
                            <label for="blueprint-file-input" class="w-full py-2.5 bg-zinc-900 hover:bg-zinc-800 border border-zinc-700 hover:border-zinc-500 text-zinc-200 rounded-lg text-xs font-bold uppercase tracking-wider transition-all cursor-pointer inline-flex items-center justify-center gap-2 text-center shadow-sm">
                                <span>📥 Choose .json Blueprint File</span>
                            </label>
                        </div>
                    </section>

                    <!-- Active Blueprints Registry -->
                    <section class="glass-card rounded-2xl p-6 shadow-2xl space-y-4">
                        <h3 class="text-xs font-extrabold uppercase tracking-widest text-zinc-100 border-b border-zinc-800 pb-3 flex items-center gap-2">
                            <span>⚡</span> Active Registered Blueprints
                        </h3>
                        <div id="studio-registered-agents-list" class="space-y-3 max-h-80 overflow-y-auto">
                            <!-- Dynamic cards populated via JS -->
                        </div>
                    </section>
                </div>
            </div>
        </main>
    </div>

    <!-- Audit Explorer Tab Content -->
    <div id="tab-content-audit" class="tab-pane hidden flex-1 flex flex-col">
        <main class="flex-1 max-w-7xl w-full mx-auto px-4 sm:px-6 lg:px-8 py-8 space-y-8">
            <!-- Header Banner -->
            <div class="glass-card rounded-2xl p-6 shadow-2xl flex flex-col md:flex-row items-start md:items-center justify-between gap-4 border-l-4 border-emerald-500">
                <div>
                    <h2 class="text-lg font-bold text-zinc-100 flex items-center gap-2">
                        <span>📊 Token Cost &amp; Execution Audit Explorer</span>
                        <span class="text-[10px] uppercase font-mono px-2 py-0.5 rounded-full bg-emerald-500/20 text-emerald-300 border border-emerald-500/30">SQLite Ledger</span>
                    </h2>
                    <p class="text-xs text-zinc-400 mt-1">Real-time token metrics, estimated USD LLM execution costs, latency breakdown, and historical audit reports.</p>
                </div>
                <div class="flex items-center gap-3">
                    <button onclick="exportAuditCSV()" class="px-4 py-2 bg-emerald-600 hover:bg-emerald-500 text-white rounded-xl text-xs font-bold transition-all shadow-lg flex items-center gap-2 cursor-pointer">
                        <span>📥 Export CSV Report</span>
                    </button>
                    <button onclick="pruneAuditRecords()" class="px-3 py-2 bg-zinc-800 hover:bg-rose-900/40 text-rose-300 border border-rose-800/40 rounded-xl text-xs font-medium transition-all cursor-pointer">
                        <span>🗑️ Prune (&gt;30 Days)</span>
                    </button>
                </div>
            </div>

            <!-- Top Metric Cards Grid -->
            <div class="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-5">
                <div class="glass-card rounded-xl p-5 border border-zinc-800/80 bg-zinc-900/50">
                    <div class="text-[10px] font-mono uppercase tracking-wider text-zinc-400">Total Spent (USD)</div>
                    <div id="audit-kpi-total-cost" class="text-2xl font-extrabold text-emerald-400 mt-1">$0.0000</div>
                    <div class="text-[9px] text-zinc-500 mt-1">Calculated via LLM node rates</div>
                </div>
                <div class="glass-card rounded-xl p-5 border border-zinc-800/80 bg-zinc-900/50">
                    <div class="text-[10px] font-mono uppercase tracking-wider text-zinc-400">Total Tokens Processed</div>
                    <div id="audit-kpi-total-tokens" class="text-2xl font-extrabold text-indigo-400 mt-1">0 Tokens</div>
                    <div class="text-[9px] text-zinc-500 mt-1">Prompt &amp; Completion tokens</div>
                </div>
                <div class="glass-card rounded-xl p-5 border border-zinc-800/80 bg-zinc-900/50">
                    <div class="text-[10px] font-mono uppercase tracking-wider text-zinc-400">Avg Execution Duration</div>
                    <div id="audit-kpi-avg-duration" class="text-2xl font-extrabold text-amber-400 mt-1">0 ms</div>
                    <div class="text-[9px] text-zinc-500 mt-1">Average task turn latency</div>
                </div>
                <div class="glass-card rounded-xl p-5 border border-zinc-800/80 bg-zinc-900/50">
                    <div class="text-[10px] font-mono uppercase tracking-wider text-zinc-400">Total Execution Turns</div>
                    <div id="audit-kpi-total-tasks" class="text-2xl font-extrabold text-sky-400 mt-1">0 Turns</div>
                    <div class="text-[9px] text-zinc-500 mt-1">Recorded audit ledger rows</div>
                </div>
            </div>

            <!-- Cost Breakdown Section -->
            <div class="grid grid-cols-1 lg:grid-cols-2 gap-8">
                <!-- Agent Cost Distribution -->
                <div class="glass-card rounded-2xl p-6 shadow-2xl space-y-4">
                    <h3 class="text-xs font-extrabold uppercase tracking-widest text-zinc-200 border-b border-zinc-800 pb-3 flex items-center justify-between">
                        <span>Cost Distribution by Agent</span>
                        <span class="text-[9px] text-zinc-400 font-mono">USD ($)</span>
                    </h3>
                    <div id="audit-agent-cost-bars" class="space-y-3 max-h-60 overflow-y-auto pr-1">
                        <div class="text-xs text-zinc-500 italic">No audit records recorded yet.</div>
                    </div>
                </div>

                <!-- Model Cost Distribution -->
                <div class="glass-card rounded-2xl p-6 shadow-2xl space-y-4">
                    <h3 class="text-xs font-extrabold uppercase tracking-widest text-zinc-200 border-b border-zinc-800 pb-3 flex items-center justify-between">
                        <span>Cost Distribution by Model Node</span>
                        <span class="text-[9px] text-zinc-400 font-mono">USD ($)</span>
                    </h3>
                    <div id="audit-model-cost-bars" class="space-y-3 max-h-60 overflow-y-auto pr-1">
                        <div class="text-xs text-zinc-500 italic">No audit records recorded yet.</div>
                    </div>
                </div>
            </div>

            <!-- Filterable Audit Table -->
            <div class="glass-card rounded-2xl p-6 shadow-2xl space-y-4">
                <div class="flex flex-col sm:flex-row items-start sm:items-center justify-between gap-4 border-b border-zinc-800 pb-4">
                    <h3 class="text-xs font-extrabold uppercase tracking-widest text-zinc-200">Execution Telemetry Ledger</h3>
                    <div class="flex items-center gap-3 w-full sm:w-auto">
                        <input type="text" id="audit-search-input" onkeyup="fetchAuditExplorerData()" placeholder="Filter by Agent, Model or Correlation ID..." class="bg-zinc-950 border border-zinc-800 rounded-xl px-3 py-1.5 text-xs text-zinc-200 focus:outline-none focus:border-emerald-500 w-full sm:w-64" />
                        <button onclick="fetchAuditExplorerData()" class="px-3 py-1.5 bg-zinc-800 hover:bg-zinc-700 text-zinc-200 rounded-xl text-xs font-semibold cursor-pointer">🔄 Refresh</button>
                    </div>
                </div>
                <div class="overflow-x-auto">
                    <table class="w-full text-left border-collapse">
                        <thead>
                            <tr class="border-b border-zinc-800/80 text-[10px] font-mono text-zinc-400 uppercase tracking-wider bg-zinc-950/40">
                                <th class="py-3 px-3">Timestamp</th>
                                <th class="py-3 px-3">Correlation ID</th>
                                <th class="py-3 px-3">Agent ID</th>
                                <th class="py-3 px-3">Model Node</th>
                                <th class="py-3 px-3">Prompt / Comp</th>
                                <th class="py-3 px-3">Est. Cost (USD)</th>
                                <th class="py-3 px-3">Duration</th>
                                <th class="py-3 px-3">Status</th>
                            </tr>
                        </thead>
                        <tbody id="audit-records-table-body" class="divide-y divide-zinc-800/40 text-xs">
                            <tr>
                                <td colspan="8" class="py-6 text-center text-zinc-500 italic">Loading audit telemetry records...</td>
                            </tr>
                        </tbody>
                    </table>
                </div>
            </div>
        </main>
    </div>

    <!-- Agent Network Topology Graph Tab Content -->
    <div id="tab-content-network" class="tab-pane hidden flex-1 flex flex-col">
        <main class="flex-1 max-w-7xl w-full mx-auto px-4 sm:px-6 lg:px-8 py-8 space-y-6 flex flex-col">
            <!-- Header Banner -->
            <div class="glass-card rounded-2xl p-6 shadow-2xl flex flex-col md:flex-row items-start md:items-center justify-between gap-4 border-l-4 border-indigo-500">
                <div>
                    <h2 class="text-lg font-bold text-zinc-100 flex items-center gap-2">
                        <span>📡 Mission Control Radar &amp; Network Topology</span>
                        <span class="text-[10px] uppercase font-mono px-2 py-0.5 rounded-full bg-emerald-500/20 text-emerald-300 border border-emerald-500/30 font-bold flex items-center gap-1">
                            <span class="w-1.5 h-1.5 rounded-full bg-emerald-400 animate-ping"></span> RADAR SWEEP ACTIVE
                        </span>
                    </h2>
                    <p class="text-xs text-zinc-400 mt-1 font-mono">Tactical agent radar telemetry, real-time azimuth bearings, and active signal firing pulses.</p>
                </div>
                <div class="flex items-center gap-3 font-mono">
                    <button onclick="resetNetworkLayout()" class="px-3.5 py-1.5 bg-zinc-800 hover:bg-zinc-700 text-zinc-200 rounded-xl text-xs font-bold transition-all cursor-pointer border border-zinc-700">🔄 Reset Radar Grid</button>
                    <button onclick="triggerSamplePulse()" class="px-3.5 py-1.5 bg-emerald-600 hover:bg-emerald-500 text-white rounded-xl text-xs font-bold transition-all shadow-lg shadow-emerald-600/20 cursor-pointer">📡 Fire Pulse</button>
                </div>
            </div>

            <!-- Tactical Mission Control Legend & Status Bar -->
            <div class="flex flex-wrap items-center justify-between gap-4 bg-zinc-950/80 p-3.5 rounded-xl border border-emerald-500/20 font-mono shadow-inner">
                <div class="flex items-center gap-4 flex-wrap text-xs">
                    <span class="text-emerald-400 font-bold uppercase text-[10px] tracking-wider">HUD RETICLE:</span>
                    <span class="inline-flex items-center gap-1.5"><span class="w-3 h-3 rounded-full bg-amber-500 shadow-sm shadow-amber-500/50"></span> <span class="text-zinc-300">User Client</span></span>
                    <span class="inline-flex items-center gap-1.5"><span class="w-3 h-3 rounded-full bg-indigo-500 shadow-sm shadow-indigo-500/50"></span> <span class="text-zinc-300">Orchestrator</span></span>
                    <span class="inline-flex items-center gap-1.5"><span class="w-3 h-3 rounded-full bg-emerald-500 shadow-sm shadow-emerald-500/50"></span> <span class="text-zinc-300">Specialist Node</span></span>
                    <span class="inline-flex items-center gap-1.5"><span class="w-2.5 h-2.5 rounded-full bg-cyan-400 animate-ping"></span> <span class="text-cyan-300 font-bold">SSE Signal Pulse</span></span>
                </div>
                <div class="text-[10px] text-emerald-400/90 font-bold tracking-widest flex items-center gap-2">
                    <span class="w-2 h-2 rounded-full bg-emerald-400 animate-pulse"></span>
                    <span>FREQUENCY: 2.4GHZ // ALL SYSTEMS NOMINAL</span>
                </div>
            </div>

            <!-- Canvas Container & Side Inspector Grid -->
            <div class="grid grid-cols-1 lg:grid-cols-4 gap-6 flex-1 min-h-[550px]">
                <!-- Interactive Canvas (3 Cols) -->
                <div class="lg:col-span-3 glass-card rounded-2xl p-2 relative overflow-hidden border border-zinc-800 flex flex-col h-[540px]">
                    <canvas id="network-canvas" class="w-full h-[520px] rounded-xl bg-zinc-950/80 cursor-grab active:cursor-grabbing block" style="width: 100%; height: 520px;"></canvas>
                </div>

                <!-- Node Inspector Drawer (1 Col) -->
                <div class="lg:col-span-1 glass-card rounded-2xl p-5 border border-zinc-800 flex flex-col space-y-4">
                    <h3 class="text-xs font-extrabold uppercase tracking-widest text-zinc-200 border-b border-zinc-800 pb-3 flex items-center justify-between">
                        <span>Node Inspector</span>
                        <span id="inspector-badge" class="text-[9px] font-mono px-2 py-0.5 rounded bg-zinc-800 text-zinc-400">SELECT NODE</span>
                    </h3>
                    <div id="inspector-content" class="space-y-4 text-xs font-mono text-zinc-300 flex-1 overflow-y-auto">
                        <div class="text-center text-zinc-500 italic py-12">Click any agent or tool node in the network to inspect properties and recent activity.</div>
                    </div>
                </div>
            </div>
        </main>
    </div>

    <!-- Schedules Tab Content -->
    <div id="tab-content-scheduler" class="tab-pane hidden flex-1 flex flex-col">
        <main class="flex-1 max-w-7xl w-full mx-auto px-4 sm:px-6 lg:px-8 py-8 grid grid-cols-1 lg:grid-cols-3 gap-8">
            <!-- Left Column: Add Schedule Form (Taking 1/3 width) -->
            <div class="lg:col-span-1 space-y-8">
                <section class="glass-card rounded-2xl p-6 shadow-2xl">
                    <h2 class="text-xs font-extrabold uppercase tracking-widest text-zinc-100 mb-4 border-b border-zinc-800 pb-3">Create Cron Schedule</h2>
                    <form id="schedule-form" hx-post="/api/schedules" hx-target="#schedules-list" hx-swap="innerHTML" class="space-y-4">
                        <div class="space-y-1.5">
                            <label for="sched_name" class="block text-[10px] font-bold text-zinc-400 uppercase font-mono">Schedule Name</label>
                            <input type="text" name="name" id="sched_name" required placeholder="e.g. Daily Market Briefing" class="w-full bg-zinc-950 border border-zinc-800 rounded-lg px-3 py-2 text-xs focus:outline-none focus:border-indigo-500 focus:ring-2 focus:ring-indigo-500/20 text-zinc-200" />
                        </div>
                        <div class="space-y-1.5">
                            <label for="sched_cron" class="block text-[10px] font-bold text-zinc-400 uppercase font-mono">Cron Expression (5-field)</label>
                            <input type="text" name="cron" id="sched_cron" required placeholder="e.g. 0 9 * * 1-5" class="w-full bg-zinc-950 border border-zinc-800 rounded-lg px-3 py-2 text-xs font-mono focus:outline-none focus:border-indigo-500 focus:ring-2 focus:ring-indigo-500/20 text-zinc-200" />
                            <div class="text-[9px] text-zinc-500 font-mono">Min Hour DOM Month DOW</div>
                        </div>
                        <div class="space-y-1.5">
                            <label for="sched_task" class="block text-[10px] font-bold text-zinc-400 uppercase font-mono">Goal/Task Prompt</label>
                            <textarea name="task" id="sched_task" rows="4" required placeholder="e.g. Research options volatility surfaces and compile a report" class="w-full bg-zinc-950 border border-zinc-800 rounded-lg px-3 py-2 text-xs focus:outline-none focus:border-indigo-500 focus:ring-2 focus:ring-indigo-500/20 text-zinc-200 resize-none"></textarea>
                        </div>
                        <button type="submit" class="w-full py-3 bg-gradient-to-r from-indigo-600 to-purple-600 hover:from-indigo-500 hover:to-purple-500 text-white rounded-xl text-xs font-bold uppercase tracking-wider transition-all active:scale-95 shadow-md cursor-pointer flex items-center justify-center">
                            Add Cron Schedule
                        </button>
                    </form>
                </section>
            </div>

            <!-- Right Column: Active Schedules (Taking 2/3 width) -->
            <div class="lg:col-span-2 space-y-8">
                <section class="glass-card rounded-2xl p-6 shadow-2xl flex flex-col h-[560px]">
                    <div class="flex items-center justify-between mb-4 border-b border-zinc-800 pb-3">
                        <h2 class="text-xs font-extrabold uppercase tracking-widest text-zinc-200">Active Cron Schedules</h2>
                        <span class="text-[9px] font-mono bg-zinc-900 border border-zinc-800 text-zinc-400 px-2 py-0.5 rounded">Persistent SQLite</span>
                    </div>
                    <div id="schedules-list" class="flex-1 overflow-y-auto space-y-4">
                        {{ template "schedules-list-tmpl" .Schedules }}
                    </div>
                </section>
            </div>
        </main>
    </div>

    <!-- Footer -->
    <footer class="border-t border-zinc-800/80 bg-zinc-950/80 py-4 text-center text-[10px] text-zinc-500 font-mono uppercase tracking-widest mt-auto">
        Go Engine // HTMX SSE Telemetry // Ephemeral WASM &amp; Docker Runtime
    </footer>

    <!-- HTMX Local -->
    <script src="/static/htmx.min.js"></script>
    <!-- HTMX SSE Extension Local -->
    <script src="/static/sse.js"></script>

    <!-- Security (C-3): CSRF Token injection for all HTMX mutating requests.
         Reads the _csrf_token cookie (set by the server on page load) and
         attaches it as an X-CSRF-Token header on every state-mutating request. -->
    <script>
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
    </script>

    <!-- Auto-scroll, form reset, and tab navigation helpers -->
    <script>
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

        document.addEventListener('DOMContentLoaded', function() {
            const saved = localStorage.getItem('theme_preference');
            if (saved === 'light') {
                document.body.classList.add('light-theme');
                document.documentElement.classList.remove('dark');
                document.documentElement.classList.add('light-theme');
                updateThemeUI(true);
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
        document.addEventListener('htmx:sseBeforeMessage', function(e) {
            setTimeout(function() {
                var consoleDiv = document.getElementById('console-logs');
                if (consoleDiv) {
                    consoleDiv.scrollTop = consoleDiv.scrollHeight;
                }
            }, 50);
        });

        document.addEventListener('htmx:afterSwap', function(e) {
            if (e.target && (e.target.id === 'console-logs' || (e.target.classList && e.target.classList.contains('log-entry')))) {
                const logs = e.target.querySelectorAll ? e.target.querySelectorAll('.log-entry') : [e.target];
                if (logs.length > 0) {
                    const lastLog = logs[logs.length - 1];
                    const sender = lastLog.getAttribute('data-sender') || '';
                    const recipient = lastLog.getAttribute('data-recipient') || '';
                    if (typeof emitLiveNetworkPulse === 'function') {
                        emitLiveNetworkPulse(sender, recipient);
                    }
                }
            }
        });

        window.currentLogFilter = 'all';

        window.setLogFilter = function(filter) {
            window.currentLogFilter = filter;

            const buttons = ['all', 'user', 'orchestration', 'specialists'];
            buttons.forEach(b => {
                const btn = document.getElementById('filter-btn-' + b);
                if (btn) {
                    if (b === filter) {
                        btn.className = "px-3 py-1 text-[9px] font-mono font-bold uppercase tracking-wider bg-indigo-600 text-white rounded-md cursor-pointer transition-all active:scale-95 shadow-sm";
                    } else {
                        btn.className = "px-3 py-1 text-[9px] font-mono font-bold uppercase tracking-wider bg-zinc-900 border border-zinc-800 text-zinc-400 hover:text-zinc-200 rounded-md cursor-pointer transition-all active:scale-95";
                    }
                }
            });

            const entries = document.querySelectorAll('.log-entry');
            entries.forEach(entry => {
                window.applyFilterToElement(entry);
            });

            var consoleDiv = document.getElementById('console-logs');
            if (consoleDiv) {
                consoleDiv.scrollTop = consoleDiv.scrollHeight;
            }
        };

        window.applyFilterToElement = function(el) {
            const sender = el.getAttribute('data-sender') || '';
            const recipient = el.getAttribute('data-recipient') || '';
            const text = el.textContent || '';
            
            const isOrchestration = ['triage-agent', 'planner-agent', 'supervisor-agent'].includes(sender) || 
                                    ['triage-agent', 'planner-agent', 'supervisor-agent'].includes(recipient);
                                    
            const isUserDirect = (sender === 'USER' || recipient === 'USER');
            const isSupervisorProgress = text.includes('[Supervisor Check]') || text.includes('[Supervisor Escalation]');

            let show = false;
            if (window.currentLogFilter === 'all') {
                show = true;
            } else if (window.currentLogFilter === 'user') {
                show = isUserDirect && !isSupervisorProgress;
            } else if (window.currentLogFilter === 'orchestration') {
                show = isOrchestration || isSupervisorProgress;
            } else if (window.currentLogFilter === 'specialists') {
                show = !isOrchestration && !isUserDirect && !isSupervisorProgress;
            }

            el.style.display = show ? 'flex' : 'none';
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
                if (['dashboard', 'studio', 'audit', 'config', 'scheduler'].includes(hash)) {
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

                // --- MISSION CONTROL RADAR HUD: Concentric Sonar Rings ---
                [0.25, 0.5, 0.75, 1.0].forEach((ratio, idx) => {
                    const r = maxRadius * ratio;
                    ctx.beginPath();
                    ctx.arc(cx, cy, r, 0, Math.PI * 2);
                    ctx.strokeStyle = isLight ? 'rgba(79, 70, 229, 0.15)' : 'rgba(16, 185, 129, 0.18)';
                    ctx.lineWidth = 1;
                    ctx.setLineDash([3, 4]);
                    ctx.stroke();
                    ctx.setLineDash([]);

                    ctx.font = '9px "JetBrains Mono", monospace';
                    ctx.fillStyle = isLight ? 'rgba(79, 70, 229, 0.45)' : 'rgba(16, 185, 129, 0.45)';
                    ctx.textAlign = 'center';
                    ctx.fillText((ratio * 200).toFixed(0) + 'KM', cx + r - 14, cy - 3);
                });

                // --- MISSION CONTROL RADAR HUD: Azimuth Axes & Crosshairs ---
                ctx.beginPath();
                ctx.moveTo(cx - maxRadius - 12, cy); ctx.lineTo(cx + maxRadius + 12, cy);
                ctx.moveTo(cx, cy - maxRadius - 12); ctx.lineTo(cx, cy + maxRadius + 12);
                ctx.strokeStyle = isLight ? 'rgba(79, 70, 229, 0.25)' : 'rgba(16, 185, 129, 0.28)';
                ctx.setLineDash([2, 3]);
                ctx.stroke();
                ctx.setLineDash([]);

                // Diagonal 45° Radial Guides
                [Math.PI / 4, 3 * Math.PI / 4, 5 * Math.PI / 4, 7 * Math.PI / 4].forEach(ang => {
                    ctx.beginPath();
                    ctx.moveTo(cx, cy);
                    ctx.lineTo(cx + Math.cos(ang) * maxRadius, cy + Math.sin(ang) * maxRadius);
                    ctx.strokeStyle = isLight ? 'rgba(79, 70, 229, 0.1)' : 'rgba(16, 185, 129, 0.12)';
                    ctx.setLineDash([1, 4]);
                    ctx.stroke();
                    ctx.setLineDash([]);
                });

                // Compass Azimuth Labels
                ctx.font = '10px "JetBrains Mono", monospace';
                ctx.fillStyle = isLight ? '#4f46e5' : '#10b981';
                ctx.textAlign = 'center';
                ctx.fillText('N 000°', cx, cy - maxRadius - 14);
                ctx.fillText('S 180°', cx, cy + maxRadius + 18);
                ctx.textAlign = 'left';
                ctx.fillText('E 090°', cx + maxRadius + 16, cy + 3);
                ctx.textAlign = 'right';
                ctx.fillText('W 270°', cx - maxRadius - 16, cy + 3);

                // --- MISSION CONTROL RADAR HUD: Rotating Radar Sweep Scanner Beam ---
                window.radarSweepAngle = (window.radarSweepAngle + 0.012) % (Math.PI * 2);
                const currentAngle = window.radarSweepAngle;

                // Sweep Trail (Conical Sector)
                ctx.save();
                const sweepSteps = 30;
                for (let i = 0; i < sweepSteps; i++) {
                    const alpha = (1 - i / sweepSteps) * 0.15;
                    const a1 = currentAngle - (i * 0.015);
                    const a2 = currentAngle - ((i + 1) * 0.015);

                    ctx.beginPath();
                    ctx.moveTo(cx, cy);
                    ctx.arc(cx, cy, maxRadius, a2, a1);
                    ctx.fillStyle = isLight ? 'rgba(99, 102, 241, ' + alpha + ')' : 'rgba(16, 185, 129, ' + alpha + ')';
                    ctx.fill();
                }

                // Leading Edge Line of Radar Sweep
                ctx.beginPath();
                ctx.moveTo(cx, cy);
                ctx.lineTo(cx + Math.cos(currentAngle) * maxRadius, cy + Math.sin(currentAngle) * maxRadius);
                ctx.strokeStyle = isLight ? '#4f46e5' : '#10b981';
                ctx.lineWidth = 2;
                ctx.shadowColor = isLight ? '#6366f1' : '#34d399';
                ctx.shadowBlur = 10;
                ctx.stroke();
                ctx.shadowBlur = 0;
                ctx.restore();

                // --- MISSION CONTROL RADAR HUD: Corner Tactical Reticles ---
                const hudM = 14;
                const hudL = 18;
                ctx.strokeStyle = isLight ? 'rgba(79, 70, 229, 0.45)' : 'rgba(16, 185, 129, 0.5)';
                ctx.lineWidth = 2;

                ctx.beginPath(); ctx.moveTo(hudM, hudM + hudL); ctx.lineTo(hudM, hudM); ctx.lineTo(hudM + hudL, hudM); ctx.stroke();
                ctx.beginPath(); ctx.moveTo(width - hudM - hudL, hudM); ctx.lineTo(width - hudM, hudM); ctx.lineTo(width - hudM, hudM + hudL); ctx.stroke();
                ctx.beginPath(); ctx.moveTo(hudM, height - hudM - hudL); ctx.lineTo(hudM, height - hudM); ctx.lineTo(hudM + hudL, height - hudM); ctx.stroke();
                ctx.beginPath(); ctx.moveTo(width - hudM - hudL, height - hudM); ctx.lineTo(width - hudM, height - hudM); ctx.lineTo(width - hudM, height - hudM - hudL); ctx.stroke();

                // Telemetry Header Info Overlay
                const degVal = Math.floor((currentAngle * 180 / Math.PI) % 360);
                const degStr = (degVal < 100 ? (degVal < 10 ? '00' : '0') : '') + degVal;
                ctx.font = '10px "JetBrains Mono", monospace';
                ctx.fillStyle = isLight ? '#4f46e5' : '#10b981';
                ctx.textAlign = 'left';
                ctx.fillText('📡 RADAR TELEMETRY // SWEEP: ' + degStr + '°', hudM + 10, hudM + 18);
                ctx.fillStyle = isLight ? '#64748b' : '#71717a';
                ctx.fillText('TARGETS: ' + networkNodes.length + ' ACTIVE AGENTS | ORBIT RAD: ' + Math.floor(maxRadius) + 'PX', hudM + 10, hudM + 32);

                // --- RENDER INTERACTION EDGES ---
                networkEdges.forEach(e => {
                    if (!e.source || !e.target) return;
                    if (isNaN(e.source.x) || isNaN(e.source.y) || isNaN(e.target.x) || isNaN(e.target.y)) return;
                    ctx.beginPath();
                    ctx.moveTo(e.source.x, e.source.y);
                    ctx.lineTo(e.target.x, e.target.y);
                    ctx.strokeStyle = isLight ? 'rgba(199, 210, 254, 0.8)' : 'rgba(52, 211, 153, 0.25)';
                    ctx.lineWidth = Math.min(3, 1 + e.weight * 0.2);
                    ctx.setLineDash([4, 2]);
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
                    ctx.arc(px, py, 6, 0, Math.PI * 2);
                    ctx.fillStyle = p.color || '#38bdf8';
                    ctx.shadowColor = p.color || '#38bdf8';
                    ctx.shadowBlur = 14;
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

        let currentArtifactRawText = "";

        function openArtifactViewer(path, filename, artifactType, agentName) {
            path = (path || '').replace(/^file:\/\//, '').replace(/^file:/, '').replace(/[\x60"'\s.,;:]+$/g, '').replace(/^[\x60"'\s]+/g, '');
            const modal = document.getElementById('artifact-viewer-modal');
            const titleEl = document.getElementById('artifact-modal-title');
            const badgeEl = document.getElementById('artifact-modal-badge');
            const agentEl = document.getElementById('artifact-modal-agent');
            const downloadBtn = document.getElementById('artifact-modal-download-btn');
            const copyBtn = document.getElementById('artifact-modal-copy-btn');
            const iconEl = document.getElementById('artifact-modal-icon');

            const textEl = document.getElementById('artifact-modal-text-content');
            const pdfIframe = document.getElementById('artifact-modal-pdf-iframe');
            const imgWrapper = document.getElementById('artifact-modal-image-wrapper');
            const imgEl = document.getElementById('artifact-modal-image');
            const loadingEl = document.getElementById('artifact-modal-loading');

            titleEl.textContent = filename || 'Artifact';
            agentEl.textContent = 'produced by ' + (agentName || 'agent');
            badgeEl.textContent = (artifactType || 'file').toUpperCase();
            downloadBtn.href = '/api/artifact?path=' + encodeURIComponent(path);

            textEl.classList.add('hidden');
            pdfIframe.classList.add('hidden');
            imgWrapper.classList.add('hidden');
            loadingEl.classList.remove('hidden');
            copyBtn.classList.add('hidden');
            modal.classList.remove('hidden');
            currentArtifactRawText = "";

            if (artifactType === 'pdf') {
                iconEl.textContent = '📕';
                pdfIframe.src = '/api/artifact?path=' + encodeURIComponent(path);
                pdfIframe.classList.remove('hidden');
                loadingEl.classList.add('hidden');
            } else if (artifactType === 'image') {
                iconEl.textContent = '🖼️';
                imgEl.src = '/api/artifact?path=' + encodeURIComponent(path);
                imgWrapper.classList.remove('hidden');
                loadingEl.classList.add('hidden');
            } else {
                iconEl.textContent = artifactType === 'code' ? '💻' : (artifactType === 'json' ? '⚙️' : '📄');
                copyBtn.classList.remove('hidden');
                fetch('/api/artifact?path=' + encodeURIComponent(path))
                    .then(res => res.text())
                    .then(text => {
                        currentArtifactRawText = text;
                        textEl.textContent = text;
                        loadingEl.classList.add('hidden');
                        textEl.classList.remove('hidden');
                    })
                    .catch(err => {
                        loadingEl.classList.add('hidden');
                        textEl.textContent = "Error loading artifact content: " + err;
                        textEl.classList.remove('hidden');
                    });
            }
        }

        function closeArtifactViewer() {
            document.getElementById('artifact-viewer-modal').classList.add('hidden');
            document.getElementById('artifact-modal-pdf-iframe').src = 'about:blank';
        }

        function copyArtifactToClipboard() {
            if (currentArtifactRawText) {
                navigator.clipboard.writeText(currentArtifactRawText).then(() => {
                    const copyBtn = document.getElementById('artifact-modal-copy-btn');
                    const orig = copyBtn.innerHTML;
                    copyBtn.innerHTML = '<span>✅</span> <span>Copied!</span>';
                    setTimeout(() => copyBtn.innerHTML = orig, 2000);
                });
            }
        }

        document.addEventListener('keydown', function(e) {
            if (e.key === 'Escape') {
                closeArtifactViewer();
            }
        });

        document.addEventListener('click', function(e) {
            const link = e.target.closest('a');
            if (link) {
                const href = link.getAttribute('href') || '';
                if (href.startsWith('file:') || href.startsWith('file://')) {
                    e.preventDefault();
                    const cleanPath = href.replace(/^file:\/\//, '').replace(/^file:/, '').replace(/[.,;:\s]+$/, '');
                    const baseName = cleanPath.split('/').pop().split('?')[0];
                    let type = 'file';
                    const lower = baseName.toLowerCase();
                    if (lower.endsWith('.pdf')) type = 'pdf';
                    else if (lower.endsWith('.png') || lower.endsWith('.jpg') || lower.endsWith('.jpeg') || lower.endsWith('.webp')) type = 'image';
                    else if (lower.endsWith('.md') || lower.endsWith('.txt')) type = 'doc';
                    else if (lower.endsWith('.json')) type = 'json';
                    else if (lower.endsWith('.py') || lower.endsWith('.sh') || lower.endsWith('.go')) type = 'code';

                    openArtifactViewer(cleanPath, baseName, type, 'agent');
                }
            }
        });
    </script>

    <!-- Artifact Viewer Modal Overlay -->
    <div id="artifact-viewer-modal" class="fixed inset-0 z-50 flex items-center justify-center bg-black/80 backdrop-blur-md hidden p-4 sm:p-6 transition-all duration-300">
        <div class="bg-zinc-900 border border-zinc-800 rounded-2xl shadow-2xl w-full max-w-5xl max-h-[90vh] flex flex-col overflow-hidden animate-in fade-in zoom-in-95">
            <!-- Modal Header -->
            <div class="flex items-center justify-between px-6 py-4 bg-zinc-950 text-white border-b border-zinc-800">
                <div class="flex items-center gap-3 overflow-hidden">
                    <span id="artifact-modal-icon" class="text-xl">📄</span>
                    <div class="flex flex-col truncate">
                        <div class="flex items-center gap-2">
                            <span id="artifact-modal-title" class="font-mono text-sm font-bold text-white truncate">artifact</span>
                            <span id="artifact-modal-badge" class="text-[9px] font-mono font-bold bg-zinc-800 text-indigo-400 px-2.5 py-0.5 rounded border border-zinc-700 uppercase">FILE</span>
                        </div>
                        <span id="artifact-modal-agent" class="text-[10px] text-zinc-400 font-mono">produced by agent</span>
                    </div>
                </div>
                <div class="flex items-center gap-3">
                    <button id="artifact-modal-copy-btn" onclick="copyArtifactToClipboard()" class="inline-flex items-center gap-1.5 px-3 py-1.5 bg-zinc-800 hover:bg-zinc-700 text-zinc-200 rounded-lg text-[10px] font-mono font-bold uppercase transition-all cursor-pointer border border-zinc-700">
                        <span>📋</span> <span>Copy</span>
                    </button>
                    <a id="artifact-modal-download-btn" href="#" target="_blank" class="inline-flex items-center gap-1.5 px-3 py-1.5 bg-indigo-600 hover:bg-indigo-500 text-white rounded-lg text-[10px] font-mono font-bold uppercase transition-all cursor-pointer shadow-sm">
                        <span>↗️</span> <span>Download / Open</span>
                    </a>
                    <button onclick="closeArtifactViewer()" class="p-1.5 text-zinc-400 hover:text-white rounded-lg hover:bg-zinc-800 transition-all cursor-pointer text-lg font-bold">
                        ✕
                    </button>
                </div>
            </div>

            <!-- Modal Content Body -->
            <div class="flex-1 overflow-auto bg-zinc-950 p-6 text-zinc-100 font-mono text-xs leading-relaxed" id="artifact-modal-content-wrapper">
                <!-- Text / Code Viewer -->
                <pre id="artifact-modal-text-content" class="whitespace-pre-wrap break-all hidden selection:bg-indigo-500 selection:text-white"></pre>

                <!-- PDF Viewer Iframe -->
                <iframe id="artifact-modal-pdf-iframe" class="w-full h-[65vh] border-0 rounded-xl hidden bg-white"></iframe>

                <!-- Image Viewer -->
                <div id="artifact-modal-image-wrapper" class="flex justify-center items-center min-h-[400px] hidden bg-zinc-900/50 p-4 rounded-xl border border-zinc-800">
                    <img id="artifact-modal-image" src="" class="max-h-[65vh] max-w-full object-contain rounded-lg shadow-2xl" alt="Artifact Preview" />
                </div>

                <!-- Loading Spinner -->
                <div id="artifact-modal-loading" class="flex flex-col items-center justify-center py-20 gap-3 text-zinc-400 font-mono">
                    <div class="w-8 h-8 border-2 border-zinc-700 border-t-indigo-500 rounded-full animate-spin"></div>
                    <span>Loading artifact content...</span>
                </div>
            </div>
        </div>
    </div>
</body>
</html>
`))

// PendingApprovalTemplate renders a pending approval card with glowing amber left border.
var PendingApprovalTemplate = template.Must(template.New("approval").Parse(`
<div id="approval-{{.CorrelationID}}" class="p-5 bg-zinc-900/90 border border-zinc-800 border-l-4 border-l-amber-500 rounded-xl flex flex-col gap-4 shadow-xl backdrop-blur-md">
    <div class="flex justify-between items-start">
        <div class="flex flex-col">
            <span class="text-[9px] text-amber-400 font-bold font-mono uppercase tracking-widest flex items-center gap-1.5">
                <span class="w-2 h-2 rounded-full bg-amber-400 animate-ping"></span> Approval Requested (HITL)
            </span>
            <span class="font-bold text-zinc-100 text-xs mt-0.5 font-mono">{{.Sender}} ──▶ {{.Metadata.action}}</span>
        </div>
        <span class="text-[9px] font-mono bg-amber-500/10 text-amber-300 border border-amber-500/20 px-2.5 py-0.5 rounded-full uppercase font-bold">
            {{.Metadata.risk_level}} RISK
        </span>
    </div>
    
    <div id="approval-content-{{.CorrelationID}}" class="approval-content text-xs text-zinc-300 font-mono bg-zinc-950 p-4 rounded-xl border border-zinc-800/90 leading-relaxed shadow-inner overflow-x-auto max-h-[360px]">
        {{.Content}}
    </div>

    <!-- Approval form using htmx POST, replacing the parent card with response HTML -->
    <form hx-post="/api/hitl/respond" hx-target="#approval-{{.CorrelationID}}" hx-swap="outerHTML" class="flex gap-3 justify-end">
        <input type="hidden" name="correlation_id" value="{{.CorrelationID}}">
        <button type="submit" name="decision" value="REJECTED" class="px-4 py-2 bg-zinc-900 hover:bg-rose-950/60 text-zinc-400 hover:text-rose-300 border border-zinc-700 hover:border-rose-800 rounded-lg text-[10px] font-bold uppercase tracking-wider transition-all active:scale-95 cursor-pointer">
            Deny Execution
        </button>
        <button type="submit" name="decision" value="APPROVED" class="px-4 py-2 bg-gradient-to-r from-emerald-600 to-teal-600 hover:from-emerald-500 hover:to-teal-500 text-white rounded-lg text-[10px] font-bold uppercase tracking-wider transition-all shadow-md shadow-emerald-500/20 active:scale-95 cursor-pointer">
            Approve &amp; Continue
        </button>
    </form>
</div>
<script>(function(){
  var el = document.getElementById("approval-content-{{.CorrelationID}}");
  if (!el) return;
  var raw = {{.RawJSONContent}};
  var action = {{.RawJSONAction}};
  var lowerAction = (action || "").toLowerCase();
  var lowerRaw = (raw || "").toLowerCase();

  var isPython = lowerAction.includes("python") || lowerAction.includes("docker") || lowerAction.includes("code") ||
                 lowerAction.includes("script") || lowerRaw.includes("import ") || lowerRaw.includes("def ") ||
                 lowerRaw.includes("print(") || lowerRaw.includes("return ");

  var fence = "\x60\x60\x60";
  var formattedMd = raw;
  if (isPython && !raw.trim().startsWith(fence)) {
      formattedMd = fence + "python\n" + raw.trim() + "\n" + fence;
  }

  if (typeof marked !== "undefined") {
      el.innerHTML = marked.parse(formattedMd, { breaks: true, gfm: true });
  } else {
      el.textContent = raw;
  }

  if (isPython) {
      el.querySelectorAll("pre code").forEach(function(block) {
          var codeText = block.innerHTML;
          codeText = codeText
              .replace(/(#(.*)$)/gm, '<span class="text-zinc-500 italic">$1</span>')
              .replace(/(".*?"|'.*?')/g, '<span class="text-amber-300">$1</span>')
              .replace(/\b(def|class|import|from|return|if|elif|else|while|for|in|try|except|finally|with|as|pass|break|continue|lambda|yield|raise|async|await|and|or|not|is|True|False|None)\b/g, '<span class="text-purple-400 font-bold">$1</span>')
              .replace(/\b(print|len|range|enumerate|zip|dict|list|set|tuple|int|str|float|bool|open|map|filter)\b(?=\()/g, '<span class="text-cyan-400 font-bold">$1</span>');
          block.innerHTML = codeText;
      });
  }
})();</script>
`))

// LogSnippetTemplate renders a log entry with marked.js markdown rendering and role-colored badges.
var LogSnippetTemplate = template.Must(template.New("log").Parse(`
<div class="log-entry py-2.5 px-3 rounded-lg border border-zinc-900 hover:border-zinc-800 hover:bg-zinc-900/40 flex gap-3 items-start text-zinc-300 transition-all group" data-sender="{{.Sender}}" data-recipient="{{.Recipient}}">
    <span class="text-zinc-500 shrink-0 select-none font-mono text-[9px] pt-0.5 whitespace-nowrap">[{{.Time}}]</span>
    <div class="flex items-center gap-1.5 shrink-0 text-[10px] font-mono">
        <span class="sender-badge px-2 py-0.5 rounded font-bold uppercase shadow-sm bg-zinc-800 text-zinc-200 border border-zinc-700" data-sender="{{.Sender}}">{{.Sender}}</span>
        <span class="text-zinc-400">➔</span>
        <span class="recipient-badge px-2 py-0.5 rounded font-bold uppercase shadow-sm bg-zinc-900 text-zinc-400 border border-zinc-800" data-recipient="{{.Recipient}}">{{.Recipient}}</span>
    </div>
    <div id="md-{{.Time}}-{{.Sender}}" class="md-content text-zinc-200 flex-1 min-w-0 text-[11px] leading-relaxed"></div>
</div>
<script>(function(){
  var el=document.getElementById("md-{{.Time}}-{{.Sender}}");
  if(!el)return;
  var raw={{.RawJSON}};
  if(typeof marked!=="undefined"){el.innerHTML=marked.parse(raw,{breaks:true,gfm:true});}
  else{el.textContent=raw;}
  
  var entry = el.closest(".log-entry");
  if(entry && typeof window.applyFilterToElement === "function") {
    window.applyFilterToElement(entry);
  }
  
  if (typeof window.emitLiveNetworkPulse === "function") {
    window.emitLiveNetworkPulse("{{.Sender}}", "{{.Recipient}}");
  }

  var log=document.getElementById("console-logs");
  if(log)log.scrollTop=log.scrollHeight;
})();</script>
`))

// ActionTakenTemplate renders the state of the card after decision is submitted.
var ActionTakenTemplate = template.Must(template.New("action").Parse(`
<div class="p-4 bg-zinc-950 border border-zinc-800 text-indigo-400 rounded-xl text-[10px] uppercase font-mono tracking-wider text-center flex items-center justify-center gap-2 shadow-inner">
    <span class="w-2 h-2 rounded-full bg-indigo-500"></span>
    <span>RESOLVED // OPERATOR DECISION: {{.Decision}}</span>
</div>
`))

// ArtifactCardTemplate renders a downloadable artifact card pushed via SSE.
var ArtifactCardTemplate = template.Must(template.New("artifact").Parse(`
<div class="p-4 bg-zinc-900/80 border border-zinc-800 rounded-xl flex flex-col gap-3 shadow-lg glass-card-hover" style="animation: fadeSlideIn 0.3s cubic-bezier(0.4, 0, 0.2, 1);">
    <div class="flex items-start justify-between gap-2">
        <div class="flex flex-col gap-1">
            <div class="flex items-center gap-2 flex-wrap">
                {{if eq .ArtifactType "pdf"}}<span class="text-[9px] font-bold font-mono bg-rose-500/10 text-rose-400 border border-rose-500/20 px-2 py-0.5 rounded uppercase">PDF Report</span>{{end}}
                {{if eq .ArtifactType "email"}}<span class="text-[9px] font-bold font-mono bg-amber-500/10 text-amber-400 border border-amber-500/20 px-2 py-0.5 rounded uppercase">Email Draft</span>{{end}}
                {{if eq .ArtifactType "excel"}}<span class="text-[9px] font-bold font-mono bg-emerald-500/10 text-emerald-400 border border-emerald-500/20 px-2 py-0.5 rounded uppercase">Workbook</span>{{end}}
                {{if eq .ArtifactType "code"}}<span class="text-[9px] font-bold font-mono bg-purple-500/10 text-purple-400 border border-purple-500/20 px-2 py-0.5 rounded uppercase">Code Script</span>{{end}}
                {{if eq .ArtifactType "json"}}<span class="text-[9px] font-bold font-mono bg-cyan-500/10 text-cyan-400 border border-cyan-500/20 px-2 py-0.5 rounded uppercase">JSON Data</span>{{end}}
                {{if eq .ArtifactType "doc"}}<span class="text-[9px] font-bold font-mono bg-indigo-500/10 text-indigo-400 border border-indigo-500/20 px-2 py-0.5 rounded uppercase">Markdown</span>{{end}}
                {{if eq .ArtifactType "image"}}<span class="text-[9px] font-bold font-mono bg-teal-500/10 text-teal-400 border border-teal-500/20 px-2 py-0.5 rounded uppercase">Image</span>{{end}}
                {{if eq .ArtifactType "text"}}<span class="text-[9px] font-bold font-mono bg-zinc-800 text-zinc-300 border border-zinc-700 px-2 py-0.5 rounded uppercase">Text File</span>{{end}}
                {{if eq .ArtifactType "file"}}<span class="text-[9px] font-bold font-mono bg-zinc-800 text-zinc-300 border border-zinc-700 px-2 py-0.5 rounded uppercase">File</span>{{end}}
                <span class="text-[9px] font-mono text-zinc-500">{{.Time}}</span>
            </div>
            <span class="text-xs font-mono font-bold text-zinc-100 break-all pt-0.5">{{.Filename}}</span>
            <span class="text-[10px] text-zinc-400 font-mono">produced by <span class="font-bold text-indigo-400">{{.Agent}}</span></span>
        </div>
    </div>
    <div class="grid grid-cols-2 gap-2 pt-1">
        <button onclick="openArtifactViewer('{{.Path}}', '{{.Filename}}', '{{.ArtifactType}}', '{{.Agent}}')"
                class="inline-flex items-center justify-center gap-1.5 px-3 py-2 bg-indigo-600 hover:bg-indigo-500 text-white rounded-lg text-[10px] font-bold uppercase tracking-wider transition-all active:scale-95 cursor-pointer shadow-sm">
            <span>👁️ Preview</span>
        </button>
        <a href="/api/artifact?path={{.Path}}" target="_blank"
           class="inline-flex items-center justify-center gap-1.5 px-3 py-2 bg-zinc-800 hover:bg-zinc-700 border border-zinc-700 text-zinc-200 rounded-lg text-[10px] font-bold uppercase tracking-wider transition-all active:scale-95 text-center">
            <span>↗️ Download</span>
        </a>
    </div>
</div>
<script>document.getElementById('artifacts-empty')?.remove();</script>
<style>
@keyframes fadeSlideIn {
    from { opacity: 0; transform: translateY(-8px); }
    to   { opacity: 1; transform: translateY(0); }
}
</style>
`))

// schedules-list-tmpl defines the nested template for schedules grid
var _ = template.Must(DashboardPage.Parse(`
{{ define "schedules-list-tmpl" }}
    {{ if . }}
        <div class="grid grid-cols-1 md:grid-cols-2 gap-4">
            {{ range . }}
            <div id="schedule-card-{{.ID}}" class="p-4 bg-zinc-900/80 border border-zinc-800 rounded-xl flex flex-col justify-between shadow-lg gap-3 hover:border-indigo-500/40 transition-all">
                <div class="flex flex-col gap-1.5">
                    <div class="flex items-center justify-between">
                        <span class="font-bold text-zinc-100 text-xs uppercase tracking-wider font-mono">{{.Name}}</span>
                        <span class="text-[9px] font-mono bg-emerald-500/10 text-emerald-400 border border-emerald-500/20 px-2 py-0.5 rounded-full uppercase font-bold">Active</span>
                    </div>
                    <div class="text-[10px] text-zinc-400 font-mono">
                        Cron: <span class="text-indigo-400 font-bold">{{.Cron}}</span>
                    </div>
                    <div class="text-[10px] text-zinc-400 font-mono">
                        Next Fire: <span class="text-emerald-400 font-bold">{{.NextFire}}</span>
                    </div>
                    <p class="text-xs text-zinc-300 bg-zinc-950 p-3 rounded-lg border border-zinc-800/80 font-mono line-clamp-3 leading-relaxed">
                        {{.Task}}
                    </p>
                </div>
                <button hx-delete="/api/schedules/{{.ID}}" hx-target="#schedules-list" hx-swap="innerHTML" class="w-full py-2 border border-rose-900/60 text-rose-400 hover:text-white hover:bg-rose-600 rounded-lg text-[9px] font-bold uppercase tracking-wider transition-all active:scale-95 cursor-pointer">
                    Delete Schedule
                </button>
            </div>
            {{ end }}
        </div>
    {{ else }}
        <div class="text-[11px] text-zinc-500 italic p-12 border border-dashed border-zinc-800 rounded-xl text-center font-mono bg-zinc-950/40">
            NO CRON SCHEDULES REGISTERED.
        </div>
    {{ end }}
{{ end }}
`))
