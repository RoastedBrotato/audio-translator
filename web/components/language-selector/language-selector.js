/**
 * Language Selector Component
 * Provides language data and utilities for creating language selector dropdowns.
 */

// Comprehensive language definitions
export const LANGUAGES = [
  { code: 'auto', name: 'Auto-detect', native: 'Auto-detect', sourceOnly: true },
  { code: 'en', name: 'English', native: 'English' },
  { code: 'ar', name: 'Arabic', native: 'العربية' },
  { code: 'ur', name: 'Urdu', native: 'اردو' },
  { code: 'hi', name: 'Hindi', native: 'हिन्दी' },
  { code: 'ml', name: 'Malayalam', native: 'മലയാളം' },
  { code: 'te', name: 'Telugu', native: 'తెలుగు' },
  { code: 'ta', name: 'Tamil', native: 'தமிழ்' },
  { code: 'bn', name: 'Bengali', native: 'বাংলা' },
  { code: 'fr', name: 'French', native: 'Français' },
  { code: 'es', name: 'Spanish', native: 'Español' },
  { code: 'de', name: 'German', native: 'Deutsch' },
  { code: 'zh', name: 'Chinese', native: '中文' },
  { code: 'ja', name: 'Japanese', native: '日本語' },
  { code: 'ko', name: 'Korean', native: '한국어' }
];

/**
 * Get language by code
 * @param {string} code - Language code (e.g., 'en', 'ar')
 * @returns {object|null} Language object or null if not found
 */
export function getLanguage(code) {
  return LANGUAGES.find(lang => lang.code === code) || null;
}

/**
 * Get formatted language display name
 * @param {string} code - Language code
 * @returns {string} Formatted display name (e.g., "English", "العربية (Arabic)")
 */
export function getLanguageDisplayName(code) {
  const lang = getLanguage(code);
  if (!lang) return code;

  if (lang.code === 'auto') {
    return lang.name;
  }

  return lang.native === lang.name ? lang.name : `${lang.native} (${lang.name})`;
}

/**
 * Create language selector options HTML
 * @param {object} options - Configuration options
 * @param {boolean} options.includeAutoDetect - Include auto-detect option (default: false)
 * @param {string} options.selectedCode - Pre-selected language code
 * @returns {string} HTML string of option elements
 */
export function createLanguageOptions(options = {}) {
  const { includeAutoDetect = false, selectedCode = null } = options;

  let html = '';

  for (const lang of LANGUAGES) {
    // Skip auto-detect if not requested
    if (lang.sourceOnly && !includeAutoDetect) {
      continue;
    }

    const selected = lang.code === selectedCode ? ' selected' : '';
    const displayName = getLanguageDisplayName(lang.code);

    html += `<option value="${lang.code}"${selected}>${displayName}</option>\n      `;
  }

  return html.trim();
}

/**
 * Populate a select element with language options
 * @param {HTMLSelectElement} selectElement - The select element to populate
 * @param {object} options - Configuration options
 * @param {boolean} options.includeAutoDetect - Include auto-detect option (default: false)
 * @param {string} options.selectedCode - Pre-selected language code
 */
export function populateLanguageSelector(selectElement, options = {}) {
  if (!selectElement) {
    console.error('Language selector element not found');
    return;
  }

  const { includeAutoDetect = false, selectedCode = null } = options;

  // Clear existing options
  selectElement.innerHTML = '';

  for (const lang of LANGUAGES) {
    // Skip auto-detect if not requested
    if (lang.sourceOnly && !includeAutoDetect) {
      continue;
    }

    const option = document.createElement('option');
    option.value = lang.code;
    option.textContent = getLanguageDisplayName(lang.code);

    if (lang.code === selectedCode) {
      option.selected = true;
    }

    selectElement.appendChild(option);
  }
}

/**
 * Initialize language selectors on a page
 * @param {object} selectors - Object with selector IDs
 * @param {string} selectors.source - Source language selector ID
 * @param {string} selectors.target - Target language selector ID
 * @param {object} defaults - Default selections
 * @param {string} defaults.source - Default source language code
 * @param {string} defaults.target - Default target language code
 */
export function initializeLanguageSelectors(selectors, defaults = {}) {
  const { source = null, target = null } = selectors;
  const { source: defaultSource = 'auto', target: defaultTarget = 'en' } = defaults;

  if (source) {
    const sourceElement = document.getElementById(source);
    if (sourceElement) {
      populateLanguageSelector(sourceElement, {
        includeAutoDetect: true,
        selectedCode: defaultSource
      });
    }
  }

  if (target) {
    const targetElement = document.getElementById(target);
    if (targetElement) {
      populateLanguageSelector(targetElement, {
        includeAutoDetect: false,
        selectedCode: defaultTarget
      });
    }
  }
}
