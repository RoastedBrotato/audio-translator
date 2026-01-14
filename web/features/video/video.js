import { formatDuration } from '../../assets/js/audio-processor.js';
import { getAccessToken, postJsonWithAuth } from '../../assets/js/utils.js';
import { ProgressManager } from '../../components/progress-manager/progress-manager.js';

// Video upload and processing script
const uploadArea = document.getElementById('uploadArea');
const videoFile = document.getElementById('videoFile');
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
const duration = document.getElementById('duration');
const sourceLang = document.getElementById('sourceLang');
const targetLang = document.getElementById('targetLang');
const errorMessage = document.getElementById('errorMessage');
const generateTTS = document.getElementById('generateTTS');
const cloneVoice = document.getElementById('cloneVoice');
const downloadBtn = document.getElementById('downloadBtn');
const voiceCloneWarning = document.getElementById('voiceCloneWarning');

let selectedFile = null;
let videoPath = null;
let progressWS = null;
let currentSessionId = null;

// Languages supported by voice cloning (XTTS model)
const voiceCloningSupportedLanguages = [
    'en', 'es', 'fr', 'de', 'it', 'pt', 'pl', 'tr', 'ru', 'nl',
    'cs', 'ar', 'zh', 'hu', 'ko', 'ja', 'hi'
];

// Check if voice cloning is supported for selected language
function checkVoiceCloningSupport() {
    const selectedLang = targetLang.value;
    const isSupported = voiceCloningSupportedLanguages.includes(selectedLang);

    if (!isSupported && cloneVoice.checked) {
        voiceCloneWarning.style.display = 'block';
    } else {
        voiceCloneWarning.style.display = 'none';
    }
}

// Listen for target language changes
targetLang.addEventListener('change', checkVoiceCloningSupport);
cloneVoice.addEventListener('change', checkVoiceCloningSupport);

// Click to upload
uploadArea.addEventListener('click', () => {
    videoFile.click();
});

// File selection
videoFile.addEventListener('change', (e) => {
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
    
    // Check if it's a video file
    if (!file.type.startsWith('video/')) {
        showError('Please select a valid video file');
        return;
    }
    
    // Check file size (500MB max)
    if (file.size > 500 * 1024 * 1024) {
        showError('File size must be less than 500MB');
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
    await startUpload(false);
});

async function startUpload(forceProcessing) {
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
    downloadBtn.classList.remove('show');
    
    try {
        // Create form data
        const formData = new FormData();
        formData.append('video', selectedFile);
        formData.append('sourceLang', sourceLang.value);
        formData.append('targetLang', targetLang.value);
        formData.append('generateTTS', generateTTS.checked ? 'true' : 'false');
        formData.append('cloneVoice', cloneVoice.checked ? 'true' : 'false');
        if (forceProcessing) {
            formData.append('force', 'true');
        }
        
        // Upload video and get session ID
        const headers = {};
        const token = getAccessToken();
        if (token) {
            headers.Authorization = `Bearer ${token}`;
        }

        const response = await fetch('/upload', {
            method: 'POST',
            headers,
            body: formData
        });
        
        const data = await response.json();
        
        if (!data.success || !data.sessionId) {
            throw new Error(data.error || 'Failed to start processing');
        }

        currentSessionId = data.sessionId;

        // Create progress manager and connect to WebSocket
        const progressManager = new ProgressManager(data.sessionId, {
            progressFill,
            progressText,
            progressStage
        });

        await progressManager.connect();

        // Store reference for cleanup
        progressWS = progressManager;

        // Handle completion
        progressManager.onComplete(async (update) => {
            // Display results if available
            if (update.results) {
                if (update.results.existing) {
                    const reuse = confirm('We found a previous upload for this file. Use existing result?');
                    if (!reuse) {
                        progressContainer.classList.remove('show');
                        uploadBtn.disabled = false;
                        clearBtn.disabled = false;
                        await startUpload(true);
                        return;
                    }
                    progressContainer.classList.remove('show');
                    uploadBtn.disabled = false;
                    clearBtn.disabled = false;
                    showError('Using existing upload. No new processing was done.');
                    return;
                }
                transcription.textContent = update.results.transcription || 'No transcription available';
                translation.textContent = update.results.translation || 'No translation available';
                duration.textContent = update.results.duration ? formatDuration(update.results.duration) : 'Unknown';

                // Store video path and show download button if TTS was generated
                if (update.results.videoPath) {
                    videoPath = update.results.videoPath;
                    downloadBtn.classList.add('show');
                } else {
                    downloadBtn.classList.remove('show');
                }

                // Show detected language if available
                if (update.results.detectedLang) {
                    console.log('Detected language:', update.results.detectedLang);
                }

                if (!update.results.existing) {
                    await postJsonWithAuth('/api/history/video', {
                    sessionId: currentSessionId,
                    filename: selectedFile ? selectedFile.name : 'upload',
                    transcription: update.results.transcription || '',
                    translation: update.results.translation || '',
                    videoPath: update.results.minioVideoKey || '',
                    audioPath: update.results.minioAudioKey || '',
                    ttsPath: update.results.minioTtsKey || '',
                    sourceLang: sourceLang.value,
                    targetLang: targetLang.value,
                    durationSeconds: update.results.duration ? Math.round(update.results.duration) : 0
                    });
                }
            }

            // Wait a bit then show results
            setTimeout(() => {
                progressContainer.classList.remove('show');
                if (update.results) {
                    results.classList.add('show');
                }

                // Re-enable buttons
                uploadBtn.disabled = false;
                clearBtn.disabled = false;
            }, 500);
        });

        // Handle errors
        progressManager.onError((error, update) => {
            throw error;
        });
        
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
}

// Clear button
clearBtn.addEventListener('click', () => {
    // Close progress WebSocket if open
    if (progressWS) {
        progressWS.close();
        progressWS = null;
    }
    
    selectedFile = null;
    videoFile.value = '';
    fileInfo.classList.remove('show');
    uploadBtn.disabled = true;
    progressContainer.classList.remove('show');
    results.classList.remove('show');
    errorMessage.classList.remove('show');
    progressFill.style.width = '0%';
    progressStage.textContent = '';
    downloadBtn.classList.remove('show');
    videoPath = null;
});

// Download button
downloadBtn.addEventListener('click', () => {
    if (videoPath) {
        window.location.href = `/download/${videoPath}`;
    }
});

// Initialize voice cloning support check on page load
checkVoiceCloningSupport();
