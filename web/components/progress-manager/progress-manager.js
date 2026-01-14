/**
 * Progress Manager Component
 * Handles WebSocket-based progress tracking for long-running operations.
 */

// Stage emoji mappings for visual feedback
export const STAGE_EMOJIS = {
  'upload': '📤',
  'saving': '💾',
  'detection': '🔍',
  'extraction': '🎵',
  'transcription': '📝',
  'translation': '🌍',
  'tts': '🔊',
  'processing': '⚙️',
  'complete': '✅'
};

/**
 * ProgressManager class for handling WebSocket progress updates
 */
export class ProgressManager {
  /**
   * Create a new ProgressManager instance
   * @param {string} sessionId - The session ID to track
   * @param {object} elements - DOM elements for UI updates
   * @param {HTMLElement} elements.progressFill - Progress bar fill element
   * @param {HTMLElement} elements.progressText - Progress text element
   * @param {HTMLElement} elements.progressStage - Progress stage indicator element
   * @param {object} options - Configuration options
   * @param {object} options.stageEmojis - Custom stage emoji mappings (optional)
   */
  constructor(sessionId, elements, options = {}) {
    this.sessionId = sessionId;
    this.elements = elements;
    this.stageEmojis = options.stageEmojis || STAGE_EMOJIS;
    this.ws = null;
    this.onCompleteCallback = null;
    this.onErrorCallback = null;
    this.onUpdateCallback = null;
  }

  /**
   * Connect to the progress WebSocket
   * @returns {Promise<void>}
   */
  async connect() {
    return new Promise((resolve, reject) => {
      const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
      const wsUrl = `${protocol}//${window.location.host}/ws/progress/${this.sessionId}`;

      this.ws = new WebSocket(wsUrl);

      this.ws.onopen = () => {
        console.log('Progress WebSocket connected');
        resolve();
      };

      this.ws.onerror = (error) => {
        console.error('WebSocket error:', error);
        reject(error);
        if (this.onErrorCallback) {
          this.onErrorCallback(error);
        }
      };

      this.ws.onmessage = (event) => {
        const update = JSON.parse(event.data);
        console.log('Progress update:', update);
        this.handleUpdate(update);
      };

      this.ws.onclose = () => {
        console.log('Progress WebSocket closed');
      };
    });
  }

  /**
   * Handle progress update from WebSocket
   * @param {object} update - Progress update object
   */
  handleUpdate(update) {
    // Update progress bar
    if (this.elements.progressFill) {
      this.elements.progressFill.style.width = `${update.progress}%`;
    }

    // Update stage indicator with emoji
    if (this.elements.progressStage) {
      const emoji = this.stageEmojis[update.stage] || '⏳';
      this.elements.progressStage.textContent = `${emoji} ${update.stage.toUpperCase()}`;
    }

    // Update progress text
    if (this.elements.progressText) {
      this.elements.progressText.textContent = update.message;
    }

    // Call custom update callback if provided
    if (this.onUpdateCallback) {
      this.onUpdateCallback(update);
    }

    // Check for errors
    if (update.error) {
      this.handleError(update);
      return;
    }

    // Check for completion
    if (update.stage === 'complete') {
      this.handleComplete(update);
    }
  }

  /**
   * Handle error update
   * @param {object} update - Error update object
   */
  handleError(update) {
    const error = new Error(update.message || 'Processing failed');
    console.error('Progress error:', error);

    this.cleanup();

    if (this.onErrorCallback) {
      this.onErrorCallback(error, update);
    }
  }

  /**
   * Handle completion
   * @param {object} update - Completion update object
   */
  handleComplete(update) {
    this.cleanup();

    if (this.onCompleteCallback) {
      this.onCompleteCallback(update);
    }
  }

  /**
   * Set callback for completion event
   * @param {Function} callback - Callback function (receives update object)
   */
  onComplete(callback) {
    this.onCompleteCallback = callback;
    return this;
  }

  /**
   * Set callback for error event
   * @param {Function} callback - Callback function (receives error and update)
   */
  onError(callback) {
    this.onErrorCallback = callback;
    return this;
  }

  /**
   * Set callback for progress update event
   * @param {Function} callback - Callback function (receives update object)
   */
  onUpdate(callback) {
    this.onUpdateCallback = callback;
    return this;
  }

  /**
   * Clean up and close WebSocket connection
   */
  cleanup() {
    if (this.ws) {
      this.ws.close();
      this.ws = null;
    }
  }

  /**
   * Manually close the connection (alias for cleanup)
   */
  close() {
    this.cleanup();
  }
}
