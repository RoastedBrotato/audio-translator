let ws = null;
let mediaStream = null;
let audioContext = null;
let isStreaming = false;
let sessionId = null;
let startTime = null;

let finalizedSegments = [];
let currentPartialText = "";
let segmentCount = 0;
let translationCount = 0;

const btnStart = document.getElementById('btnStart');
const btnStop = document.getElementById('btnStop');
const sourceLang = document.getElementById('sourceLang');
const targetLang = document.getElementById('targetLang');
const liveCaption = document.getElementById('liveCaption');
const translationsContainer = document.getElementById('translationsContainer');
const downloadSection = document.getElementById('downloadSection');
const recordingIndicator = document.getElementById('recordingIndicator');

btnStart.addEventListener('click', startStreaming);
btnStop.addEventListener('click', stopStreaming);

async function startStreaming() {
  try {
    sessionId = 'stream_' + Date.now() + '_' + Math.random().toString(36).substr(2, 9);
    
    // Connect to streaming WebSocket (port 8003 for ASR streaming service)
    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    const lang = sourceLang.value === 'auto' ? 'auto' : sourceLang.value;
    ws = new WebSocket(`${protocol}//localhost:8003/stream?language=${lang}`);
    
    ws.onopen = () => {
      console.log('Streaming WebSocket connected');
    };
    
    ws.onmessage = (event) => {
      const data = JSON.parse(event.data);
      handleStreamingMessage(data);
    };
    
    ws.onerror = (error) => {
      console.error('WebSocket error:', error);
    };
    
    ws.onclose = () => {
      console.log('WebSocket closed');
    };
    
    // Get microphone access
    mediaStream = await navigator.mediaDevices.getUserMedia({ 
      audio: {
        channelCount: 1,
        echoCancellation: true,
        noiseSuppression: true,
        autoGainControl: true
      } 
    });
    
    // Create audio context
    audioContext = new (window.AudioContext || window.webkitAudioContext)();
    const source = audioContext.createMediaStreamSource(mediaStream);
    
    // Resample to 16kHz
    const targetSampleRate = 16000;
    const actualSampleRate = audioContext.sampleRate;
    console.log(`Sample rate: ${actualSampleRate} -> ${targetSampleRate}`);
    
    const processor = audioContext.createScriptProcessor(4096, 1, 1);
    
    processor.onaudioprocess = (e) => {
      if (isStreaming && ws && ws.readyState === WebSocket.OPEN) {
        const inputData = e.inputBuffer.getChannelData(0);
        const resampledData = resample(inputData, actualSampleRate, targetSampleRate);
        const pcm16 = floatTo16BitPCM(resampledData);
        ws.send(pcm16);
      }
    };
    
    source.connect(processor);
    processor.connect(audioContext.destination);
    
    // Update UI
    isStreaming = true;
    btnStart.disabled = true;
    btnStop.disabled = false;
    sourceLang.disabled = true;
    targetLang.disabled = true;
    recordingIndicator.innerHTML = '<span class="recording-indicator"></span>';
    
    // Start duration timer
    startTime = Date.now();
    updateDuration();
    
    console.log('Streaming started');
    
  } catch (err) {
    console.error('Error starting stream:', err);
    alert('Failed to start streaming: ' + err.message);
  }
}

async function stopStreaming() {
  try {
    isStreaming = false;
    
    // Stop audio
    if (mediaStream) {
      mediaStream.getTracks().forEach(track => track.stop());
    }
    if (audioContext) {
      await audioContext.close();
    }
    
    // Close WebSocket
    if (ws) {
      ws.close();
    }
    
    // Update UI
    btnStart.disabled = false;
    btnStop.disabled = true;
    sourceLang.disabled = false;
    targetLang.disabled = false;
    recordingIndicator.innerHTML = '';
    liveCaption.innerHTML = '<span style="opacity: 0.5;">Processing complete. See translations below.</span>';
    
    // Show download options
    downloadSection.style.display = 'block';
    
    // TODO: Trigger final high-quality processing
    await triggerFinalProcessing();
    
    console.log('Streaming stopped');
    
  } catch (err) {
    console.error('Error stopping stream:', err);
    alert('Failed to stop streaming: ' + err.message);
  }
}

function handleStreamingMessage(data) {
  if (data.type === 'partial') {
    // Update live caption with partial text
    currentPartialText = data.text;
    liveCaption.innerHTML = `<span class="partial-text">${escapeHtml(data.text)}</span>`;
    
  } else if (data.type === 'final') {
    // Finalized segment - update caption and translate
    const finalText = data.text;
    liveCaption.innerHTML = `<span class="final-text">${escapeHtml(finalText)}</span>`;
    
    // Add to finalized segments
    finalizedSegments.push({
      index: ++segmentCount,
      original: finalText,
      timestamp: new Date().toISOString()
    });
    
    // Update segment count
    document.getElementById('segmentCount').textContent = segmentCount;
    
    // Translate this segment
    translateSegment(finalText, segmentCount - 1);
    
    // Clear after a moment
    setTimeout(() => {
      liveCaption.innerHTML = '<span style="opacity: 0.5;">Listening...</span>';
    }, 2000);
  }
}

async function translateSegment(text, index) {
  try {
    const response = await fetch('http://localhost:8004/translate', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        text: text,
        source_lang: 'ur',  // Specify Urdu as source
        target_lang: targetLang.value
      })
    });
    
    const data = await response.json();
    const translation = data.translation || data.translated_text || text; // Handle both response formats
    
    // Update the segment with translation
    finalizedSegments[index].translation = translation;
    
    // Add to UI
    addTranslationToUI(finalizedSegments[index]);
    
    // Update translation count
    translationCount++;
    document.getElementById('translationCount').textContent = translationCount;
    
  } catch (err) {
    console.error('Translation error:', err);
    finalizedSegments[index].translation = text; // Fallback
    addTranslationToUI(finalizedSegments[index]);
  }
}

function addTranslationToUI(segment) {
  const div = document.createElement('div');
  div.className = 'translation-item';
  div.innerHTML = `
    <div class="translation-original">${escapeHtml(segment.original)}</div>
    <div class="translation-translated">→ ${escapeHtml(segment.translation || 'Translating...')}</div>
  `;
  translationsContainer.appendChild(div);
  translationsContainer.scrollTop = translationsContainer.scrollHeight;
}

async function triggerFinalProcessing() {
  // TODO: Send full audio for high-quality re-transcription
  console.log('Final processing would happen here');
  console.log(`Collected ${finalizedSegments.length} segments`);
}

function updateDuration() {
  if (!isStreaming) return;
  
  const elapsed = Math.floor((Date.now() - startTime) / 1000);
  const minutes = Math.floor(elapsed / 60);
  const seconds = elapsed % 60;
  document.getElementById('duration').textContent = `${minutes}:${seconds.toString().padStart(2, '0')}`;
  
  setTimeout(updateDuration, 1000);
}

function downloadTranscript() {
  let content = `Live Streaming Transcript\n`;
  content += `Date: ${new Date().toLocaleString()}\n`;
  content += `Source: ${sourceLang.value} → Target: ${targetLang.value}\n`;
  content += `Total Segments: ${finalizedSegments.length}\n\n`;
  content += `${'='.repeat(60)}\n\n`;
  
  finalizedSegments.forEach((seg, idx) => {
    content += `Segment ${idx + 1}:\n`;
    content += `Original: ${seg.original}\n`;
    content += `Translation: ${seg.translation || 'N/A'}\n\n`;
  });
  
  downloadFile(content, `transcript_${sessionId}.txt`, 'text/plain');
}

function downloadJSON() {
  const data = {
    sessionId: sessionId,
    timestamp: new Date().toISOString(),
    sourceLang: sourceLang.value,
    targetLang: targetLang.value,
    segments: finalizedSegments
  };
  
  downloadFile(JSON.stringify(data, null, 2), `transcript_${sessionId}.json`, 'application/json');
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

function resample(audioData, sourceSampleRate, targetSampleRate = 16000) {
  if (sourceSampleRate === targetSampleRate) {
    return audioData;
  }
  
  const ratio = sourceSampleRate / targetSampleRate;
  const newLength = Math.round(audioData.length / ratio);
  const result = new Float32Array(newLength);
  
  for (let i = 0; i < newLength; i++) {
    const srcIndex = i * ratio;
    const srcIndexFloor = Math.floor(srcIndex);
    const srcIndexCeil = Math.min(srcIndexFloor + 1, audioData.length - 1);
    const t = srcIndex - srcIndexFloor;
    result[i] = audioData[srcIndexFloor] * (1 - t) + audioData[srcIndexCeil] * t;
  }
  
  return result;
}

function escapeHtml(text) {
  const div = document.createElement('div');
  div.textContent = text;
  return div.innerHTML;
}
