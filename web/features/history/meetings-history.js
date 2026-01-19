import { initAuth, login } from '/assets/js/auth.js';
import { getConfig } from '/assets/js/config.js';
import { getAccessToken, getLanguageName, escapeHtml } from '/assets/js/utils.js';

const authOverlay = document.getElementById('authOverlay');
const signInBtn = document.getElementById('signInBtn');
const mainContent = document.getElementById('mainContent');
const loadingState = document.getElementById('loadingState');
const meetingsGrid = document.getElementById('meetingsGrid');
const emptyState = document.getElementById('emptyState');
const pagination = document.getElementById('pagination');
const prevBtn = document.getElementById('prevBtn');
const nextBtn = document.getElementById('nextBtn');
const pageInfo = document.getElementById('pageInfo');
const statusFilter = document.getElementById('statusFilter');
const statusGrid = document.getElementById('statusGrid');
const statusTimestamp = document.getElementById('statusTimestamp');
const refreshStatusBtn = document.getElementById('refreshStatus');

const PAGE_LIMIT = 12;
let currentOffset = 0;
let currentStatus = 'all';
let totalMeetings = 0;

function showAuthRequired() {
    authOverlay.style.display = 'flex';
    mainContent.style.display = 'none';
}

function showMainContent() {
    authOverlay.style.display = 'none';
    mainContent.style.display = 'block';
}

function setLoading(isLoading) {
    loadingState.style.display = isLoading ? 'block' : 'none';
    meetingsGrid.style.display = 'none';
    emptyState.style.display = 'none';
    pagination.style.display = 'none';
}

function showEmptyState(title, message, showAction) {
    const titleEl = emptyState.querySelector('h3');
    const messageEl = emptyState.querySelector('p');
    const actionEl = emptyState.querySelector('a');

    titleEl.textContent = title;
    messageEl.textContent = message;
    actionEl.style.display = showAction ? 'inline-flex' : 'none';

    loadingState.style.display = 'none';
    meetingsGrid.style.display = 'none';
    emptyState.style.display = 'block';
    pagination.style.display = 'none';
}

function formatDateTime(isoString) {
    if (!isoString) return 'Unknown';
    const date = new Date(isoString);
    if (Number.isNaN(date.getTime())) return 'Unknown';
    return date.toLocaleString();
}

function formatDuration(seconds) {
    if (seconds === null || seconds === undefined) return 'In progress';
    const totalSeconds = Math.max(0, seconds);
    const hours = Math.floor(totalSeconds / 3600);
    const minutes = Math.floor((totalSeconds % 3600) / 60);
    if (hours > 0) {
        return `${hours}h ${minutes}m`;
    }
    return `${minutes}m`;
}

function getRoleBadgeClass(role) {
    switch (role) {
        case 'owner':
            return 'role-badge role-owner';
        case 'editor':
            return 'role-badge role-editor';
        case 'viewer':
            return 'role-badge role-viewer';
        default:
            return 'role-badge';
    }
}

function renderMeetings(meetings) {
    if (!meetings || meetings.length === 0) {
        showEmptyState('No meetings found', 'Your past meetings will appear here.', true);
        return;
    }

    meetingsGrid.innerHTML = meetings.map((meeting) => {
        const languages = (meeting.availableLanguages || []).map((lang) => (
            `<span class="lang-badge">${escapeHtml(getLanguageName(lang))}</span>`
        )).join('');

        const userRole = meeting.userRole || meeting.role || 'viewer';
        const roleBadgeClass = getRoleBadgeClass(userRole);

        return `
            <div class="meeting-card" data-id="${escapeHtml(meeting.id)}">
                <div class="meeting-card-header">
                    <div>
                        <div class="meeting-code">${escapeHtml(meeting.roomCode)}</div>
                        <span class="${roleBadgeClass}">${escapeHtml(userRole)}</span>
                    </div>
                    <span class="meeting-status ${meeting.isActive ? 'active' : 'ended'}">
                        ${meeting.isActive ? 'Active' : 'Ended'}
                    </span>
                </div>
                <div class="meeting-meta">
                    <div class="meeting-meta-item">Date: ${formatDateTime(meeting.createdAt)}</div>
                    <div class="meeting-meta-item">Duration: ${formatDuration(meeting.durationSeconds)}</div>
                    <div class="meeting-meta-item">Participants: ${meeting.participantCount}</div>
                </div>
                ${meeting.minutesSummary ? `<div class="minutes-summary">Minutes: ${escapeHtml(meeting.minutesSummary)}</div>` : ''}
                ${languages ? `<div class="meeting-languages">${languages}</div>` : ''}
            </div>
        `;
    }).join('');

    meetingsGrid.querySelectorAll('.meeting-card').forEach((card) => {
        card.addEventListener('click', () => {
            const meetingId = card.dataset.id;
            if (meetingId) {
                window.location.href = `/features/history/meeting-detail.html?id=${encodeURIComponent(meetingId)}`;
            }
        });
    });

    meetingsGrid.style.display = 'grid';
    emptyState.style.display = 'none';
    loadingState.style.display = 'none';
}

function updatePagination() {
    if (totalMeetings <= PAGE_LIMIT) {
        pagination.style.display = 'none';
        return;
    }

    const start = currentOffset + 1;
    const end = Math.min(currentOffset + PAGE_LIMIT, totalMeetings);
    pageInfo.textContent = `Showing ${start}-${end} of ${totalMeetings}`;

    prevBtn.disabled = currentOffset === 0;
    nextBtn.disabled = currentOffset + PAGE_LIMIT >= totalMeetings;

    pagination.style.display = 'flex';
}

function formatStatusDetail(data) {
    if (!data || typeof data !== 'object') {
        return '';
    }

    const details = [];
    if (data.status) {
        details.push(`status: ${data.status}`);
    }
    if (data.model) {
        details.push(`model: ${data.model}`);
    } else if (data.default_model) {
        details.push(`model: ${data.default_model}`);
    }
    if (typeof data.xtts_loaded === 'boolean') {
        details.push(`xtts: ${data.xtts_loaded ? 'ready' : 'loading'}`);
    }

    return details.join(' • ');
}

async function checkServiceHealth(service, baseUrl) {
    const item = statusGrid.querySelector(`[data-service="${service}"]`);
    if (!item) {
        return;
    }

    const pill = item.querySelector('.status-pill');
    const detail = item.querySelector('.status-detail');

    pill.textContent = 'Checking...';
    pill.classList.remove('ok', 'error');
    detail.textContent = '';

    if (!baseUrl) {
        pill.textContent = 'Not configured';
        pill.classList.add('error');
        return;
    }

    const controller = new AbortController();
    const timeoutId = setTimeout(() => controller.abort(), 5000);

    try {
        const response = await fetch(`${baseUrl}/health`, { signal: controller.signal });
        clearTimeout(timeoutId);

        if (!response.ok) {
            pill.textContent = `Error (${response.status})`;
            pill.classList.add('error');
            return;
        }

        let data = null;
        try {
            data = await response.json();
        } catch (err) {
            data = null;
        }

        pill.textContent = 'Healthy';
        pill.classList.add('ok');
        detail.textContent = formatStatusDetail(data);
    } catch (error) {
        clearTimeout(timeoutId);
        const isTimeout = error && error.name === 'AbortError';
        pill.textContent = isTimeout ? 'Timeout' : 'Unavailable';
        pill.classList.add('error');
    }
}

async function loadServiceStatus() {
    if (!statusGrid) {
        return;
    }

    const config = await getConfig();
    const services = config.services || {};

    await Promise.all([
        checkServiceHealth('asr', services.asrBaseUrl),
        checkServiceHealth('translation', services.translationBaseUrl),
        checkServiceHealth('tts', services.ttsBaseUrl),
        checkServiceHealth('embedding', services.embeddingBaseUrl),
        checkServiceHealth('llm', services.llmBaseUrl)
    ]);

    if (statusTimestamp) {
        const now = new Date();
        statusTimestamp.textContent = `Last checked: ${now.toLocaleTimeString()}`;
    }
}

async function loadMeetings() {
    setLoading(true);

    const token = getAccessToken();
    if (!token) {
        showAuthRequired();
        return;
    }

    try {
        const url = new URL('/api/users/me/meetings', window.location.origin);
        url.searchParams.set('limit', PAGE_LIMIT.toString());
        url.searchParams.set('offset', currentOffset.toString());
        if (currentStatus !== 'all') {
            url.searchParams.set('status', currentStatus);
        }

        const response = await fetch(url.toString(), {
            headers: {
                'Authorization': `Bearer ${token}`
            }
        });

        if (response.status === 401 || response.status === 403) {
            showAuthRequired();
            return;
        }

        if (!response.ok) {
            throw new Error(`Failed to load meetings (${response.status})`);
        }

        const data = await response.json();
        totalMeetings = data.total || 0;
        renderMeetings(data.meetings || []);
        updatePagination();
    } catch (error) {
        console.error('Failed to load meetings:', error);
        showEmptyState('Unable to load meetings', 'Please try again in a moment.', false);
    }
}

async function init() {
    signInBtn.addEventListener('click', () => login());
    refreshStatusBtn.addEventListener('click', () => loadServiceStatus());

    const profile = await initAuth();
    const token = getAccessToken();
    if (!profile || !token) {
        showAuthRequired();
        return;
    }

    showMainContent();
    await loadServiceStatus();
    await loadMeetings();
}

statusFilter.addEventListener('change', () => {
    currentStatus = statusFilter.value;
    currentOffset = 0;
    loadMeetings();
});

prevBtn.addEventListener('click', () => {
    currentOffset = Math.max(0, currentOffset - PAGE_LIMIT);
    loadMeetings();
});

nextBtn.addEventListener('click', () => {
    currentOffset = currentOffset + PAGE_LIMIT;
    loadMeetings();
});

init();
