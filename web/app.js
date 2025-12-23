const btnStart = document.getElementById("btnStart");
const btnStop = document.getElementById("btnStop");
const statusEl = document.getElementById("status");
const targetLangEl = document.getElementById("targetLang");
const sourceLangEl = document.getElementById("sourceLang");

const finalSrc = document.getElementById("finalSrc");
const partialSrc = document.getElementById("partialSrc");
const finalTr = document.getElementById("finalTr");
const partialTr = document.getElementById("partialTr");

console.log('app.js loaded successfully');
console.log('Elements found:', {btnStart, btnStop, statusEl, targetLangEl, sourceLangEl, finalSrc, partialSrc, finalTr, partialTr});

let ws = null;
let audioCtx = null;
let sourceNode = null;
let workletNode = null;

let lastSampleRate = 48000;

// Very simple linear resampler (good enough for MVP)
function resampleTo16k(float32, inRate) {
  const outRate = 16000;
  if (inRate === outRate) return float32;

  const ratio = inRate / outRate;
  const outLength = Math.floor(float32.length / ratio);
  const out = new Float32Array(outLength);

  for (let i = 0; i < outLength; i++) {
    const x = i * ratio;
    const x0 = Math.floor(x);
    const x1 = Math.min(x0 + 1, float32.length - 1);
    const t = x - x0;
    out[i] = float32[x0] * (1 - t) + float32[x1] * t;
  }
  return out;
}

function floatToInt16PCM(float32) {
  const GAIN = 2.5;  // Balanced gain for moderate voices
  const out = new Int16Array(float32.length);
  for (let i = 0; i < float32.length; i++) {
    let s = Math.max(-1, Math.min(1, float32[i] * GAIN));  // Apply gain with clipping
    out[i] = s < 0 ? Math.round(s * 32768) : Math.round(s * 32767);
  }
  return out;
}

function addLine(container, text) {
  const div = document.createElement("div");
  div.className = "line";
  div.textContent = text;
  container.appendChild(div);
  container.scrollTop = container.scrollHeight;
}

function setStatus(s) {
  statusEl.textContent = s;
}

function connectWS() {
  return new Promise((resolve, reject) => {
    ws = new WebSocket(`ws://${location.host}/ws`);
    ws.binaryType = "arraybuffer";

    ws.onopen = () => resolve();
    ws.onerror = (e) => reject(e);

    ws.onmessage = (evt) => {
      const msg = JSON.parse(evt.data);
      console.log('WebSocket message:', msg);

      if (msg.type === "partial") {
        partialSrc.textContent = msg.text || "";
      } else if (msg.type === "final") {
        partialSrc.textContent = "";
        addLine(finalSrc, msg.text);
      } else if (msg.type === "translation") {
        // match by id if you want; MVP just appends
        addLine(finalTr, msg.text);
      } else if (msg.type === "partial_translation") {
        partialTr.textContent = msg.text || "";
      } else if (msg.type === "info") {
        setStatus(msg.text);
      }
    };

    ws.onclose = () => {
      setStatus("Disconnected");
      ws = null;
    };
  });
}

async function start() {
  console.log('START BUTTON CLICKED!');
  try {
    btnStart.disabled = true;
    btnStop.disabled = false;

    console.log('Connecting to WebSocket...');
    await connectWS();
    setStatus("Connected");
    console.log('WebSocket connected');

    // Start audio
    console.log('Requesting microphone access...');
    const stream = await navigator.mediaDevices.getUserMedia({ audio: true });
    audioCtx = new AudioContext();
    lastSampleRate = audioCtx.sampleRate;
    console.log(`Audio context created, sample rate: ${lastSampleRate}Hz`);

    await audioCtx.audioWorklet.addModule("./pcm-worklet.js");
    console.log('Audio worklet loaded');

    sourceNode = audioCtx.createMediaStreamSource(stream);
    workletNode = new AudioWorkletNode(audioCtx, "pcm-worklet");

    // Control message
    const startMsg = {
      type: "start",
      sampleRate: 16000,
      targetLang: targetLangEl.value,
      sourceLang: sourceLangEl ? sourceLangEl.value : "auto"
    };
    console.log('Sending start message:', startMsg);
    ws.send(JSON.stringify(startMsg));

    let chunkCount = 0;
    workletNode.port.onmessage = (e) => {
      if (!ws || ws.readyState !== WebSocket.OPEN) return;

      const floatChunk = e.data; // Float32Array at device sample rate
      
      // Calculate RMS of original float audio
      let sum = 0;
      for (let i = 0; i < floatChunk.length; i++) {
        sum += floatChunk[i] * floatChunk[i];
      }
      const floatRMS = Math.sqrt(sum / floatChunk.length);
      
      const resampled = resampleTo16k(floatChunk, lastSampleRate);
      const pcm16 = floatToInt16PCM(resampled);
      
      // Calculate RMS of int16 PCM
      let pcmSum = 0;
      for (let i = 0; i < pcm16.length; i++) {
        pcmSum += pcm16[i] * pcm16[i];
      }
      const pcmRMS = Math.sqrt(pcmSum / pcm16.length);

      chunkCount++;
      if (chunkCount % 10 === 0) {
        console.log(`Chunk ${chunkCount}: floatRMS=${floatRMS.toFixed(4)}, pcmRMS=${pcmRMS.toFixed(0)}, samples=${pcm16.length}, max=${Math.max(...pcm16)}, min=${Math.min(...pcm16)}`);
      }

      // Send as binary
      ws.send(pcm16.buffer);
    };

    sourceNode.connect(workletNode);
    // (Do not connect to destination to avoid echo)
    setStatus(`Streaming @ ${lastSampleRate}Hz → 16kHz PCM`);
    console.log('Audio pipeline connected, streaming started');
  } catch (error) {
    console.error('ERROR in start():', error);
    alert('Error: ' + error.message);
    btnStart.disabled = false;
    btnStop.disabled = true;
  }
}

async function stop() {
  btnStart.disabled = false;
  btnStop.disabled = true;

  try {
    if (ws && ws.readyState === WebSocket.OPEN) {
      ws.send(JSON.stringify({ type: "stop" }));
      ws.close();
    }
  } catch {}

  try {
    if (workletNode) workletNode.disconnect();
    if (sourceNode) sourceNode.disconnect();
    if (audioCtx) await audioCtx.close();
  } catch {}

  ws = null; audioCtx = null; sourceNode = null; workletNode = null;
  partialSrc.textContent = "";
  partialTr.textContent = "";
  setStatus("Idle");
}

btnStart.onclick = start;
btnStop.onclick = stop;

console.log('Event handlers attached to buttons');
