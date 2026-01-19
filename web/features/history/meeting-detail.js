import { initAuth, login } from '/assets/js/auth.js';
import { getAccessToken, downloadBlob, escapeHtml, getLanguageName } from '/assets/js/utils.js';

const authOverlay = document.getElementById('authOverlay');
const signInBtn = document.getElementById('signInBtn');
const mainContent = document.getElementById('mainContent');

const roomCodeEl = document.getElementById('roomCode');
const meetingSubtitle = document.getElementById('meetingSubtitle');
const meetingStatus = document.getElementById('meetingStatus');
const meetingCreated = document.getElementById('meetingCreated');
const meetingDuration = document.getElementById('meetingDuration');
const meetingChunks = document.getElementById('meetingChunks');
const meetingRoleEl = document.getElementById('meetingRole');
const accessControlSection = document.getElementById('accessControlSection');
const accessControlList = document.getElementById('accessControlList');
const grantAccessBtn = document.getElementById('grantAccessBtn');
const participantsList = document.getElementById('participantsList');
const snapshotTable = document.getElementById('snapshotTable');
const snapshotBody = document.getElementById('snapshotBody');
const snapshotEmpty = document.getElementById('snapshotEmpty');
const minutesEmpty = document.getElementById('minutesEmpty');
const minutesContent = document.getElementById('minutesContent');
const minutesParticipants = document.getElementById('minutesParticipants');
const minutesKeyPoints = document.getElementById('minutesKeyPoints');
const minutesActionItems = document.getElementById('minutesActionItems');
const minutesDecisions = document.getElementById('minutesDecisions');
const minutesSummary = document.getElementById('minutesSummary');

const chatLanguage = document.getElementById('chatLanguage');
const chatResponseLanguage = document.getElementById('chatResponseLanguage');
const chatStatus = document.getElementById('chatStatus');
const chatMessages = document.getElementById('chatMessages');
const chatInput = document.getElementById('chatInput');
const sendBtn = document.getElementById('sendBtn');

const tabButtons = document.querySelectorAll('.tab-button');
const tabInfo = document.getElementById('tabInfo');
const tabChat = document.getElementById('tabChat');

const urlParams = new URLSearchParams(window.location.search);
const meetingId = urlParams.get('id');
let meetingRoomCode = '';
let chatSessionId = '';
let chatReady = false;
let preferredChatLanguage = localStorage.getItem('chatResponseLanguage') || 'en';

function showAuthRequired() {
    authOverlay.style.display = 'flex';
    mainContent.style.display = 'none';
}

function showMainContent() {
    authOverlay.style.display = 'none';
    mainContent.style.display = 'block';
}

function formatDateTime(isoString) {
    if (!isoString) return 'Unknown';
    const date = new Date(isoString);
    if (Number.isNaN(date.getTime())) return 'Unknown';
    return date.toLocaleString();
}

function formatDuration(createdAt, endedAt) {
    if (!createdAt || !endedAt) return 'In progress';
    const start = new Date(createdAt);
    const end = new Date(endedAt);
    if (Number.isNaN(start.getTime()) || Number.isNaN(end.getTime())) return 'Unknown';
    const seconds = Math.max(0, Math.floor((end - start) / 1000));
    const hours = Math.floor(seconds / 3600);
    const minutes = Math.floor((seconds % 3600) / 60);
    if (hours > 0) {
        return `${hours}h ${minutes}m`;
    }
    return `${minutes}m`;
}

function setChatStatus(message) {
    chatStatus.textContent = message;
}

function addChatMessage(role, text, language) {
    const message = document.createElement('div');
    message.className = `chat-message ${role}`;
    message.textContent = text;

    // Add language attribute for RTL support
    if (language) {
        message.setAttribute('data-lang', language);
    }

    chatMessages.appendChild(message);
    chatMessages.scrollTop = chatMessages.scrollHeight;
}

function resetChat() {
    chatMessages.innerHTML = '';
    chatSessionId = '';
    if (!chatReady) {
        addChatMessage('assistant', 'RAG chat is not available for this meeting yet.');
    }
}

async function createChatSession(token) {
    const language = chatLanguage.value;
    if (!language || !meetingId) return null;

    const response = await fetch('/api/chat/sessions', {
        method: 'POST',
        headers: {
            'Content-Type': 'application/json',
            ...(token ? { 'Authorization': `Bearer ${token}` } : {})
        },
        body: JSON.stringify({
            meetingId,
            language
        })
    });

    if (!response.ok) {
        throw new Error(`Failed to create chat session (${response.status})`);
    }

    const data = await response.json();
    return data.sessionId || data.id || data.session_id || null;
}

async function sendChatMessage() {
    const question = chatInput.value.trim();
    if (!question || !meetingId) return;

    if (!chatReady) {
        addChatMessage('assistant', 'RAG chat is not available for this meeting yet.');
        return;
    }

    const token = getAccessToken();

    sendBtn.disabled = true;
    chatInput.disabled = true;

    addChatMessage('user', question);
    chatInput.value = '';

    try {
        if (!chatSessionId) {
            chatSessionId = await createChatSession(token);
        }

        if (!chatSessionId) {
            throw new Error('Unable to create chat session');
        }

        const response = await fetch('/api/chat/query', {
            method: 'POST',
            headers: {
                'Content-Type': 'application/json',
                ...(token ? { 'Authorization': `Bearer ${token}` } : {})
            },
            body: JSON.stringify({
                sessionId: chatSessionId,
                question,
                meetingId,
                language: chatLanguage.value,
                chatLanguage: chatResponseLanguage.value,
                topK: 5
            })
        });

        if (!response.ok) {
            throw new Error(`Query failed (${response.status})`);
        }

        const data = await response.json();
        addChatMessage('assistant', data.answer || 'No answer returned.', chatResponseLanguage.value);
    } catch (error) {
        console.error('Chat query failed:', error);
        const isNetworkError = error && (error.name === 'TypeError' || `${error}`.includes('NetworkError'));
        const fallbackMessage = isNetworkError
            ? 'The chat service is warming up. Please retry in a few seconds.'
            : 'Sorry, I could not answer that right now.';
        addChatMessage('assistant', fallbackMessage);
        if (isNetworkError) {
            setChatStatus('Service warming up. Retry your question shortly.');
        }
    } finally {
        sendBtn.disabled = false;
        chatInput.disabled = false;
        chatInput.focus();
    }
}

function renderParticipants(participants) {
    if (!participants || participants.length === 0) {
        participantsList.innerHTML = '<div class="placeholder-text">No participants recorded.</div>';
        return;
    }

    participantsList.innerHTML = participants.map((participant) => `
        <div class="participant-card">
            <div>
                <strong>${escapeHtml(participant.name || 'Guest')}</strong>
                <div class="participant-meta">Target: ${escapeHtml(getLanguageName(participant.targetLanguage || ''))}</div>
            </div>
            <div class="participant-meta">
                Joined: ${formatDateTime(participant.joinedAt)}<br>
                Left: ${participant.leftAt ? formatDateTime(participant.leftAt) : 'Active'}
            </div>
        </div>
    `).join('');
}

function renderSnapshots(snapshots) {
    if (!snapshots || snapshots.length === 0) {
        snapshotTable.style.display = 'none';
        snapshotEmpty.textContent = 'No transcript snapshots available for this meeting.';
        return;
    }

    snapshotEmpty.textContent = '';
    snapshotTable.style.display = 'table';
    snapshotBody.innerHTML = snapshots.map((snapshot) => `
        <tr>
            <td>${escapeHtml(getLanguageName(snapshot.language))}</td>
            <td>${formatDateTime(snapshot.createdAt)}</td>
            <td><button class="btn-secondary" data-lang="${escapeHtml(snapshot.language)}">Download</button></td>
        </tr>
    `).join('');

    snapshotBody.querySelectorAll('button').forEach((button) => {
        button.addEventListener('click', async () => {
            const language = button.dataset.lang;
            if (!meetingRoomCode || !language) return;
            try {
                const response = await fetch(`/api/meetings/${encodeURIComponent(meetingRoomCode)}/transcript-snapshot?lang=${encodeURIComponent(language)}`);
                if (!response.ok) {
                    throw new Error(`Download failed (${response.status})`);
                }
                const blob = await response.blob();
                downloadBlob(blob, `meeting-${meetingRoomCode}-${language}-transcript.txt`);
            } catch (error) {
                console.error('Snapshot download failed:', error);
                alert('Failed to download transcript snapshot.');
            }
        });
    });
}

function renderListSection(title, items) {
    if (!items || items.length === 0) {
        return `<div class=\"placeholder-text\">No ${title.toLowerCase()} recorded.</div>`;
    }
    const listItems = items.map((item) => `<li>${escapeHtml(item)}</li>`).join('');
    return `<strong>${escapeHtml(title)}</strong><ul>${listItems}</ul>`;
}

function renderMinutes(minutes, summaryText) {
    if (!minutes && !summaryText) {
        minutesEmpty.textContent = 'Minutes not available yet. Check back after processing completes.';
        minutesContent.style.display = 'none';
        return;
    }

    minutesEmpty.textContent = '';
    minutesContent.style.display = 'block';

    const participants = (minutes && minutes.participants) ? minutes.participants : [];
    minutesParticipants.innerHTML = participants.length
        ? participants.map((name) => `
            <div class=\"participant-card\">
                <strong>${escapeHtml(name)}</strong>
            </div>
        `).join('')
        : '<div class=\"placeholder-text\">No participants listed.</div>';

    const keyPoints = minutes ? minutes.key_points : [];
    const actionItems = minutes ? minutes.action_items : [];
    const decisions = minutes ? minutes.decisions : [];
    const summary = minutes && minutes.summary ? minutes.summary : summaryText;

    minutesKeyPoints.innerHTML = renderListSection('Key Points', keyPoints);
    minutesActionItems.innerHTML = renderListSection('Action Items', actionItems);
    minutesDecisions.innerHTML = renderListSection('Decisions', decisions);
    minutesSummary.innerHTML = summary
        ? `<strong>Summary</strong><p>${escapeHtml(summary)}</p>`
        : '<div class=\"placeholder-text\">No summary provided.</div>';
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

function renderAccessControl(accessControl) {
    if (!accessControl || accessControl.length === 0) {
        accessControlList.innerHTML = '<div class="placeholder-text">No explicit access permissions set. All participants have viewer access by default.</div>';
        return;
    }

    accessControlList.innerHTML = accessControl.map((entry) => {
        const badgeClass = getRoleBadgeClass(entry.role);
        return `
            <div class="access-entry">
                <div class="access-user">
                    <strong>${escapeHtml(entry.displayName || entry.username)}</strong>
                    <div class="access-meta">@${escapeHtml(entry.username)}</div>
                </div>
                <div class="access-controls">
                    <select class="role-select" data-user-id="${entry.userId}">
                        <option value="viewer" ${entry.role === 'viewer' ? 'selected' : ''}>Viewer</option>
                        <option value="editor" ${entry.role === 'editor' ? 'selected' : ''}>Editor</option>
                    </select>
                    <button class="btn-danger-small revoke-btn" data-user-id="${entry.userId}" data-username="${escapeHtml(entry.displayName || entry.username)}">Revoke</button>
                </div>
            </div>
        `;
    }).join('');

    // Add event listeners for role changes
    accessControlList.querySelectorAll('.role-select').forEach((select) => {
        select.addEventListener('change', async (e) => {
            const userId = parseInt(e.target.dataset.userId);
            const newRole = e.target.value;
            await updateUserRole(userId, newRole);
        });
    });

    // Add event listeners for revoke buttons
    accessControlList.querySelectorAll('.revoke-btn').forEach((button) => {
        button.addEventListener('click', async (e) => {
            const userId = parseInt(e.target.dataset.userId);
            const username = e.target.dataset.username;
            if (confirm(`Are you sure you want to revoke access for ${username}?`)) {
                await revokeUserAccess(userId);
            }
        });
    });
}

async function updateUserRole(userId, newRole) {
    const token = getAccessToken();
    if (!token) return;

    try {
        const response = await fetch('/api/meetings/access/update', {
            method: 'PUT',
            headers: {
                'Content-Type': 'application/json',
                'Authorization': `Bearer ${token}`
            },
            body: JSON.stringify({
                meetingId: meetingId,
                userId: userId,
                role: newRole
            })
        });

        if (!response.ok) {
            throw new Error(`Failed to update role (${response.status})`);
        }

        await loadMeetingDetail();
    } catch (error) {
        console.error('Failed to update role:', error);
        alert('Failed to update role. Please try again.');
        await loadMeetingDetail();
    }
}

async function revokeUserAccess(userId) {
    const token = getAccessToken();
    if (!token) return;

    try {
        const response = await fetch('/api/meetings/access/revoke', {
            method: 'DELETE',
            headers: {
                'Content-Type': 'application/json',
                'Authorization': `Bearer ${token}`
            },
            body: JSON.stringify({
                meetingId: meetingId,
                userId: userId
            })
        });

        if (!response.ok) {
            const data = await response.json();
            throw new Error(data.error || `Failed to revoke access (${response.status})`);
        }

        await loadMeetingDetail();
    } catch (error) {
        console.error('Failed to revoke access:', error);
        alert(error.message || 'Failed to revoke access. Please try again.');
        await loadMeetingDetail();
    }
}

async function showGrantAccessModal() {
    const token = getAccessToken();
    if (!token) return;

    // Get available participants
    try {
        const response = await fetch(`/api/meetings/participants/available/${encodeURIComponent(meetingId)}`, {
            headers: {
                'Authorization': `Bearer ${token}`
            }
        });

        if (!response.ok) {
            throw new Error(`Failed to get available participants (${response.status})`);
        }

        const data = await response.json();
        const participants = data.participants || [];

        if (participants.length === 0) {
            alert('No participants available to grant access. All participants already have explicit permissions.');
            return;
        }

        const selectedUserId = prompt(`Select user ID to grant access:\n${participants.map(p => `${p.userId}: ${p.participantName}`).join('\n')}`);

        if (!selectedUserId) return;

        const userId = parseInt(selectedUserId);
        const role = prompt('Select role (viewer or editor):', 'viewer');

        if (!role || (role !== 'viewer' && role !== 'editor')) {
            alert('Invalid role. Must be "viewer" or "editor".');
            return;
        }

        await grantAccess(userId, role);
    } catch (error) {
        console.error('Failed to show grant access modal:', error);
        alert('Failed to load participants. Please try again.');
    }
}

async function grantAccess(userId, role) {
    const token = getAccessToken();
    if (!token) return;

    try {
        const response = await fetch('/api/meetings/access/grant', {
            method: 'POST',
            headers: {
                'Content-Type': 'application/json',
                'Authorization': `Bearer ${token}`
            },
            body: JSON.stringify({
                meetingId: meetingId,
                userId: userId,
                role: role
            })
        });

        if (!response.ok) {
            const data = await response.json();
            throw new Error(data.error || `Failed to grant access (${response.status})`);
        }

        await loadMeetingDetail();
    } catch (error) {
        console.error('Failed to grant access:', error);
        alert(error.message || 'Failed to grant access. Please try again.');
    }
}

function renderChatLanguages(snapshots) {
    chatLanguage.innerHTML = '';

    if (!snapshots || snapshots.length === 0) {
        chatLanguage.innerHTML = '<option value="">No transcript languages</option>';
        chatLanguage.disabled = true;
        setChatStatus('Transcript snapshots unavailable.');
        return;
    }

    chatLanguage.disabled = false;
    chatLanguage.innerHTML = snapshots.map((snapshot) => (
        `<option value="${escapeHtml(snapshot.language)}">${escapeHtml(getLanguageName(snapshot.language))}</option>`
    )).join('');

    setChatStatus('Ready to answer questions about this transcript.');
}

function initializeChatResponseLanguage() {
    const languages = [
        { code: 'en', name: 'English' },
        { code: 'ar', name: 'العربية (Arabic)' },
        { code: 'ur', name: 'اردو (Urdu)' },
        { code: 'hi', name: 'हिन्दी (Hindi)' },
        { code: 'ml', name: 'മലയാളം (Malayalam)' },
        { code: 'te', name: 'తెలుగు (Telugu)' },
        { code: 'ta', name: 'தமிழ் (Tamil)' },
        { code: 'bn', name: 'বাংলা (Bengali)' },
        { code: 'fr', name: 'Français (French)' },
        { code: 'es', name: 'Español (Spanish)' },
        { code: 'de', name: 'Deutsch (German)' },
        { code: 'zh', name: '中文 (Chinese)' },
        { code: 'ja', name: '日本語 (Japanese)' },
        { code: 'ko', name: '한국어 (Korean)' }
    ];

    chatResponseLanguage.innerHTML = languages.map((lang) =>
        `<option value="${escapeHtml(lang.code)}">${escapeHtml(lang.name)}</option>`
    ).join('');

    chatResponseLanguage.value = preferredChatLanguage;
}

function updateSummary(detail) {
    meetingRoomCode = detail.roomCode || '';
    roomCodeEl.textContent = meetingRoomCode || detail.id || 'Unknown';

    meetingSubtitle.textContent = detail.isActive ? 'Meeting is still active.' : 'Meeting has ended.';
    meetingStatus.textContent = detail.isActive ? 'Active' : 'Ended';
    meetingCreated.textContent = formatDateTime(detail.createdAt);
    meetingDuration.textContent = formatDuration(detail.createdAt, detail.endedAt);
    meetingChunks.textContent = detail.chunkCount ? `${detail.chunkCount} chunks` : 'Not ready';
    chatReady = Boolean(detail.hasRAGChunks);

    // Display user's role
    if (detail.userRole && meetingRoleEl) {
        const roleBadgeClass = getRoleBadgeClass(detail.userRole);
        meetingRoleEl.innerHTML = `<span class="${roleBadgeClass}">${escapeHtml(detail.userRole)}</span>`;
    }

    // Show/hide access control section based on permissions
    if (detail.canManageAccess && accessControlSection) {
        accessControlSection.style.display = 'block';
        renderAccessControl(detail.accessControl || []);

        // Set up grant access button
        if (grantAccessBtn) {
            grantAccessBtn.onclick = showGrantAccessModal;
        }
    } else if (accessControlSection) {
        accessControlSection.style.display = 'none';
    }

    if (!chatReady) {
        setChatStatus('RAG chat is not available for this meeting yet.');
        sendBtn.disabled = true;
        chatInput.disabled = true;
    } else {
        sendBtn.disabled = false;
        chatInput.disabled = false;
    }
}

async function loadMeetingDetail() {
    if (!meetingId) {
        meetingSubtitle.textContent = 'Missing meeting ID.';
        return;
    }

    const token = getAccessToken();
    if (!token) {
        showAuthRequired();
        return;
    }

    try {
        const response = await fetch(`/api/users/me/meetings/${encodeURIComponent(meetingId)}`, {
            headers: {
                'Authorization': `Bearer ${token}`
            }
        });

        if (response.status === 401 || response.status === 403) {
            showAuthRequired();
            return;
        }

        if (!response.ok) {
            throw new Error(`Failed to load meeting (${response.status})`);
        }

        const data = await response.json();
        const detail = data.meeting;

        updateSummary(detail);
        renderParticipants(detail.participants || []);
        renderSnapshots(detail.transcriptSnapshots || []);
        renderChatLanguages(detail.transcriptSnapshots || []);
        renderMinutes(detail.minutes || null, detail.minutesSummary || '');
        resetChat();
    } catch (error) {
        console.error('Failed to load meeting detail:', error);
        meetingSubtitle.textContent = 'Unable to load meeting details.';
    }
}

function setupTabs() {
    tabButtons.forEach((button) => {
        button.addEventListener('click', () => {
            tabButtons.forEach((btn) => btn.classList.remove('active'));
            button.classList.add('active');

            if (button.dataset.tab === 'chat') {
                tabChat.classList.add('active');
                tabInfo.classList.remove('active');
            } else {
                tabInfo.classList.add('active');
                tabChat.classList.remove('active');
            }
        });
    });
}

function setupChatControls() {
    sendBtn.addEventListener('click', sendChatMessage);
    chatInput.addEventListener('keydown', (event) => {
        if (event.key === 'Enter') {
            sendChatMessage();
        }
    });

    chatLanguage.addEventListener('change', () => {
        chatSessionId = '';
        chatMessages.innerHTML = '';
        if (chatReady) {
            addChatMessage('assistant', 'Language updated. Ask your next question.');
        }
    });

    chatResponseLanguage.addEventListener('change', () => {
        preferredChatLanguage = chatResponseLanguage.value;
        localStorage.setItem('chatResponseLanguage', preferredChatLanguage);
        addChatMessage('assistant', 'Response language updated. Your next answer will be in the selected language.', preferredChatLanguage);
    });
}

async function init() {
    signInBtn.addEventListener('click', () => login());

    const profile = await initAuth();
    const token = getAccessToken();
    if (!profile || !token) {
        showAuthRequired();
        return;
    }

    showMainContent();
    setupTabs();
    setupChatControls();
    initializeChatResponseLanguage();
    await loadMeetingDetail();
}

init();
