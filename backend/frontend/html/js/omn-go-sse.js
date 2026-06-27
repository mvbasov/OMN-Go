// --- OMN-Go Server Extensions ---
// These modules interact with the Go backend API. They will cleanly bypass themselves
// if the user is merely viewing an exported HTML file locally without the server.

if (window.location.protocol !== 'file:') {
const Logger = (function() {
        async function syncAction(action) {
                    let force = document.getElementById('forceSyncCb') && document.getElementById('forceSyncCb').checked;
                    if (force) {
                        if (!confirm("WARNING: Force " + action + " is a destructive operation that may overwrite remote or local changes. Are you sure?")) {
                            return;
                        }
                    }
                    const fd = new URLSearchParams();
                    fd.append('action', action);
                    if (force) fd.append('force', 'true');
                    
                    const res = await fetch('/api/sync', { method: 'POST', body: fd });
                    
                    if (force && document.getElementById('forceSyncCb')) {
                        document.getElementById('forceSyncCb').checked = false;
                    }
                    
                    if (res.ok) {
                        alert(action.charAt(0).toUpperCase() + action.slice(1) + ' complete!');
                        window.location.reload();
                    } else {
                        let msg = await res.text();
                        console.error('OMN-Go sync failed:', msg);
                        alert(action.charAt(0).toUpperCase() + action.slice(1) + ' failed: ' + msg + '\n\nOpen console (F12) to copy error details.');
                    }
                }
    
        // Export to global scope to preserve HTML onclick attributes
        window.syncAction = syncAction;
        return { syncAction };
    })();
    
} else {
    console.warn("OMN-Go: Page opened locally. Server Extensions (Sync/SSE) safely disabled.");
}
