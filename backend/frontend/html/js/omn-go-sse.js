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

		    const capAction = action.charAt(0).toUpperCase() + action.slice(1);
                    
                    if (res.ok) {
			if (confirm(capAction + ' complete!\n\nWould you like to reload the page now to see updated content (console will be reset)?')) {
                            window.location.reload();
			}
                    } else {
                        let msg = await res.text();
                        console.error('OMN-Go sync failed:', msg);
                        alert(capAction + ' failed: ' + msg + '\n\nOpen console to copy error details.');
                    }
                }
    
        // Export to global scope to preserve HTML onclick attributes
        window.syncAction = syncAction;
        return { syncAction };
    })();
    
} else {
    console.warn("OMN-Go: Page opened locally. Server Extensions (Sync/SSE) safely disabled.");
}
