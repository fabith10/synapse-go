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