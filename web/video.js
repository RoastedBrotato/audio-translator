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

let selectedFile = null;
let videoPath = null;

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
    
    // Disable buttons during processing
    uploadBtn.disabled = true;
    clearBtn.disabled = true;
    
    // Show progress
    progressContainer.classList.add('show');
    progressFill.style.width = '30%';
    progressText.textContent = 'Uploading video...';
    
    // Hide previous results and errors
    results.classList.remove('show');
    errorMessage.classList.remove('show');
    
    try {
        // Create form data
        const formData = new FormData();
        formData.append('video', selectedFile);
        formData.append('sourceLang', sourceLang.value);
        formData.append('targetLang', targetLang.value);
        formData.append('generateTTS', generateTTS.checked ? 'true' : 'false');
        formData.append('cloneVoice', cloneVoice.checked ? 'true' : 'false');
        
        // Update progress
        progressFill.style.width = '50%';
        progressText.textContent = 'Extracting and transcribing audio...';
        
        // If TTS is enabled, update progress text
        if (generateTTS.checked) {
            if (cloneVoice.checked) {
                progressText.textContent = 'Processing with voice cloning (this may take a minute)...';
            } else {
                progressText.textContent = 'Extracting audio, transcribing, and generating translation audio...';
            }
        }
        
        // Upload video
        const response = await fetch('/upload', {
            method: 'POST',
            body: formData
        });
        
        const data = await response.json();
        
        if (!data.success) {
            throw new Error(data.error || 'Failed to process video');
        }
        
        // Update progress
        progressFill.style.width = '100%';
        progressText.textContent = 'Complete!';
        
        // Display results
        transcription.textContent = data.transcription || 'No transcription available';
        translation.textContent = data.translation || 'No translation available';
        duration.textContent = data.duration ? `${data.duration.toFixed(2)} seconds` : 'Unknown';
        
        // Store video path and show download button if TTS was generated
        if (data.videoPath) {
            videoPath = data.videoPath;
            downloadBtn.classList.add('show');
        } else {
            downloadBtn.classList.remove('show');
        }
        
        // Show results
        setTimeout(() => {
            progressContainer.classList.remove('show');
            results.classList.add('show');
        }, 500);
        
    } catch (error) {
        console.error('Error:', error);
        progressContainer.classList.remove('show');
        showError(error.message);
    } finally {
        // Re-enable buttons
        uploadBtn.disabled = false;
        clearBtn.disabled = false;
    }
});

// Clear button
clearBtn.addEventListener('click', () => {
    selectedFile = null;
    videoFile.value = '';
    fileInfo.classList.remove('show');
    uploadBtn.disabled = true;
    progressContainer.classList.remove('show');
    results.classList.remove('show');
    errorMessage.classList.remove('show');
    progressFill.style.width = '0%';
    downloadBtn.classList.remove('show');
    videoPath = null;
});

// Download button
downloadBtn.addEventListener('click', () => {
    if (videoPath) {
        window.location.href = `/download/${videoPath}`;
    }
});
