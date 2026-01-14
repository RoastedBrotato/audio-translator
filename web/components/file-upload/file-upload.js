/**
 * File Upload Widget Component
 * Provides reusable file upload functionality with drag-and-drop support.
 */

/**
 * FileUploadWidget class for handling file uploads
 */
export class FileUploadWidget {
  /**
   * Create a new FileUploadWidget instance
   * @param {object} elements - DOM elements
   * @param {HTMLElement} elements.uploadArea - Upload area element
   * @param {HTMLInputElement} elements.fileInput - Hidden file input element
   * @param {HTMLElement} elements.fileInfo - File info display element
   * @param {HTMLElement} elements.fileName - File name display element
   * @param {HTMLElement} elements.fileSize - File size display element
   * @param {object} options - Configuration options
   * @param {string} options.acceptedType - Accepted file type (e.g., 'audio/', 'video/')
   * @param {number} options.maxSizeMB - Maximum file size in MB (default: 100)
   */
  constructor(elements, options = {}) {
    this.elements = elements;
    this.options = {
      acceptedType: options.acceptedType || 'audio/',
      maxSizeMB: options.maxSizeMB || 100,
      ...options
    };
    this.selectedFile = null;
    this.onFileSelectedCallback = null;
    this.onErrorCallback = null;

    this.init();
  }

  /**
   * Initialize the widget by setting up event listeners
   */
  init() {
    const { uploadArea, fileInput } = this.elements;

    // Click to upload
    uploadArea.addEventListener('click', () => {
      fileInput.click();
    });

    // File selection via input
    fileInput.addEventListener('change', (e) => {
      this.handleFile(e.target.files[0]);
    });

    // Drag and drop events
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
      this.handleFile(e.dataTransfer.files[0]);
    });
  }

  /**
   * Handle file selection
   * @param {File} file - The selected file
   */
  handleFile(file) {
    if (!file) return;

    // Validate file type
    if (!file.type.startsWith(this.options.acceptedType)) {
      const fileType = this.options.acceptedType.replace('/', '');
      this.triggerError(`Please select a valid ${fileType} file`);
      return;
    }

    // Validate file size
    const maxSizeBytes = this.options.maxSizeMB * 1024 * 1024;
    if (file.size > maxSizeBytes) {
      this.triggerError(`File size must be less than ${this.options.maxSizeMB}MB`);
      return;
    }

    // Store the file
    this.selectedFile = file;

    // Display file info
    this.showFileInfo(file);

    // Trigger callback
    if (this.onFileSelectedCallback) {
      this.onFileSelectedCallback(file);
    }
  }

  /**
   * Display file information
   * @param {File} file - The file to display info for
   */
  showFileInfo(file) {
    const { fileInfo, fileName, fileSize } = this.elements;

    if (fileName) {
      fileName.textContent = file.name;
    }

    if (fileSize) {
      const sizeMB = (file.size / (1024 * 1024)).toFixed(2);
      fileSize.textContent = `${sizeMB} MB`;
    }

    if (fileInfo) {
      fileInfo.classList.add('show');
    }
  }

  /**
   * Trigger error callback
   * @param {string} message - Error message
   */
  triggerError(message) {
    console.error('File upload error:', message);
    if (this.onErrorCallback) {
      this.onErrorCallback(message);
    }
  }

  /**
   * Set callback for file selected event
   * @param {Function} callback - Callback function (receives file)
   */
  onFileSelected(callback) {
    this.onFileSelectedCallback = callback;
    return this;
  }

  /**
   * Set callback for error event
   * @param {Function} callback - Callback function (receives error message)
   */
  onError(callback) {
    this.onErrorCallback = callback;
    return this;
  }

  /**
   * Get the selected file
   * @returns {File|null} The selected file or null
   */
  getFile() {
    return this.selectedFile;
  }

  /**
   * Clear the selected file and reset UI
   */
  clear() {
    this.selectedFile = null;
    const { fileInput, fileInfo } = this.elements;

    if (fileInput) {
      fileInput.value = '';
    }

    if (fileInfo) {
      fileInfo.classList.remove('show');
    }
  }

  /**
   * Check if a file is selected
   * @returns {boolean} True if file is selected
   */
  hasFile() {
    return this.selectedFile !== null;
  }
}

/**
 * Factory function to create a file upload widget
 * @param {string|HTMLElement} uploadAreaSelector - Upload area selector or element
 * @param {string|HTMLInputElement} fileInputSelector - File input selector or element
 * @param {object} displayElements - Display elements selectors or elements
 * @param {string|HTMLElement} displayElements.fileInfo - File info container
 * @param {string|HTMLElement} displayElements.fileName - File name element
 * @param {string|HTMLElement} displayElements.fileSize - File size element
 * @param {object} options - Widget options
 * @returns {FileUploadWidget} File upload widget instance
 */
export function createFileUploadWidget(uploadAreaSelector, fileInputSelector, displayElements, options = {}) {
  const getElement = (selector) => {
    if (typeof selector === 'string') {
      return document.querySelector(selector);
    }
    return selector;
  };

  const elements = {
    uploadArea: getElement(uploadAreaSelector),
    fileInput: getElement(fileInputSelector),
    fileInfo: getElement(displayElements.fileInfo),
    fileName: getElement(displayElements.fileName),
    fileSize: getElement(displayElements.fileSize)
  };

  return new FileUploadWidget(elements, options);
}
