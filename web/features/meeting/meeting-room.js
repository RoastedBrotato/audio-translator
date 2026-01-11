/**
 * Meeting Room - Real-time Audio Translation
 * Refactored to use shared utilities
 */

// Import shared utilities
import { convertToPCM16, getAudioLevel, resampleAudio } from '/assets/js/audio-processor.js';
import { getLanguageName, escapeHtml, getAccessToken } from '/assets/js/utils.js';

// Meeting WebSocket Client
let meetingWs = null;
let audioContext = null;
let audioSource = null;
let audioProcessor = null;
let isMuted = true;
let isConnected = false;

// Session data
let myParticipantId = null;
let myParticipantName = null;
let myTargetLanguage = null;
let meetingId = null;
let roomCode = null;
let meetingMode = null;
let hostToken = null;
let isEndingMeeting = false;

// Track speaking participants
const speakingParticipants = new Set();
const diarizationDefaults = {
    minSpeakers: 2,
    maxSpeakers: 0,
    strictness: 0.5
};
const diarizationPresets = {
    fast: { minSpeakers: 1, maxSpeakers: 0, strictness: 0.2 },
    balanced: { minSpeakers: 2, maxSpeakers: 0, strictness: 0.5 },
    strict: { minSpeakers: 2, maxSpeakers: 0, strictness: 0.8 }
};

// Initialize on page load
document.addEventListener('DOMContentLoaded', async function() {
    // Get session data
    myParticipantId = sessionStorage.getItem('participantId');
    myParticipantName = sessionStorage.getItem('participantName');
    myTargetLanguage = sessionStorage.getItem('targetLanguage');
    meetingId = sessionStorage.getItem('meetingId');
    roomCode = sessionStorage.getItem('roomCode');
    hostToken = sessionStorage.getItem('hostToken');
    if (!hostToken && roomCode) {
        const storedHostRoomCode = localStorage.getItem('hostRoomCode');
        const storedHostToken = localStorage.getItem('hostToken');
        if (storedHostRoomCode === roomCode && storedHostToken) {
            hostToken = storedHostToken;
            sessionStorage.setItem('hostToken', storedHostToken);
            sessionStorage.setItem('hostRoomCode', storedHostRoomCode);
        }
    }

    // Check if session data exists
    if (!myParticipantId || !myParticipantName || !myTargetLanguage || !meetingId) {
        alert('Missing session data. Please join the meeting again.');
        window.location.href = 'meeting-join.html';
        return;
    }

    // Display room code
    document.getElementById('roomCode').textContent = roomCode || meetingId;

    // Fetch meeting info to get mode
    try {
        const response = await fetch(`/api/meetings/${roomCode || meetingId}`);
        const data = await response.json();
        if (data.success) {
            meetingMode = data.mode;
            const modeText = meetingMode === 'shared' ? '🎤 Shared Room (Speaker Identification)' : '📱 Individual Devices';
            document.getElementById('meetingMode').textContent = modeText;
        }
    } catch (error) {
        console.error('Failed to fetch meeting info:', error);
        meetingMode = 'individual'; // Default
    }

    initializeDiarizationControls();

    // Set language selector
    document.getElementById('languageChange').value = myTargetLanguage;

    // Setup event listeners
    setupEventListeners();

    // Connect to meeting
    connectToMeeting();

    // Link guest participant to authenticated user if available.
    linkParticipantToUser();
});

async function linkParticipantToUser() {
    const token = getAccessToken();
    if (!token || !myParticipantId || !meetingId) {
        return;
    }

    const meetingKey = roomCode || meetingId;
    try {
        await fetch(`/api/meetings/${meetingKey}/link`, {
            method: 'POST',
            headers: {
                'Content-Type': 'application/json',
                'Authorization': `Bearer ${token}`
            },
            body: JSON.stringify({
                participantId: parseInt(myParticipantId, 10)
            })
        });
    } catch (error) {
        console.warn('Failed to link participant to user:', error);
    }
}

function setupEventListeners() {
    // Mute/Unmute button
    document.getElementById('muteButton').addEventListener('click', toggleMute);

    // Leave meeting button
    document.getElementById('leaveButton').addEventListener('click', leaveMeeting);

    const endMeetingButton = document.getElementById('endMeetingButton');
    if (endMeetingButton && hostToken) {
        endMeetingButton.style.display = 'inline-flex';
        endMeetingButton.addEventListener('click', endMeeting);
    }

    // Language change
    document.getElementById('languageChange').addEventListener('change', function(e) {
        myTargetLanguage = e.target.value;
        sessionStorage.setItem('targetLanguage', myTargetLanguage);
        if (meetingWs && meetingWs.readyState === WebSocket.OPEN) {
            meetingWs.send(JSON.stringify({
                type: 'update_language',
                targetLanguage: myTargetLanguage
            }));
        }
    });

    // Reconnect button
    document.getElementById('reconnectButton').addEventListener('click', function() {
        document.getElementById('connectionStatus').style.display = 'none';
        connectToMeeting();
    });

    // Download transcript
    document.getElementById('downloadTranscript').addEventListener('click', downloadTranscript);

    const downloadSnapshotBtn = document.getElementById('downloadSnapshot');
    if (downloadSnapshotBtn) {
        downloadSnapshotBtn.addEventListener('click', downloadTranscriptSnapshot);
    }

    const minInput = document.getElementById('minSpeakersInput');
    const maxInput = document.getElementById('maxSpeakersInput');
    const strictnessInput = document.getElementById('diarizationStrictness');
    const strictnessValue = document.getElementById('diarizationStrictnessValue');
    const presetSelect = document.getElementById('diarizationPreset');

    if (minInput && maxInput && strictnessInput && strictnessValue && presetSelect) {
        const updateStrictnessLabel = () => {
            strictnessValue.textContent = parseFloat(strictnessInput.value).toFixed(2);
        };

        updateStrictnessLabel();

        presetSelect.addEventListener('change', () => {
            const preset = diarizationPresets[presetSelect.value];
            if (preset) {
                minInput.value = preset.minSpeakers;
                maxInput.value = preset.maxSpeakers;
                strictnessInput.value = preset.strictness;
                updateStrictnessLabel();
                sessionStorage.setItem('diarizationPreset', presetSelect.value);
                sessionStorage.setItem('diarizationMinSpeakers', minInput.value);
                sessionStorage.setItem('diarizationMaxSpeakers', maxInput.value);
                sessionStorage.setItem('diarizationStrictness', strictnessInput.value);
            }
        });

        minInput.addEventListener('change', () => {
            presetSelect.value = 'custom';
            sessionStorage.setItem('diarizationPreset', 'custom');
            sessionStorage.setItem('diarizationMinSpeakers', minInput.value);
        });

        maxInput.addEventListener('change', () => {
            presetSelect.value = 'custom';
            sessionStorage.setItem('diarizationPreset', 'custom');
            sessionStorage.setItem('diarizationMaxSpeakers', maxInput.value);
        });

        strictnessInput.addEventListener('input', () => {
            updateStrictnessLabel();
            presetSelect.value = 'custom';
            sessionStorage.setItem('diarizationPreset', 'custom');
            sessionStorage.setItem('diarizationStrictness', strictnessInput.value);
        });
    }

    // Event delegation for speaker rename clicks
    document.getElementById('captionsContainer').addEventListener('click', function(e) {
        const speakerLabel = e.target.closest('[data-speaker-id]');
        if (speakerLabel && speakerLabel.dataset.speakerId) {
            const speakerId = speakerLabel.dataset.speakerId;
            const speakerName = speakerLabel.dataset.speakerName || 'Speaker';
            promptRenameSpeaker(speakerId, speakerName);
        }
    });
}

function initializeDiarizationControls() {
    const controls = document.getElementById('diarizationControls');
    if (!controls) {
        return;
    }

    if (meetingMode === 'shared') {
        controls.style.display = 'flex';
    } else {
        controls.style.display = 'none';
        return;
    }

    const minInput = document.getElementById('minSpeakersInput');
    const maxInput = document.getElementById('maxSpeakersInput');
    const strictnessInput = document.getElementById('diarizationStrictness');
    const strictnessValue = document.getElementById('diarizationStrictnessValue');
    const presetSelect = document.getElementById('diarizationPreset');
    if (!minInput || !maxInput || !strictnessInput || !strictnessValue || !presetSelect) {
        return;
    }

    const storedPreset = sessionStorage.getItem('diarizationPreset') || 'balanced';
    const storedMin = sessionStorage.getItem('diarizationMinSpeakers');
    const storedMax = sessionStorage.getItem('diarizationMaxSpeakers');
    const storedStrictness = sessionStorage.getItem('diarizationStrictness');

    if (storedPreset !== 'custom' && diarizationPresets[storedPreset]) {
        const preset = diarizationPresets[storedPreset];
        presetSelect.value = storedPreset;
        minInput.value = preset.minSpeakers;
        maxInput.value = preset.maxSpeakers;
        strictnessInput.value = preset.strictness;
    } else {
        presetSelect.value = 'custom';
        minInput.value = storedMin ?? diarizationDefaults.minSpeakers;
        maxInput.value = storedMax ?? diarizationDefaults.maxSpeakers;
        strictnessInput.value = storedStrictness ?? diarizationDefaults.strictness;
    }
    strictnessValue.textContent = parseFloat(strictnessInput.value).toFixed(2);
}

function getDiarizationQueryParams() {
    if (meetingMode !== 'shared') {
        return '';
    }

    const minInput = document.getElementById('minSpeakersInput');
    const maxInput = document.getElementById('maxSpeakersInput');
    const strictnessInput = document.getElementById('diarizationStrictness');

    if (!minInput || !maxInput || !strictnessInput) {
        return '';
    }

    let minSpeakers = parseInt(minInput.value, 10);
    let maxSpeakers = parseInt(maxInput.value, 10);
    const strictness = parseFloat(strictnessInput.value);

    if (Number.isNaN(minSpeakers) || minSpeakers <= 0) {
        minSpeakers = diarizationDefaults.minSpeakers;
    }

    if (Number.isNaN(maxSpeakers) || maxSpeakers < 0) {
        maxSpeakers = diarizationDefaults.maxSpeakers;
    }

    if (maxSpeakers > 0 && maxSpeakers < minSpeakers) {
        maxSpeakers = minSpeakers;
        maxInput.value = `${maxSpeakers}`;
    }

    const params = new URLSearchParams();
    if (minSpeakers > 0) {
        params.set('minSpeakers', `${minSpeakers}`);
    }
    if (maxSpeakers > 0) {
        params.set('maxSpeakers', `${maxSpeakers}`);
    }
    if (!Number.isNaN(strictness)) {
        params.set('strictness', strictness.toFixed(2));
    }

    return params.toString();
}

async function connectToMeeting() {
    showStatus('Connecting to meeting...');

    try {
        // Request microphone permission
        const stream = await navigator.mediaDevices.getUserMedia({ audio: true });

        // Connect WebSocket
        const diarizationParams = getDiarizationQueryParams();
        const baseParams = `participantId=${myParticipantId}&participantName=${encodeURIComponent(myParticipantName)}&targetLang=${myTargetLanguage}`;
        const wsUrl = diarizationParams
            ? `ws://${window.location.host}/ws/meeting/${meetingId}?${baseParams}&${diarizationParams}`
            : `ws://${window.location.host}/ws/meeting/${meetingId}?${baseParams}`;

        meetingWs = new WebSocket(wsUrl);

        meetingWs.onopen = () => {
            console.log('Connected to meeting');
            isConnected = true;
            hideStatus();
            setupAudioStreaming(stream);
            refreshSnapshotLanguages();
        };

        meetingWs.onmessage = (event) => {
            const message = JSON.parse(event.data);
            handleMeetingMessage(message);
        };

        meetingWs.onerror = (error) => {
            console.error('WebSocket error:', error);
            showStatus('Connection error. Please try reconnecting.', true);
        };

        meetingWs.onclose = () => {
            console.log('Disconnected from meeting');
            isConnected = false;
            showStatus('Disconnected from meeting', true);
            cleanupAudio();
        };

    } catch (error) {
        console.error('Connection error:', error);
        if (error.name === 'NotAllowedError') {
            showStatus('Microphone access denied. Please allow microphone access.', true);
        } else {
            showStatus('Failed to connect to meeting. Please try again.', true);
        }
    }
}

async function refreshSnapshotLanguages() {
    const select = document.getElementById('snapshotLanguage');
    const downloadBtn = document.getElementById('downloadSnapshot');
    if (!select || !downloadBtn) {
        return;
    }

    const meetingKey = roomCode || meetingId;
    try {
        const response = await fetch(`/api/meetings/${meetingKey}/transcript-snapshots`);
        if (!response.ok) {
            select.innerHTML = '<option value="">No snapshots</option>';
            downloadBtn.disabled = true;
            return;
        }

        const data = await response.json();
        const snapshots = data.snapshots || [];
        if (snapshots.length === 0) {
            select.innerHTML = '<option value="">No snapshots</option>';
            downloadBtn.disabled = true;
            return;
        }

        select.innerHTML = snapshots.map(snapshot => (
            `<option value="${snapshot.language}">${snapshot.language}</option>`
        )).join('');
        downloadBtn.disabled = false;
    } catch (error) {
        console.error('Failed to load transcript snapshots:', error);
        select.innerHTML = '<option value="">No snapshots</option>';
        downloadBtn.disabled = true;
    }
}

function setupAudioStreaming(stream) {
    try {
        // Create the AudioContext at the stream's native rate when available.
        const track = stream.getAudioTracks()[0];
        const settings = track && typeof track.getSettings === 'function' ? track.getSettings() : {};
        const streamSampleRate = Number.isFinite(settings.sampleRate) ? settings.sampleRate : null;
        audioContext = streamSampleRate ? new AudioContext({ sampleRate: streamSampleRate }) : new AudioContext();
        const nativeSampleRate = audioContext.sampleRate;
        console.log(`Native sample rate: ${nativeSampleRate}`);

        audioSource = audioContext.createMediaStreamSource(stream);
        audioProcessor = audioContext.createScriptProcessor(4096, 1, 1);

        audioSource.connect(audioProcessor);
        audioProcessor.connect(audioContext.destination);

        audioProcessor.onaudioprocess = (e) => {
            if (isMuted || !meetingWs || meetingWs.readyState !== WebSocket.OPEN) {
                return;
            }

            const inputData = e.inputBuffer.getChannelData(0);

            // Resample to 16kHz before sending
            const resampled = resampleAudio(inputData, nativeSampleRate, 16000);
            const pcm16 = convertToPCM16(resampled);

            // Send binary audio data
            meetingWs.send(pcm16);

            // Update audio level meter using shared utility
            const level = getAudioLevel(inputData);
            document.getElementById('audioLevel').style.width = level + '%';
        };

        console.log('Audio streaming initialized');
    } catch (error) {
        console.error('Error setting up audio:', error);
    }
}

function cleanupAudio() {
    if (audioProcessor) {
        audioProcessor.disconnect();
        audioProcessor = null;
    }
    if (audioSource) {
        audioSource.disconnect();
        audioSource = null;
    }
    if (audioContext) {
        audioContext.close();
        audioContext = null;
    }
}

function handleMeetingMessage(message) {
    console.log('Received message:', message.type);

    switch (message.type) {
        case 'participant_joined':
            addParticipantToUI(message);
            showSystemMessage(`${message.participantName} joined the meeting`);
            break;

        case 'participant_left':
            removeParticipantFromUI(message.participantId);
            showSystemMessage(`${message.participantName} left the meeting`);
            break;

        case 'participant_language_updated':
            updateParticipantLanguageInUI(message.participantId, message.targetLanguage);
            if (message.participantId !== parseInt(myParticipantId)) {
                showSystemMessage(`Participant updated language to ${getLanguageName(message.targetLanguage)}`);
            }
            break;

        case 'transcription':
            // Show translation in MY language
            const myTranslation = message.translations[myTargetLanguage] || message.originalText;
            const isMe = message.speakerParticipantId === parseInt(myParticipantId);
            displayCaption(
                message.speakerName,
                myTranslation,
                isMe,
                message.speakerParticipantId,
                message.speakerId,
                message.speakerLowConfidence,
                message.speakerOverlap
            );
            break;

        case 'speaker_name_updated':
            updateSpeakerNameInUI(message.speakerId, message.speakerName);
            showSystemMessage(`Speaker renamed to: ${message.speakerName}`);
            break;

        case 'error':
            console.error('Server error:', message.error);
            break;
        case 'meeting_ended':
            showStatus('Meeting ended by host.', false);
            cleanupAudio();
            refreshSnapshotLanguages();
            setTimeout(hideStatus, 1500);
            break;

        default:
            console.log('Unknown message type:', message.type);
    }
}

function addParticipantToUI(participant) {
    const list = document.getElementById('participantsList');
    const isMe = participant.participantId === parseInt(myParticipantId);

    // Remove empty state if exists
    const emptyState = list.querySelector('.empty-state');
    if (emptyState) {
        emptyState.remove();
    }

    // Check if participant already exists
    if (document.getElementById(`participant-${participant.participantId}`)) {
        return;
    }

    const div = document.createElement('div');
    div.id = `participant-${participant.participantId}`;
    div.className = 'participant-item';
    div.innerHTML = `
        <span class="participant-icon">🔇</span>
        <div class="participant-info">
            <div class="participant-name">${escapeHtml(participant.participantName)} ${isMe ? '(You)' : ''}</div>
            <div class="participant-lang">${getLanguageName(participant.targetLanguage)}</div>
        </div>
    `;

    list.appendChild(div);
    updateParticipantCount();
}

function updateParticipantLanguageInUI(participantId, targetLanguage) {
    const element = document.getElementById(`participant-${participantId}`);
    if (!element) {
        return;
    }
    const langEl = element.querySelector('.participant-lang');
    if (langEl) {
        langEl.textContent = getLanguageName(targetLanguage);
    }
}

function removeParticipantFromUI(participantId) {
    const element = document.getElementById(`participant-${participantId}`);
    if (element) {
        element.remove();
        updateParticipantCount();
    }

    // Show empty state if no participants
    const list = document.getElementById('participantsList');
    if (list.children.length === 0) {
        list.innerHTML = `
            <div class="empty-state">
                <div class="empty-state-icon">👥</div>
                <p>Waiting for participants...</p>
            </div>
        `;
    }
}

function formatSpeakerLabelText(speakerName, speakerLowConfidence, speakerOverlap) {
    let label = speakerName || 'Speaker';
    if (speakerLowConfidence) {
        label = `${label} · Unknown`;
    }
    if (speakerOverlap) {
        label = `${label} · Overlap`;
    }
    return label;
}

function formatSpeakerLabelTitle(speakerLowConfidence, speakerOverlap) {
    if (speakerLowConfidence && speakerOverlap) {
        return 'Overlap detected; speaker assignment is uncertain';
    }
    if (speakerOverlap) {
        return 'Overlapping speakers detected';
    }
    if (speakerLowConfidence) {
        return 'Low confidence speaker assignment';
    }
    return '';
}

function displayCaption(speakerName, text, isMe, speakerParticipantId, speakerId, speakerLowConfidence, speakerOverlap) {
    const container = document.getElementById('captionsContainer');

    // Remove empty state if exists
    const emptyState = container.querySelector('.empty-state');
    if (emptyState) {
        emptyState.remove();
    }

    const caption = document.createElement('div');
    caption.className = isMe ? 'caption-item caption-me' : 'caption-item';

    // Add speaker label styling for shared room mode
    // Store speaker data in data attributes for event delegation (NO inline onclick!)
    const labelText = formatSpeakerLabelText(speakerName, speakerLowConfidence, speakerOverlap);
    const labelTitle = formatSpeakerLabelTitle(speakerLowConfidence, speakerOverlap);
    const labelAttrs = `data-speaker-overlap="${speakerOverlap ? 'true' : 'false'}" data-speaker-low-confidence="${speakerLowConfidence ? 'true' : 'false'}"`;
    const labelClass = [
        'caption-speaker',
        meetingMode === 'shared' ? 'speaker-diarization' : '',
        speakerLowConfidence ? 'speaker-uncertain' : '',
        speakerOverlap ? 'speaker-overlap' : ''
    ].filter(Boolean).join(' ');

    let speakerLabel;
    if (meetingMode === 'shared' && speakerId) {
        const titleAttr = labelTitle ? `${labelTitle}. Click to rename speaker` : 'Click to rename speaker';
        // FIX: Use data attributes instead of inline onclick - prevents escaping bugs!
        speakerLabel = `<div class="${labelClass}" data-speaker-id="${escapeHtml(speakerId)}" data-speaker-name="${escapeHtml(speakerName)}" ${labelAttrs} title="${titleAttr}">[${escapeHtml(labelText)}] ✏️</div>`;
    } else if (meetingMode === 'shared') {
        const titleAttr = labelTitle ? ` title="${labelTitle}"` : '';
        speakerLabel = `<div class="${labelClass}" ${labelAttrs}${titleAttr}>[${escapeHtml(labelText)}]</div>`;
    } else {
        const titleAttr = labelTitle ? ` title="${labelTitle}"` : '';
        speakerLabel = `<div class="${labelClass}" ${labelAttrs}${titleAttr}>[${escapeHtml(labelText)}]</div>`;
    }

    caption.innerHTML = `
        ${speakerLabel}
        <div class="caption-text">${escapeHtml(text)}</div>
    `;

    container.appendChild(caption);
    container.scrollTop = container.scrollHeight; // Auto-scroll to bottom

    // Show speaking indicator (only for individual mode with participant ID)
    if (speakerParticipantId) {
        showSpeakingIndicator(speakerParticipantId);
    }

    // Keep only last 50 captions for performance
    while (container.children.length > 50) {
        container.removeChild(container.firstChild);
    }
}

function showSystemMessage(text) {
    const container = document.getElementById('captionsContainer');

    const msg = document.createElement('div');
    msg.className = 'system-message';
    msg.textContent = text;

    container.appendChild(msg);
    container.scrollTop = container.scrollHeight;
}

function showSpeakingIndicator(participantId) {
    const element = document.getElementById(`participant-${participantId}`);
    if (element) {
        element.classList.add('speaking');
        element.querySelector('.participant-icon').textContent = '🔊';

        // Remove indicator after 2 seconds
        setTimeout(() => {
            element.classList.remove('speaking');
            element.querySelector('.participant-icon').textContent = '🔇';
        }, 2000);
    }
}

function updateParticipantCount() {
    const list = document.getElementById('participantsList');
    const count = list.querySelectorAll('.participant-item').length;
    document.getElementById('participantCount').textContent = count;
}

function toggleMute() {
    isMuted = !isMuted;
    const button = document.getElementById('muteButton');
    const icon = document.getElementById('muteIcon');
    const text = document.getElementById('muteText');

    if (isMuted) {
        button.classList.remove('active');
        icon.textContent = '🎤';
        text.textContent = 'Unmute';
    } else {
        button.classList.add('active');
        icon.textContent = '🔇';
        text.textContent = 'Mute';
    }
}

function leaveMeeting() {
    if (confirm('Are you sure you want to leave the meeting?')) {
        if (meetingWs) {
            meetingWs.close();
        }
        cleanupAudio();
        sessionStorage.clear();
        window.location.href = '/index.html';
    }
}

function showStatus(message, showReconnect = false) {
    document.getElementById('statusMessage').textContent = message;
    document.getElementById('reconnectButton').style.display = showReconnect ? 'block' : 'none';
    document.getElementById('connectionStatus').style.display = 'flex';
}

function hideStatus() {
    document.getElementById('connectionStatus').style.display = 'none';
}

// Speaker name mapping functions
function promptRenameSpeaker(speakerId, currentName) {
    const newName = prompt(`Rename speaker:\nCurrent: ${currentName}\nEnter new name:`, currentName);
    if (newName && newName.trim() !== '' && newName !== currentName) {
        renameSpeaker(speakerId, newName.trim());
    }
}

async function renameSpeaker(speakerId, newName) {
    try {
        const response = await fetch(`/api/meetings/${roomCode || meetingId}/speakers/${speakerId}`, {
            method: 'POST',
            headers: {
                'Content-Type': 'application/json'
            },
            body: JSON.stringify({
                speakerName: newName
            })
        });

        const data = await response.json();
        if (!data.success) {
            alert('Failed to rename speaker: ' + (data.error || 'Unknown error'));
        }
        // Update will come via WebSocket broadcast
    } catch (error) {
        console.error('Error renaming speaker:', error);
        alert('Failed to rename speaker');
    }
}

async function downloadTranscriptFile(url, defaultFilename) {
    const response = await fetch(url);
    if (!response.ok) {
        return { ok: false, errorText: await response.text() };
    }

    const blob = await response.blob();
    if (!blob || blob.size === 0) {
        return { ok: false, empty: true };
    }

    let filename = defaultFilename;
    const disposition = response.headers.get('Content-Disposition');
    if (disposition) {
        const match = disposition.match(/filename=\"?([^\";]+)\"?/);
        if (match && match[1]) {
            filename = match[1];
        }
    }

    const urlObject = URL.createObjectURL(blob);
    const link = document.createElement('a');
    link.href = urlObject;
    link.download = filename;
    document.body.appendChild(link);
    link.click();
    document.body.removeChild(link);
    URL.revokeObjectURL(urlObject);
    return { ok: true };
}

async function downloadTranscript() {
    try {
        const lang = myTargetLanguage || 'en';
        const meetingKey = roomCode || meetingId;
        const liveUrl = `/api/meetings/${meetingKey}/transcript?lang=${encodeURIComponent(lang)}`;
        const snapshotUrl = `/api/meetings/${meetingKey}/transcript-snapshot?lang=${encodeURIComponent(lang)}`;

        const liveResult = await downloadTranscriptFile(liveUrl, `meeting_${meetingKey}_${lang}.txt`);
        if (liveResult.ok) {
            return;
        }

        const snapshotResult = await downloadTranscriptFile(snapshotUrl, `meeting_${meetingKey}_${lang}_snapshot.txt`);
        if (snapshotResult.ok) {
            return;
        }

        await refreshSnapshotLanguages();
        alert('No live transcript available. Use the Snapshot dropdown to download a stored transcript.');
    } catch (error) {
        console.error('Error downloading transcript:', error);
        alert('Failed to download transcript');
    }
}

async function downloadTranscriptSnapshot() {
    const select = document.getElementById('snapshotLanguage');
    if (!select || !select.value) {
        alert('No transcript snapshot available.');
        return;
    }

    try {
        const meetingKey = roomCode || meetingId;
        const lang = select.value;
        const snapshotUrl = `/api/meetings/${meetingKey}/transcript-snapshot?lang=${encodeURIComponent(lang)}`;
        const result = await downloadTranscriptFile(
            snapshotUrl,
            `meeting_${meetingKey}_${lang}_snapshot.txt`
        );
        if (!result.ok) {
            alert('Failed to download transcript snapshot.');
        }
    } catch (error) {
        console.error('Error downloading transcript snapshot:', error);
        alert('Failed to download transcript snapshot');
    }
}

async function endMeeting() {
    if (isEndingMeeting) {
        return;
    }
    if (!hostToken) {
        alert('Host token missing. Unable to end meeting.');
        return;
    }

    if (!confirm('End the meeting for everyone?')) {
        return;
    }

    const meetingKey = roomCode || meetingId;
    try {
        isEndingMeeting = true;
        const endMeetingButton = document.getElementById('endMeetingButton');
        if (endMeetingButton) {
            endMeetingButton.disabled = true;
            endMeetingButton.textContent = 'Ending...';
        }

        const controller = new AbortController();
        const timeoutId = setTimeout(() => controller.abort(), 10000);

        const response = await fetch(`/api/meetings/${meetingKey}/end`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ hostToken }),
            signal: controller.signal
        });
        clearTimeout(timeoutId);
        if (!response.ok) {
            const errorText = await response.text();
            alert(`Failed to end meeting: ${errorText || response.status}`);
            if (endMeetingButton) {
                endMeetingButton.disabled = false;
                endMeetingButton.textContent = 'End Meeting';
            }
            isEndingMeeting = false;
            return;
        }
        showStatus('Meeting ended. You can download snapshots now.', false);
        if (endMeetingButton) {
            endMeetingButton.disabled = true;
            endMeetingButton.textContent = 'Meeting Ended';
        }
        if (meetingWs && meetingWs.readyState === WebSocket.OPEN) {
            meetingWs.close();
        }
        cleanupAudio();
        refreshSnapshotLanguages();
        setTimeout(hideStatus, 1500);
    } catch (error) {
        console.error('Error ending meeting:', error);
        const message = error && error.name === 'AbortError'
            ? 'End meeting request timed out. Please try again.'
            : 'Failed to end meeting';
        alert(message);
        const endMeetingButton = document.getElementById('endMeetingButton');
        if (endMeetingButton) {
            endMeetingButton.disabled = false;
            endMeetingButton.textContent = 'End Meeting';
        }
    } finally {
        isEndingMeeting = false;
    }
}

function updateSpeakerNameInUI(speakerId, newName) {
    // Update all caption speaker labels with this speaker ID
    const captions = document.querySelectorAll(`[data-speaker-id="${speakerId}"]`);
    captions.forEach(caption => {
        const speakerOverlap = caption.getAttribute('data-speaker-overlap') === 'true';
        const speakerLowConfidence = caption.getAttribute('data-speaker-low-confidence') === 'true';
        const labelText = formatSpeakerLabelText(newName, speakerLowConfidence, speakerOverlap);
        const labelTitle = formatSpeakerLabelTitle(speakerLowConfidence, speakerOverlap);

        // Update data attribute for future clicks
        caption.dataset.speakerName = newName;

        // Update displayed text
        caption.innerHTML = `[${escapeHtml(labelText)}] ✏️`;

        // Update title
        if (labelTitle) {
            caption.setAttribute('title', `${labelTitle}. Click to rename speaker`);
        } else {
            caption.setAttribute('title', 'Click to rename speaker');
        }
    });
}

// Handle page unload
window.addEventListener('beforeunload', function() {
    if (meetingWs && meetingWs.readyState === WebSocket.OPEN) {
        meetingWs.close();
    }
    cleanupAudio();
});
