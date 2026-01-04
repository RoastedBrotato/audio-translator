import { convertToPCM16, resampleAudio } from '../../assets/js/audio-processor.js';
import { escapeHtml, downloadBlob, postJsonWithAuth } from '../../assets/js/utils.js';

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

    // Reset UI for new session
    translationsContainer.innerHTML = '';
    finalizedSegments = [];
    currentPartialText = "";
    segmentCount = 0;
    translationCount = 0;
    document.getElementById('segmentCount').textContent = '0';
    document.getElementById('translationCount').textContent = '0';
    downloadSection.style.display = 'none';
    liveCaption.innerHTML = '<span style="opacity: 0.5;">Connecting...</span>';

    // Get microphone access first
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
    
    let audioChunkCount = 0;
    processor.onaudioprocess = (e) => {
      if (isStreaming && ws && ws.readyState === WebSocket.OPEN) {
        const inputData = e.inputBuffer.getChannelData(0);
        const resampledData = resampleAudio(inputData, actualSampleRate, targetSampleRate);
        const pcm16 = convertToPCM16(resampledData);
        ws.send(pcm16);

        // Log every 50 chunks (~1 second)
        audioChunkCount++;
        if (audioChunkCount % 50 === 0) {
          console.log(`Sent ${audioChunkCount} audio chunks, last size: ${pcm16.byteLength} bytes`);
        }
      }
    };
    
    source.connect(processor);
    processor.connect(audioContext.destination);
    
    // Connect to streaming WebSocket (port 8003 for ASR streaming service)
    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    const lang = sourceLang.value === 'auto' ? 'auto' : sourceLang.value;
    ws = new WebSocket(`${protocol}//localhost:8003/stream?language=${lang}&session_id=${sessionId}`);
    
    // Wait for WebSocket to open before starting streaming
    await new Promise((resolve, reject) => {
      ws.onopen = () => {
        console.log('Streaming WebSocket connected');
        resolve();
      };
      
      ws.onerror = (error) => {
        console.error('WebSocket error:', error);
        reject(new Error('Failed to connect to streaming service'));
      };
      
      // Timeout after 5 seconds
      setTimeout(() => reject(new Error('WebSocket connection timeout')), 5000);
    });
    
    ws.onmessage = (event) => {
      const data = JSON.parse(event.data);
      handleStreamingMessage(data);
    };
    
    ws.onclose = () => {
      console.log('WebSocket closed');
      if (isStreaming) {
        alert('Connection lost. Please try again.');
        stopStreaming();
      }
    };
    
    // Update UI - only after WebSocket is connected
    isStreaming = true;
    btnStart.disabled = true;
    btnStop.disabled = false;
    sourceLang.disabled = true;
    targetLang.disabled = true;
    recordingIndicator.innerHTML = '<span class="recording-indicator"></span>';
    liveCaption.innerHTML = '<span style="opacity: 0.5;">🎤 Listening... Speak now!</span>';

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

    // Keep download section hidden until final processing is done
    downloadSection.style.display = 'none';

    // Trigger final high-quality processing
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

// Speaker color mapping
const speakerColors = {
  'SPEAKER_00': { bg: 'var(--speaker-0-bg)', border: 'var(--speaker-0-border)', align: 'left', name: 'Person 1' },
  'SPEAKER_01': { bg: 'var(--speaker-1-bg)', border: 'var(--speaker-1-border)', align: 'right', name: 'Person 2' },
  'SPEAKER_02': { bg: 'var(--speaker-2-bg)', border: 'var(--speaker-2-border)', align: 'left', name: 'Person 3' },
  'SPEAKER_03': { bg: 'var(--speaker-3-bg)', border: 'var(--speaker-3-border)', align: 'right', name: 'Person 4' }
};

// Get speaker style (cycles through colors if more than 4 speakers)
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

function addTranslationToUI(segment) {
  const div = document.createElement('div');

  if (segment.speaker) {
    // Chat bubble style with speaker labels
    const style = getSpeakerStyle(segment.speaker);
    const labelText = formatSpeakerLabelText(style.name, segment.speakerLowConfidence, segment.speakerOverlap);
    const labelClass = [
      'speaker-label',
      segment.speakerLowConfidence ? 'speaker-uncertain' : '',
      segment.speakerOverlap ? 'speaker-overlap' : ''
    ].filter(Boolean).join(' ');

    div.className = `translation-bubble translation-${style.align}`;
    div.innerHTML = `
      <div class="${labelClass}" style="color: ${style.border};">${labelText}</div>
      <div class="bubble-content" style="background: ${style.bg}; border-left: 4px solid ${style.border};">
        <div class="bubble-original">${escapeHtml(segment.original)}</div>
        <div class="bubble-translated">→ ${escapeHtml(segment.translation || 'Translating...')}</div>
      </div>
    `;
  } else {
    // Original style without speaker labels
    div.className = 'translation-item';
    div.innerHTML = `
      <div class="translation-original">${escapeHtml(segment.original)}</div>
      <div class="translation-translated">→ ${escapeHtml(segment.translation || 'Translating...')}</div>
    `;
  }

  translationsContainer.appendChild(div);
  translationsContainer.scrollTop = translationsContainer.scrollHeight;
}

async function triggerFinalProcessing() {
  console.log('🔄 Fetching high-quality final transcription...');
  liveCaption.innerHTML = '<span style="opacity: 0.7;">⏳ Processing final high-quality transcription...</span>';

  // Poll for the final transcription (with timeout)
  const maxAttempts = 60; // Try for up to 2 minutes (60 * 2 seconds)
  const pollInterval = 2000; // Check every 2 seconds
  
  for (let attempt = 1; attempt <= maxAttempts; attempt++) {
    try {
      console.log(`Polling attempt ${attempt}/${maxAttempts}...`);
      liveCaption.innerHTML = `<span style="opacity: 0.7;">⏳ Processing... (${attempt * 2}s)</span>`;
      
      const response = await fetch(`http://localhost:8003/transcription/${sessionId}`);

      if (response.ok) {
        const result = await response.json();
        
        if (result.success && result.data) {
          console.log('✅ Final transcription ready!');
          await displayFinalTranscription(result);
          return;
        }
      }
      
      // Wait before next attempt
      await new Promise(resolve => setTimeout(resolve, pollInterval));
      
    } catch (err) {
      console.warn(`Polling attempt ${attempt} failed:`, err);
      if (attempt === maxAttempts) {
        console.error('Failed to fetch final transcription after max attempts');
        break;
      }
      await new Promise(resolve => setTimeout(resolve, pollInterval));
    }
  }
  
  // Timeout or error - show what we have
  console.warn('Using streaming transcription (final processing timed out or failed)');
  downloadSection.style.display = 'block';
  liveCaption.innerHTML = '<span style="opacity: 0.5;">Processing complete. Using streaming results.</span>';

  await writeStreamingHistory();
}

async function displayFinalTranscription(result) {
  try {
    const data = result.data;

    if (data) {
      console.log('✅ Received high-quality transcription:', data);

      // Update the UI with the high-quality version
      liveCaption.innerHTML = '<span style="color: #10b981;">✓ High-quality processing complete!</span>';

      // Clear existing translations
      translationsContainer.innerHTML = '';
      finalizedSegments = [];
      segmentCount = 0;
      translationCount = 0;

      // Process each segment with high-quality transcription
      for (const segment of data.segments) {
        const index = finalizedSegments.length;
        finalizedSegments.push({
          index: index + 1,
          original: segment.text,
          timestamp: new Date().toISOString(),
          start: segment.start,
          end: segment.end,
          speaker: segment.speaker || null,  // Include speaker info if available
          speakerLowConfidence: segment.speaker_low_confidence || false,
          speakerOverlap: segment.speaker_overlap || false
        });

        segmentCount++;

        // Translate the improved segment
        await translateSegment(segment.text, index);
      }

      // Log speaker info if available
      if (data.num_speakers) {
        console.log(`👥 Detected ${data.num_speakers} speaker(s) in conversation`);
      }

      document.getElementById('segmentCount').textContent = segmentCount;

      // Show success message
      liveCaption.innerHTML = '<span style="color: #10b981;">✓ High-quality transcription and translation complete!</span>';

      // Now show the download section
      downloadSection.style.display = 'block';

      setTimeout(() => {
        liveCaption.innerHTML = '<span style="opacity: 0.5;">All processing complete. Download your transcript below.</span>';
      }, 3000);

      await writeStreamingHistory();

    } else {
      // Fallback - show download with current data
      downloadSection.style.display = 'block';
      liveCaption.innerHTML = '<span style="opacity: 0.5;">Processing complete. See translations below.</span>';
    }
  } catch (err) {
    console.error('Error fetching final transcription:', err);
    // Fallback - show download with current data
    downloadSection.style.display = 'block';
    liveCaption.innerHTML = '<span style="opacity: 0.5;">Processing complete. See translations below.</span>';
  }
}

async function writeStreamingHistory() {
  if (!sessionId) return;

  const finalTranscript = finalizedSegments.map((seg) => seg.original).join(' ').trim();
  const finalTranslation = finalizedSegments
    .map((seg) => seg.translation || seg.original)
    .join(' ')
    .trim();

  await postJsonWithAuth('/api/history/streaming', {
    sessionId: sessionId,
    sourceLang: sourceLang.value,
    targetLang: targetLang.value,
    totalChunks: finalizedSegments.length,
    totalDurationSeconds: startTime ? Math.round((Date.now() - startTime) / 1000) : 0,
    finalTranscript: finalTranscript,
    finalTranslation: finalTranslation
  });
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
  let content = `Live Streaming Transcript (High-Quality Processing)\n`;
  content += `Date: ${new Date().toLocaleString()}\n`;
  content += `Session ID: ${sessionId}\n`;
  content += `Source: ${sourceLang.value} → Target: ${targetLang.value}\n`;
  content += `Total Segments: ${finalizedSegments.length}\n\n`;
  content += `${'='.repeat(60)}\n\n`;

  finalizedSegments.forEach((seg, idx) => {
    content += `Segment ${idx + 1}`;
    if (seg.start !== undefined) {
      content += ` [${formatTime(seg.start)} → ${formatTime(seg.end)}]`;
    }
    content += `:\n`;
    content += `Original: ${seg.original}\n`;
    content += `Translation: ${seg.translation || 'N/A'}\n\n`;
  });

  const blob = new Blob([content], { type: 'text/plain' });
  downloadBlob(blob, `transcript_${sessionId}.txt`);
}

function formatTime(seconds) {
  const mins = Math.floor(seconds / 60);
  const secs = Math.floor(seconds % 60);
  return `${mins}:${secs.toString().padStart(2, '0')}`;
}

function downloadJSON() {
  const data = {
    sessionId: sessionId,
    timestamp: new Date().toISOString(),
    sourceLang: sourceLang.value,
    targetLang: targetLang.value,
    segments: finalizedSegments
  };
  
  const blob = new Blob([JSON.stringify(data, null, 2)], { type: 'application/json' });
  downloadBlob(blob, `transcript_${sessionId}.json`);
}

window.downloadTranscript = downloadTranscript;
window.downloadJSON = downloadJSON;
