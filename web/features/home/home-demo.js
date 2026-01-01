const demoPanel = document.getElementById('demoPanel');
const demoToggle = document.getElementById('demoToggle');
const demoStatusText = document.getElementById('demoStatusText');
const demoLive = document.getElementById('demoLive');
const demoLines = document.getElementById('demoLines');

let demoWs = null;
let demoMediaStream = null;
let demoAudioContext = null;
let demoProcessor = null;
let isDemoRunning = false;
let demoSessionId = null;

function setDemoStatus(status, liveText) {
  demoStatusText.textContent = status;
  if (liveText !== undefined) {
    demoLive.textContent = liveText;
  }
}

function setDemoRunning(running) {
  isDemoRunning = running;
  demoPanel.classList.toggle('is-listening', running);
  demoToggle.textContent = running ? 'Stop demo' : 'Start demo';
}

function appendDemoLine(text) {
  if (!text) return;
  const line = document.createElement('div');
  line.className = 'demo-line';
  line.textContent = text;
  demoLines.appendChild(line);
  while (demoLines.children.length > 4) {
    demoLines.removeChild(demoLines.firstChild);
  }
}

function floatTo16BitPCM(float32Array) {
  const buffer = new ArrayBuffer(float32Array.length * 2);
  const view = new DataView(buffer);
  for (let i = 0; i < float32Array.length; i += 1) {
    let s = Math.max(-1, Math.min(1, float32Array[i]));
    s = s < 0 ? s * 0x8000 : s * 0x7fff;
    view.setInt16(i * 2, s, true);
  }
  return buffer;
}

function resample(audioBuffer, inputSampleRate, targetSampleRate) {
  if (inputSampleRate === targetSampleRate) {
    return audioBuffer;
  }
  const ratio = inputSampleRate / targetSampleRate;
  const newLength = Math.round(audioBuffer.length / ratio);
  const resampled = new Float32Array(newLength);
  for (let i = 0; i < newLength; i += 1) {
    const origin = i * ratio;
    const left = Math.floor(origin);
    const right = Math.min(Math.ceil(origin), audioBuffer.length - 1);
    const weight = origin - left;
    resampled[i] = audioBuffer[left] * (1 - weight) + audioBuffer[right] * weight;
  }
  return resampled;
}

async function startDemo() {
  if (isDemoRunning) return;
  demoSessionId = `home_demo_${Date.now()}`;
  demoLines.innerHTML = '';
  setDemoRunning(true);
  setDemoStatus('Connecting...', 'Connecting to ASR...');

  try {
    demoMediaStream = await navigator.mediaDevices.getUserMedia({
      audio: {
        channelCount: 1,
        echoCancellation: true,
        noiseSuppression: true,
        autoGainControl: true
      }
    });

    demoAudioContext = new (window.AudioContext || window.webkitAudioContext)();
    const source = demoAudioContext.createMediaStreamSource(demoMediaStream);
    demoProcessor = demoAudioContext.createScriptProcessor(4096, 1, 1);
    const targetSampleRate = 16000;
    const actualSampleRate = demoAudioContext.sampleRate;

    demoProcessor.onaudioprocess = (event) => {
      if (!isDemoRunning || !demoWs || demoWs.readyState !== WebSocket.OPEN) {
        return;
      }
      const inputData = event.inputBuffer.getChannelData(0);
      const resampledData = resample(inputData, actualSampleRate, targetSampleRate);
      const pcm16 = floatTo16BitPCM(resampledData);
      demoWs.send(pcm16);
    };

    source.connect(demoProcessor);
    demoProcessor.connect(demoAudioContext.destination);

    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    const asrHost = window.location.hostname || 'localhost';
    demoWs = new WebSocket(`${protocol}//${asrHost}:8003/stream?language=auto&session_id=${demoSessionId}`);

    demoWs.onopen = () => {
      setDemoStatus('Listening...', 'Listening for speech...');
    };

    demoWs.onmessage = (event) => {
      const data = JSON.parse(event.data);
      if (data.type === 'partial') {
        setDemoStatus('Listening...', data.text || 'Listening for speech...');
      } else if (data.type === 'final') {
        appendDemoLine(data.text || '');
        setDemoStatus('Listening...', 'Listening for speech...');
      }
    };

    demoWs.onerror = () => {
      setDemoStatus('Connection error', 'Unable to connect to ASR.');
    };

    demoWs.onclose = () => {
      if (isDemoRunning) {
        setDemoStatus('Disconnected', 'Connection closed.');
        stopDemo();
      }
    };
  } catch (error) {
    setDemoRunning(false);
    setDemoStatus('Microphone blocked', 'Please allow microphone access.');
  }
}

function stopDemo() {
  if (!isDemoRunning) return;
  setDemoRunning(false);
  setDemoStatus('Ready to listen', 'Waiting for audio...');

  if (demoProcessor) {
    demoProcessor.disconnect();
    demoProcessor = null;
  }
  if (demoAudioContext) {
    demoAudioContext.close();
    demoAudioContext = null;
  }
  if (demoMediaStream) {
    demoMediaStream.getTracks().forEach((track) => track.stop());
    demoMediaStream = null;
  }
  if (demoWs) {
    demoWs.close();
    demoWs = null;
  }
}

if (demoPanel && demoToggle && demoStatusText && demoLive && demoLines) {
  demoToggle.addEventListener('click', () => {
    if (isDemoRunning) {
      stopDemo();
    } else {
      startDemo();
    }
  });
}
