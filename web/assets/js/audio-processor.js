/**
 * Shared Audio Processing Utilities
 * Used by streaming, recording, and meeting features
 */

/**
 * Convert Float32Array to PCM16 (Int16Array as ArrayBuffer)
 */
export function convertToPCM16(float32Array) {
    const buffer = new ArrayBuffer(float32Array.length * 2);
    const view = new DataView(buffer);

    for (let i = 0; i < float32Array.length; i++) {
        const s = Math.max(-1, Math.min(1, float32Array[i]));
        const val = s < 0 ? s * 0x8000 : s * 0x7FFF;
        view.setInt16(i * 2, val, true); // true = little endian
    }

    return buffer;
}

/**
 * Resample Float32Array audio data to a target sample rate.
 */
export function resampleAudio(audioData, sourceSampleRate, targetSampleRate = 16000) {
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

/**
 * Convert int16 samples to WAV file format
 */
export function samplesToWAV(samples, sampleRate) {
    const buffer = new ArrayBuffer(44 + samples.length * 2);
    const view = new DataView(buffer);

    // RIFF header
    const writeString = (offset, string) => {
        for (let i = 0; i < string.length; i++) {
            view.setUint8(offset + i, string.charCodeAt(i));
        }
    };

    writeString(0, 'RIFF');
    view.setUint32(4, 36 + samples.length * 2, true);
    writeString(8, 'WAVE');
    writeString(12, 'fmt ');
    view.setUint32(16, 16, true);
    view.setUint16(20, 1, true); // PCM
    view.setUint16(22, 1, true); // Mono
    view.setUint32(24, sampleRate, true);
    view.setUint32(28, sampleRate * 2, true); // Byte rate
    view.setUint16(32, 2, true); // Block align
    view.setUint16(34, 16, true); // Bits per sample
    writeString(36, 'data');
    view.setUint32(40, samples.length * 2, true);

    // Write samples
    let offset = 44;
    for (let i = 0; i < samples.length; i++) {
        view.setInt16(offset, samples[i], true);
        offset += 2;
    }

    return buffer;
}

/**
 * Voice Activity Detection - check if audio has speech
 */
export function hasVoiceActivity(samples, threshold = 0.01) {
    if (!samples || samples.length === 0) return false;

    // Calculate RMS energy
    let sum = 0;
    for (let i = 0; i < samples.length; i++) {
        const normalized = samples[i] / 32768.0;
        sum += normalized * normalized;
    }
    const rms = Math.sqrt(sum / samples.length);

    return rms > threshold;
}

/**
 * Calculate audio level for visual feedback (0-100)
 */
export function getAudioLevel(samples) {
    if (!samples || samples.length === 0) return 0;

    const rms = Math.sqrt(
        samples.reduce((sum, val) => sum + val * val, 0) / samples.length
    );
    return Math.min(100, Math.floor(rms * 1000));
}

/**
 * Format seconds into mm:ss or h:mm:ss.
 */
export function formatDuration(seconds) {
    if (!Number.isFinite(seconds)) return '0:00';
    const totalSeconds = Math.max(0, Math.floor(seconds));
    const hours = Math.floor(totalSeconds / 3600);
    const minutes = Math.floor((totalSeconds % 3600) / 60);
    const secs = totalSeconds % 60;

    if (hours > 0) {
        return `${hours}:${minutes.toString().padStart(2, '0')}:${secs.toString().padStart(2, '0')}`;
    }
    return `${minutes}:${secs.toString().padStart(2, '0')}`;
}
