class PCMWorkletProcessor extends AudioWorkletProcessor {
  constructor() {
    super();
    this.buffer = [];
    this.bufferSize = 4096; // Send chunks of 4096 samples (~256ms at 16kHz)
    this.processCount = 0;
  }

  process(inputs) {
    const input = inputs[0];
    if (!input || !input[0]) return true;
    
    // Copy channel 0 samples into buffer
    const channel = input[0];
    this.processCount++;
    
    // Log first few calls to verify it's working
    if (this.processCount <= 5) {
      console.log(`PCM Worklet process() call #${this.processCount}, received ${channel.length} samples`);
    }
    
    this.buffer.push(...channel);
    
    // Send when we have enough samples
    if (this.buffer.length >= this.bufferSize) {
      const chunk = new Float32Array(this.buffer.splice(0, this.bufferSize));
      this.port.postMessage(chunk);
    }
    
    return true;
  }
}
registerProcessor("pcm-worklet", PCMWorkletProcessor);
