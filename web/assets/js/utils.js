/**
 * General Utility Functions
 */

/**
 * Debounce function calls
 */
export function debounce(func, wait) {
    let timeout;
    return function executedFunction(...args) {
        const later = () => {
            clearTimeout(timeout);
            func(...args);
        };
        clearTimeout(timeout);
        timeout = setTimeout(later, wait);
    };
}

/**
 * Format timestamp for display
 */
export function formatTimestamp(date) {
    return date.toLocaleTimeString('en-US', {
        hour12: false,
        hour: '2-digit',
        minute: '2-digit',
        second: '2-digit'
    });
}

/**
 * Escape HTML to prevent XSS
 */
export function escapeHtml(text) {
    const div = document.createElement('div');
    div.textContent = text;
    return div.innerHTML;
}

/**
 * Get language display name from code
 */
export function getLanguageName(code) {
    const languages = {
        'en': 'English',
        'ar': 'Arabic',
        'ur': 'Urdu',
        'es': 'Spanish',
        'fr': 'French',
        'de': 'German',
        'zh': 'Chinese',
        'ja': 'Japanese',
        'ko': 'Korean',
        'hi': 'Hindi'
    };
    return languages[code] || code;
}

/**
 * Download a file from blob
 */
export function downloadBlob(blob, filename) {
    const url = URL.createObjectURL(blob);
    const link = document.createElement('a');
    link.href = url;
    link.download = filename;
    document.body.appendChild(link);
    link.click();
    document.body.removeChild(link);
    URL.revokeObjectURL(url);
}

export function getAccessToken() {
    const authRaw = localStorage.getItem('keycloak_auth');
    if (authRaw) {
        try {
            const auth = JSON.parse(authRaw);
            if (auth && auth.access_token) {
                return auth.access_token;
            }
        } catch (err) {
            console.warn('Invalid auth storage', err);
        }
    }

    return (
        localStorage.getItem('keycloak_access_token') ||
        sessionStorage.getItem('keycloak_access_token') ||
        localStorage.getItem('access_token') ||
        sessionStorage.getItem('access_token')
    );
}

export async function postJsonWithAuth(url, payload) {
    const token = getAccessToken();
    if (!token) {
        console.info('Auth token missing; skipping history write.');
        return null;
    }

    const response = await fetch(url, {
        method: 'POST',
        headers: {
            'Content-Type': 'application/json',
            'Authorization': `Bearer ${token}`
        },
        body: JSON.stringify(payload)
    });

    if (!response.ok) {
        console.warn(`History write failed: ${response.status}`);
        return null;
    }

    return response.json();
}
