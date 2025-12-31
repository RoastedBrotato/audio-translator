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

// Track speaking participants
const speakingParticipants = new Set();

// Initialize on page load
document.addEventListener('DOMContentLoaded', function() {
    // Get session data
    myParticipantId = sessionStorage.getItem('participantId');
    myParticipantName = sessionStorage.getItem('participantName');
    myTargetLanguage = sessionStorage.getItem('targetLanguage');
    meetingId = sessionStorage.getItem('meetingId');
    roomCode = sessionStorage.getItem('roomCode');

    // Check if session data exists
    if (!myParticipantId || !myParticipantName || !myTargetLanguage || !meetingId) {
        alert('Missing session data. Please join the meeting again.');
        window.location.href = 'meeting-join.html';
        return;
    }

    // Display room code
    document.getElementById('roomCode').textContent = roomCode || meetingId;

    // Set language selector
    document.getElementById('languageChange').value = myTargetLanguage;

    // Setup event listeners
    setupEventListeners();

    // Connect to meeting
    connectToMeeting();
});

function setupEventListeners() {
    // Mute/Unmute button
    document.getElementById('muteButton').addEventListener('click', toggleMute);

    // Leave meeting button
    document.getElementById('leaveButton').addEventListener('click', leaveMeeting);

    // Language change
    document.getElementById('languageChange').addEventListener('change', function(e) {
        myTargetLanguage = e.target.value;
        sessionStorage.setItem('targetLanguage', myTargetLanguage);
        // TODO: Send language update to server
    });

    // Reconnect button
    document.getElementById('reconnectButton').addEventListener('click', function() {
        document.getElementById('connectionStatus').style.display = 'none';
        connectToMeeting();
    });
}

async function connectToMeeting() {
    showStatus('Connecting to meeting...');

    try {
        // Request microphone permission
        const stream = await navigator.mediaDevices.getUserMedia({ audio: true });

        // Connect WebSocket
        const wsUrl = `ws://${window.location.host}/ws/meeting/${meetingId}?participantId=${myParticipantId}&participantName=${encodeURIComponent(myParticipantName)}&targetLang=${myTargetLanguage}`;

        meetingWs = new WebSocket(wsUrl);

        meetingWs.onopen = () => {
            console.log('Connected to meeting');
            isConnected = true;
            hideStatus();
            setupAudioStreaming(stream);
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

function setupAudioStreaming(stream) {
    try {
        audioContext = new AudioContext({ sampleRate: 16000 });
        audioSource = audioContext.createMediaStreamSource(stream);
        audioProcessor = audioContext.createScriptProcessor(4096, 1, 1);

        audioSource.connect(audioProcessor);
        audioProcessor.connect(audioContext.destination);

        audioProcessor.onaudioprocess = (e) => {
            if (isMuted || !meetingWs || meetingWs.readyState !== WebSocket.OPEN) {
                return;
            }

            const inputData = e.inputBuffer.getChannelData(0);
            const pcm16 = convertToPCM16(inputData);

            // Send binary audio data
            meetingWs.send(pcm16);

            // Update audio level meter
            updateAudioMeter(inputData);
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

        case 'transcription':
            // Show translation in MY language
            const myTranslation = message.translations[myTargetLanguage] || message.originalText;
            const isMe = message.speakerParticipantId === parseInt(myParticipantId);
            displayCaption(message.speakerName, myTranslation, isMe, message.speakerParticipantId);
            break;

        case 'error':
            console.error('Server error:', message.error);
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
            <div class="participant-name">${participant.participantName} ${isMe ? '(You)' : ''}</div>
            <div class="participant-lang">${getLanguageName(participant.targetLanguage)}</div>
        </div>
    `;

    list.appendChild(div);
    updateParticipantCount();
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

function displayCaption(speakerName, text, isMe, speakerParticipantId) {
    const container = document.getElementById('captionsContainer');

    // Remove empty state if exists
    const emptyState = container.querySelector('.empty-state');
    if (emptyState) {
        emptyState.remove();
    }

    const caption = document.createElement('div');
    caption.className = isMe ? 'caption-item caption-me' : 'caption-item';
    caption.innerHTML = `
        <div class="caption-speaker">[${speakerName}]</div>
        <div class="caption-text">${text}</div>
    `;

    container.appendChild(caption);
    container.scrollTop = container.scrollHeight; // Auto-scroll to bottom

    // Show speaking indicator
    showSpeakingIndicator(speakerParticipantId);

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

function updateAudioMeter(audioData) {
    // Calculate RMS for visual feedback
    const rms = Math.sqrt(audioData.reduce((sum, val) => sum + val * val, 0) / audioData.length);
    const level = Math.min(100, Math.floor(rms * 1000));
    document.getElementById('audioLevel').style.width = level + '%';
}

function leaveMeeting() {
    if (confirm('Are you sure you want to leave the meeting?')) {
        if (meetingWs) {
            meetingWs.close();
        }
        cleanupAudio();
        sessionStorage.clear();
        window.location.href = 'index.html';
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

function getLanguageName(code) {
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

// Convert Float32Array to PCM16 (Int16Array as ArrayBuffer)
function convertToPCM16(float32Array) {
    const buffer = new ArrayBuffer(float32Array.length * 2);
    const view = new DataView(buffer);

    for (let i = 0; i < float32Array.length; i++) {
        const s = Math.max(-1, Math.min(1, float32Array[i]));
        const val = s < 0 ? s * 0x8000 : s * 0x7FFF;
        view.setInt16(i * 2, val, true); // true = little endian
    }

    return buffer;
}

// Handle page unload
window.addEventListener('beforeunload', function() {
    if (meetingWs && meetingWs.readyState === WebSocket.OPEN) {
        meetingWs.close();
    }
    cleanupAudio();
});
