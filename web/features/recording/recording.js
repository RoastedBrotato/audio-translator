import { formatDuration } from '../../assets/js/audio-processor.js';
import { escapeHtml, downloadBlob } from '../../assets/js/utils.js';

// Audio upload and translation script
const uploadArea = document.getElementById('uploadArea');
const audioFile = document.getElementById('audioFile');
const fileInfo = document.getElementById('fileInfo');
const fileName = document.getElementById('fileName');
const fileSize = document.getElementById('fileSize');
const uploadBtn = document.getElementById('uploadBtn');
const clearBtn = document.getElementById('clearBtn');
const progressContainer = document.getElementById('progressContainer');
const progressFill = document.getElementById('progressFill');
const progressText = document.getElementById('progressText');
const progressStage = document.getElementById('progressStage');
const results = document.getElementById('results');
const transcription = document.getElementById('transcription');
const translation = document.getElementById('translation');
const sourceLang = document.getElementById('sourceLang');
const targetLang = document.getElementById('targetLang');
const errorMessage = document.getElementById('errorMessage');
const downloadTxtBtn = document.getElementById('downloadTxtBtn');
const downloadJsonBtn = document.getElementById('downloadJsonBtn');
const enableDiarization = document.getElementById('enableDiarization');
const enhanceAudio = document.getElementById('enhanceAudio');

let selectedFile = null;
let progressWS = null;
let resultData = null;

// Speaker color mapping (same as streaming)
const speakerColors = {
  'SPEAKER_00': { bg: 'var(--speaker-0-bg)', border: 'var(--speaker-0-border)', align: 'left', name: 'Person 1' },
  'SPEAKER_01': { bg: 'var(--speaker-1-bg)', border: 'var(--speaker-1-border)', align: 'right', name: 'Person 2' },
  'SPEAKER_02': { bg: 'var(--speaker-2-bg)', border: 'var(--speaker-2-border)', align: 'left', name: 'Person 3' },
  'SPEAKER_03': { bg: 'var(--speaker-3-bg)', border: 'var(--speaker-3-border)', align: 'right', name: 'Person 4' }
};

// Get speaker style
function getSpeakerStyle(speaker) {
  const speakerKeys = Object.keys(speakerColors);
  const speakerKey = speakerKeys[parseInt(speaker.split('_')[1] || '0') % speakerKeys.length];
  return speakerColors[speakerKey];
}

function formatSpeakerLabelText(defaultName, speakerLowConfidence, speakerOverlap) {
    let label = defaultName || 'Speaker';
    if (speakerLowConfidence) {
        label = `${label} · Unknown`;
    }
    if (speakerOverlap) {
        label = `${label} · Overlap`;
    }
    return label;
}

// Display segments with speaker bubbles
function displaySegmentsWithSpeakers(segments) {
    // Clear the result sections and convert to bubble display
    const resultsDiv = document.getElementById('results');

    // Remove the standard result sections
    const resultSections = resultsDiv.querySelectorAll('.result-section');
    resultSections.forEach(section => section.style.display = 'none');

    // Create a container for bubbles
    let bubblesContainer = resultsDiv.querySelector('.bubbles-container');
    if (!bubblesContainer) {
        bubblesContainer = document.createElement('div');
        bubblesContainer.className = 'bubbles-container';
        bubblesContainer.style.display = 'flex';
        bubblesContainer.style.flexDirection = 'column';
        bubblesContainer.style.gap = '10px';
        resultsDiv.insertBefore(bubblesContainer, resultsDiv.firstChild);
    } else {
        bubblesContainer.innerHTML = '';
    }

    // Display each segment as a bubble
    segments.forEach(segment => {
        const speaker = segment.speaker || 'SPEAKER_00';
        const style = getSpeakerStyle(speaker);
        const labelText = formatSpeakerLabelText(style.name, segment.speaker_low_confidence, segment.speaker_overlap);
        const labelClass = [
            'speaker-label',
            segment.speaker_low_confidence ? 'speaker-uncertain' : '',
            segment.speaker_overlap ? 'speaker-overlap' : ''
        ].filter(Boolean).join(' ');

        const bubble = document.createElement('div');
        bubble.className = `translation-bubble translation-${style.align}`;
        bubble.innerHTML = `
            <div class="${labelClass}" style="color: ${style.border};">${labelText}</div>
            <div class="bubble-content" style="background: ${style.bg}; border-left: 4px solid ${style.border};">
                <div class="bubble-original">${escapeHtml(segment.text)}</div>
                <div class="bubble-translated">→ ${escapeHtml(segment.translation || 'Translation pending...')}</div>
            </div>
        `;

        bubblesContainer.appendChild(bubble);
    });
}

// Stage emoji mappings
const stageEmojis = {
    'upload': '📤',
    'saving': '💾',
    'detection': '🔍',
    'transcription': '📝',
    'translation': '🌍',
    'processing': '⚙️',
    'complete': '✅'
};

// Click to upload
uploadArea.addEventListener('click', () => {
    audioFile.click();
});

// File selection
audioFile.addEventListener('change', (e) => {
    handleFile(e.target.files[0]);
});

// Drag and drop
uploadArea.addEventListener('dragover', (e) => {
    e.preventDefault();
    uploadArea.classList.add('dragging');
});

uploadArea.addEventListener('dragleave', () => {
    uploadArea.classList.remove('dragging');
});

uploadArea.addEventListener('drop', (e) => {
    e.preventDefault();
    uploadArea.classList.remove('dragging');
    handleFile(e.dataTransfer.files[0]);
});

function handleFile(file) {
    if (!file) return;

    // Check if it's an audio file
    if (!file.type.startsWith('audio/')) {
        showError('Please select a valid audio file');
        return;
    }

    // Check file size (100MB max)
    if (file.size > 100 * 1024 * 1024) {
        showError('File size must be less than 100MB');
        return;
    }

    selectedFile = file;

    // Display file info
    fileName.textContent = file.name;
    fileSize.textContent = `${(file.size / (1024 * 1024)).toFixed(2)} MB`;
    fileInfo.classList.add('show');

    // Enable upload button
    uploadBtn.disabled = false;

    // Hide previous results
    results.classList.remove('show');
    errorMessage.classList.remove('show');
}

function showError(message) {
    errorMessage.textContent = message;
    errorMessage.classList.add('show');
    setTimeout(() => {
        errorMessage.classList.remove('show');
    }, 5000);
}

// Upload button click
uploadBtn.addEventListener('click', async () => {
    if (!selectedFile) return;

    // Disable buttons during processing
    uploadBtn.disabled = true;
    clearBtn.disabled = true;

    // Show progress
    progressContainer.classList.add('show');
    progressFill.style.width = '0%';
    progressText.textContent = 'Initializing...';
    progressStage.textContent = '';

    // Hide previous results and errors
    results.classList.remove('show');
    errorMessage.classList.remove('show');

    try {
        // Create form data
        const formData = new FormData();
        formData.append('audio', selectedFile);
        formData.append('sourceLang', sourceLang.value);
        formData.append('targetLang', targetLang.value);
        formData.append('enableDiarization', enableDiarization.checked ? 'true' : 'false');
        formData.append('enhanceAudio', enhanceAudio.checked ? 'true' : 'false');

        // Upload audio and get session ID
        const response = await fetch('/upload-audio', {
            method: 'POST',
            body: formData
        });

        const data = await response.json();

        if (!data.success || !data.sessionId) {
            throw new Error(data.error || 'Failed to start processing');
        }

        // Connect to progress WebSocket
        const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
        const wsUrl = `${protocol}//${window.location.host}/ws/progress/${data.sessionId}`;
        progressWS = new WebSocket(wsUrl);

        progressWS.onopen = () => {
            console.log('Progress WebSocket connected');
        };

        progressWS.onmessage = (event) => {
            const update = JSON.parse(event.data);
            console.log('Progress update:', update);

            // Update progress bar
            progressFill.style.width = `${update.progress}%`;

            // Update stage indicator
            const emoji = stageEmojis[update.stage] || '⏳';
            progressStage.textContent = `${emoji} ${update.stage.toUpperCase()}`;

            // Update progress text
            progressText.textContent = update.message;

            // Check for errors
            if (update.error) {
                throw new Error(update.message || 'Processing failed');
            }

            // Check for completion
            if (update.stage === 'complete') {
                progressWS.close();
                progressWS = null;

                // Store results
                if (update.results) {
                    resultData = {
                        transcription: update.results.transcription || 'No transcription available',
                        translation: update.results.translation || 'No translation available',
                        detectedLang: update.results.detectedLang,
                        duration: update.results.duration,
                        sourceLang: sourceLang.value,
                        targetLang: targetLang.value,
                        fileName: selectedFile.name,
                        segments: update.results.segments || null,
                        numSpeakers: update.results.num_speakers || 0
                    };

                    // Display results
                    if (resultData.segments && resultData.segments.length > 0 && resultData.numSpeakers > 1) {
                        // Display with speaker bubbles
                        displaySegmentsWithSpeakers(resultData.segments);
                    } else {
                        // Simple text display
                        transcription.textContent = resultData.transcription;
                        translation.textContent = resultData.translation;
                    }

                    // Log detected language and speakers
                    if (update.results.detectedLang) {
                        console.log('Detected language:', update.results.detectedLang);
                    }
                    if (update.results.num_speakers) {
                        console.log(`👥 Detected ${update.results.num_speakers} speaker(s)`);
                    }
                }

                // Wait a bit then show results
                setTimeout(() => {
                    progressContainer.classList.remove('show');
                    if (resultData) {
                        results.classList.add('show');
                    }

                    // Re-enable buttons
                    uploadBtn.disabled = false;
                    clearBtn.disabled = false;
                }, 500);
            }
        };

        progressWS.onerror = (error) => {
            console.error('WebSocket error:', error);
            if (progressWS) {
                progressWS.close();
                progressWS = null;
            }
        };

        progressWS.onclose = () => {
            console.log('Progress WebSocket closed');
        };

    } catch (error) {
        console.error('Error:', error);
        progressContainer.classList.remove('show');
        showError(error.message);
        uploadBtn.disabled = false;
        clearBtn.disabled = false;

        if (progressWS) {
            progressWS.close();
            progressWS = null;
        }
    }
});

// Clear button
clearBtn.addEventListener('click', () => {
    // Close progress WebSocket if open
    if (progressWS) {
        progressWS.close();
        progressWS = null;
    }

    selectedFile = null;
    resultData = null;
    audioFile.value = '';
    fileInfo.classList.remove('show');
    uploadBtn.disabled = true;
    progressContainer.classList.remove('show');
    results.classList.remove('show');
    errorMessage.classList.remove('show');
    progressFill.style.width = '0%';
    progressStage.textContent = '';
});

// Download as text
downloadTxtBtn.addEventListener('click', () => {
    if (!resultData) return;

    let text = `Audio Translation\n`;
    text += `File: ${resultData.fileName}\n`;
    text += `Date: ${new Date().toLocaleString()}\n`;
    text += `Source Language: ${resultData.sourceLang}`;
    if (resultData.detectedLang && resultData.sourceLang === 'auto') {
        text += ` (detected: ${resultData.detectedLang})`;
    }
    text += `\n`;
    if (resultData.duration) {
        text += `Duration: ${formatDuration(resultData.duration)}\n`;
    }
    text += `Target Language: ${resultData.targetLang}\n`;
    if (resultData.numSpeakers > 1) {
        text += `Speakers Detected: ${resultData.numSpeakers}\n`;
    }
    text += `\n${'='.repeat(80)}\n\n`;

    // If diarization was used, format as conversation
    if (resultData.segments && resultData.segments.length > 0 && resultData.numSpeakers > 1) {
        text += `Original Conversation:\n\n`;
        resultData.segments.forEach((segment, idx) => {
            const speaker = segment.speaker || 'SPEAKER_00';
            const style = getSpeakerStyle(speaker);
            const labelText = formatSpeakerLabelText(style.name, segment.speaker_low_confidence, segment.speaker_overlap);
            text += `${labelText}: ${segment.text}\n`;
        });
        text += `\n${'='.repeat(80)}\n\n`;

        text += `Translated Conversation:\n\n`;
        resultData.segments.forEach((segment, idx) => {
            const speaker = segment.speaker || 'SPEAKER_00';
            const style = getSpeakerStyle(speaker);
            const translation = segment.translation || segment.text;
            const labelText = formatSpeakerLabelText(style.name, segment.speaker_low_confidence, segment.speaker_overlap);
            text += `${labelText}: ${translation}\n`;
        });
    } else {
        // Standard format for single speaker or non-diarized
        text += `Original Transcription:\n`;
        text += `${resultData.transcription}\n\n`;
        text += `${'='.repeat(80)}\n\n`;

        text += `Translation:\n`;
        text += `${resultData.translation}\n`;
    }

    downloadBlob(new Blob([text], { type: 'text/plain' }), `transcript_${Date.now()}.txt`);
});

// Download as JSON
downloadJsonBtn.addEventListener('click', () => {
    if (!resultData) return;

    const jsonData = {
        fileName: resultData.fileName,
        date: new Date().toISOString(),
        sourceLang: resultData.sourceLang,
        detectedLang: resultData.detectedLang,
        targetLang: resultData.targetLang,
        transcription: resultData.transcription,
        translation: resultData.translation
    };
    if (resultData.duration) {
        jsonData.duration = resultData.duration;
    }

    // Include speaker segments if available
    if (resultData.segments && resultData.segments.length > 0) {
        jsonData.numSpeakers = resultData.numSpeakers;
        jsonData.segments = resultData.segments.map(segment => ({
            speaker: segment.speaker,
            speakerName: formatSpeakerLabelText(getSpeakerStyle(segment.speaker).name, segment.speaker_low_confidence, segment.speaker_overlap),
            text: segment.text,
            translation: segment.translation || segment.text,
            speakerLowConfidence: segment.speaker_low_confidence || false,
            speakerOverlap: segment.speaker_overlap || false
        }));
    }

    downloadBlob(
        new Blob([JSON.stringify(jsonData, null, 2)], { type: 'application/json' }),
        `transcript_${Date.now()}.json`
    );
});
