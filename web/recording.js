// Recording Translation Frontend

let sessionId = null;
let ws = null;
let mediaRecorder = null;
let audioContext = null;
let mediaStream = null;
let isRecording = false;
let transcriptItems = [];
let processedCount = 0;
let totalChunks = 0;

const btnStart = document.getElementById('btnStart');
const btnStop = document.getElementById('btnStop');
const sourceLang = document.getElementById('sourceLang');
const targetLang = document.getElementById('targetLang');
const status = document.getElementById('status');
const chunkCounter = document.getElementById('chunkCounter');
const progressContainer = document.getElementById('progressContainer');
const progressBar = document.getElementById('progressBar');
const transcriptContainer = document.getElementById('transcriptContainer');
const downloadSection = document.getElementById('downloadSection');
const recordingIndicator = document.getElementById('recordingIndicator');
const btnDownloadTxt = document.getElementById('btnDownloadTxt');
const btnDownloadJson = document.getElementById('btnDownloadJson');

btnStart.addEventListener('click', startRecording);
btnStop.addEventListener('click', stopRecording);
btnDownloadTxt.addEventListener('click', () => downloadTranscript('txt'));
btnDownloadJson.addEventListener('click', () => downloadTranscript('json'));

async function startRecording() {
  try {
    // Generate session ID
    sessionId = 'rec_' + Date.now() + '_' + Math.random().toString(36).substr(2, 9);
    
    // Clear previous transcript
    transcriptItems = [];
    processedCount = 0;
    totalChunks = 0;
    transcriptContainer.innerHTML = '<p style="text-align: center; color: #999;">Listening...</p>';
    downloadSection.style.display = 'none';
    progressContainer.style.display = 'none';
    
    // Start recording session on backend
    const response = await fetch('/recording/start', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        sessionId: sessionId,
        sourceLang: sourceLang.value,
        targetLang: targetLang.value
      })
    });
    
    if (!response.ok) {
      throw new Error('Failed to start recording session');
    }
    
    // Connect to WebSocket for live updates
    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    ws = new WebSocket(`${protocol}//${window.location.host}/ws/recording/${sessionId}`);
    
    ws.onopen = () => {
      console.log('WebSocket connected');
    };
    
    ws.onmessage = (event) => {
      const data = JSON.parse(event.data);
      handleWSMessage(data);
    };
    
    ws.onerror = (error) => {
      console.error('WebSocket error:', error);
    };
    
    ws.onclose = () => {
      console.log('WebSocket closed');
    };
    
    // Get microphone access with minimal processing
    mediaStream = await navigator.mediaDevices.getUserMedia({ 
      audio: {
        channelCount: 1,
        echoCancellation: false,
        noiseSuppression: false,
        autoGainControl: false
      } 
    });
    
    // Create audio context for processing (use default sample rate to avoid mismatch)
    audioContext = new (window.AudioContext || window.webkitAudioContext)();
    const source = audioContext.createMediaStreamSource(mediaStream);
    
    // Resample to 16kHz using OfflineAudioContext if needed
    const targetSampleRate = 16000;
    const actualSampleRate = audioContext.sampleRate;
    console.log(`Audio context sample rate: ${actualSampleRate}, target: ${targetSampleRate}`);
    
    const processor = audioContext.createScriptProcessor(4096, 1, 1);
    
    processor.onaudioprocess = (e) => {
      if (isRecording && ws && ws.readyState === WebSocket.OPEN) {
        const inputData = e.inputBuffer.getChannelData(0);
        
        // Resample to 16kHz if needed
        const resampledData = resample(inputData, actualSampleRate, targetSampleRate);
        
        // Convert to PCM16 and send
        const pcm16 = floatTo16BitPCM(resampledData);
        ws.send(pcm16);
      }
    };
    
    source.connect(processor);
    processor.connect(audioContext.destination);
    
    // Update UI
    isRecording = true;
    btnStart.disabled = true;
    btnStop.disabled = false;
    sourceLang.disabled = true;
    targetLang.disabled = true;
    status.textContent = 'Recording...';
    recordingIndicator.innerHTML = '<span class="recording-indicator"></span>';
    
    console.log('Recording started, session:', sessionId);
    
  } catch (err) {
    console.error('Error starting recording:', err);
    alert('Failed to start recording: ' + err.message);
    status.textContent = 'Error';
  }
}

async function stopRecording() {
  try {
    isRecording = false;
    
    // Stop media stream tracks (this removes the microphone indicator)
    if (mediaStream) {
      mediaStream.getTracks().forEach(track => track.stop());
      mediaStream = null;
    }
    
    // Stop audio context
    if (audioContext) {
      audioContext.close();
      audioContext = null;
    }
    
    // DON'T close WebSocket yet - we need it to receive translations!
    // It will be closed when we connect to the progress WebSocket
    
    // Notify backend to stop recording
    const response = await fetch('/recording/stop', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ sessionId: sessionId })
    });
    
    if (!response.ok) {
      throw new Error('Failed to stop recording');
    }
    
    const data = await response.json();
    totalChunks = data.totalChunks || processedCount;
    
    // Now stop audio locally
    isRecording = false;
    if (mediaStream) {
      mediaStream.getTracks().forEach(track => track.stop());
    }
    if (audioContext) {
      await audioContext.close();
    }
    
    // Update UI
    btnStart.disabled = false;
    btnStop.disabled = true;
    sourceLang.disabled = false;
    targetLang.disabled = false;
    status.textContent = 'Processing...';
    recordingIndicator.innerHTML = '';
    
    // Show progress bar
    progressContainer.style.display = 'block';
    updateProgress();
    
    // Recording WebSocket stays open for translations
    // It will be closed by backend when complete
    
    // Connect to progress WebSocket  
    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    const progressWs = new WebSocket(`${protocol}//${window.location.host}/ws/progress/${sessionId}`);
    
    progressWs.onmessage = (event) => {
      const data = JSON.parse(event.data);
      console.log('Progress message received:', data);
      
      // Handle translation via progress WebSocket
      if (data.stage === 'translation' && data.results) {
        // Translation data is in results field
        handleWSMessage(data.results);
      } 
      // Handle progress updates
      else if (data.stage === 'processing' && data.progress !== undefined) {
        // Update progress bar directly with percentage
        progressBar.style.width = Math.round(data.progress) + '%';
        progressBar.textContent = Math.round(data.progress) + '%';
        status.textContent = data.message || 'Processing...';
      } 
      // Handle completion
      else if (data.stage === 'complete' || data.type === 'complete') {
        onProcessingComplete();
        progressWs.close();
        // Close recording WebSocket now
        if (ws && ws.readyState === WebSocket.OPEN) {
          ws.close();
        }
      } 
      // Fallback for direct translation messages
      else if (data.type === 'translation') {
        handleWSMessage(data);
      }
    };
    
    console.log('Recording stopped, processing in background');
    
  } catch (err) {
    console.error('Error stopping recording:', err);
    alert('Failed to stop recording: ' + err.message);
  }
}

function handleWSMessage(data) {
  if (data.type === 'translation') {
    processedCount++;
    
    const item = {
      index: data.index || processedCount,
      original: data.original || '',
      translation: data.translation || '',
      timestamp: data.timestamp || new Date().toISOString()
    };
    
    transcriptItems.push(item);
    addTranscriptItem(item);
    updateChunkCounter();
    
    if (!isRecording) {
      updateProgress();
    }
  } else if (data.type === 'progress') {
    updateProgress(data.current, data.total);
  } else if (data.type === 'complete') {
    console.log('Received completion signal from backend');
    onProcessingComplete();
    // Close recording WebSocket now that processing is done
    if (ws) {
      ws.close();
      ws = null;
      console.log('Recording WebSocket closed after completion');
    }
  } else if (data.type === 'error') {
    console.error('Error from server:', data.message);
    status.textContent = 'Error: ' + data.message;
  }
}

function addTranscriptItem(item) {
  // Remove placeholder if it exists
  if (transcriptContainer.children.length === 1 && 
      transcriptContainer.children[0].tagName === 'P') {
    transcriptContainer.innerHTML = '';
  }
  
  const div = document.createElement('div');
  div.className = 'transcript-item';
  div.innerHTML = `
    <div class="transcript-label">Original #${item.index}</div>
    <div class="transcript-text">${escapeHtml(item.original)}</div>
    <div class="transcript-label" style="margin-top: 10px;">Translation</div>
    <div class="transcript-text">${escapeHtml(item.translation)}</div>
  `;
  
  transcriptContainer.appendChild(div);
  transcriptContainer.scrollTop = transcriptContainer.scrollHeight;
}

function updateChunkCounter() {
  if (isRecording) {
    chunkCounter.textContent = `${processedCount} segments processed`;
  } else if (totalChunks > 0) {
    chunkCounter.textContent = `${processedCount}/${totalChunks} segments`;
  }
}

function updateProgress(current, total) {
  if (!current) current = processedCount;
  if (!total) total = totalChunks;
  
  if (total > 0) {
    const percent = Math.round((current / total) * 100);
    progressBar.style.width = percent + '%';
    progressBar.textContent = percent + '%';
  }
}

function onProcessingComplete() {
  status.textContent = 'Complete ✓';
  progressBar.style.width = '100%';
  progressBar.textContent = '100%';
  downloadSection.style.display = 'block';
  chunkCounter.textContent = `${transcriptItems.length} segments completed`;
}

function downloadTranscript(format) {
  if (format === 'txt') {
    let text = `Recording Translation\n`;
    text += `Session: ${sessionId}\n`;
    text += `Date: ${new Date().toLocaleString()}\n`;
    text += `Source Language: ${sourceLang.value}\n`;
    text += `Target Language: ${targetLang.value}\n`;
    text += `Total Segments: ${transcriptItems.length}\n`;
    text += `\n${'='.repeat(80)}\n\n`;
    
    transcriptItems.forEach((item, idx) => {
      text += `[Segment ${item.index}]\n`;
      text += `Original: ${item.original}\n`;
      text += `Translation: ${item.translation}\n`;
      text += `\n`;
    });
    
    downloadFile(text, `transcript_${sessionId}.txt`, 'text/plain');
    
  } else if (format === 'json') {
    const data = {
      sessionId: sessionId,
      date: new Date().toISOString(),
      sourceLang: sourceLang.value,
      targetLang: targetLang.value,
      totalSegments: transcriptItems.length,
      segments: transcriptItems
    };
    
    downloadFile(JSON.stringify(data, null, 2), `transcript_${sessionId}.json`, 'application/json');
  }
}

function downloadFile(content, filename, mimeType) {
  const blob = new Blob([content], { type: mimeType });
  const url = URL.createObjectURL(blob);
  const a = document.createElement('a');
  a.href = url;
  a.download = filename;
  document.body.appendChild(a);
  a.click();
  document.body.removeChild(a);
  URL.revokeObjectURL(url);
}

function floatTo16BitPCM(float32Array) {
  const int16Array = new Int16Array(float32Array.length);
  for (let i = 0; i < float32Array.length; i++) {
    const s = Math.max(-1, Math.min(1, float32Array[i]));
    int16Array[i] = s < 0 ? s * 0x8000 : s * 0x7FFF;
  }
  return int16Array.buffer;
}

// Simple linear resampling from source sample rate to 16kHz
function resample(audioData, sourceSampleRate, targetSampleRate = 16000) {
  if (sourceSampleRate === targetSampleRate) {
    return audioData;
  }
  
  const ratio = sourceSampleRate / targetSampleRate;
  const newLength = Math.round(audioData.length / ratio);
  const result = new Float32Array(newLength);
  
  for (let i = 0; i < newLength; i++) {
    const sourceIndex = i * ratio;
    const leftIndex = Math.floor(sourceIndex);
    const rightIndex = Math.min(leftIndex + 1, audioData.length - 1);
    const fraction = sourceIndex - leftIndex;
    
    // Linear interpolation
    result[i] = audioData[leftIndex] * (1 - fraction) + audioData[rightIndex] * fraction;
  }
  
  return result;
}

function escapeHtml(text) {
  const div = document.createElement('div');
  div.textContent = text;
  return div.innerHTML;
}
