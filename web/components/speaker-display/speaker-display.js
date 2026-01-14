/**
 * Speaker Display Component
 * Handles speaker color mapping, styling, and label formatting for conversation bubbles.
 */

// Speaker color mapping with chat bubble alignment
export const SPEAKER_COLORS = {
  'SPEAKER_00': { bg: 'var(--speaker-0-bg)', border: 'var(--speaker-0-border)', align: 'left', name: 'Person 1' },
  'SPEAKER_01': { bg: 'var(--speaker-1-bg)', border: 'var(--speaker-1-border)', align: 'right', name: 'Person 2' },
  'SPEAKER_02': { bg: 'var(--speaker-2-bg)', border: 'var(--speaker-2-border)', align: 'left', name: 'Person 3' },
  'SPEAKER_03': { bg: 'var(--speaker-3-bg)', border: 'var(--speaker-3-border)', align: 'right', name: 'Person 4' }
};

/**
 * Get speaker style (cycles through colors if more than 4 speakers)
 * @param {string} speaker - Speaker ID (e.g., 'SPEAKER_00', 'SPEAKER_01')
 * @returns {object} Speaker style object with bg, border, align, and name properties
 */
export function getSpeakerStyle(speaker) {
  const speakerKeys = Object.keys(SPEAKER_COLORS);
  const speakerKey = speakerKeys[parseInt(speaker.split('_')[1] || '0') % speakerKeys.length];
  return SPEAKER_COLORS[speakerKey];
}

/**
 * Format speaker label text with optional flags
 * @param {string} defaultName - Default speaker name (e.g., 'Person 1')
 * @param {boolean} speakerLowConfidence - Whether speaker identification has low confidence
 * @param {boolean} speakerOverlap - Whether there's speaker overlap in this segment
 * @returns {string} Formatted speaker label
 */
export function formatSpeakerLabelText(defaultName, speakerLowConfidence, speakerOverlap) {
  let label = defaultName || 'Speaker';
  if (speakerLowConfidence) {
    label = `${label} · Unknown`;
  }
  if (speakerOverlap) {
    label = `${label} · Overlap`;
  }
  return label;
}

/**
 * Create speaker label CSS classes based on flags
 * @param {boolean} speakerLowConfidence - Whether speaker identification has low confidence
 * @param {boolean} speakerOverlap - Whether there's speaker overlap in this segment
 * @returns {string} Space-separated CSS classes
 */
export function getSpeakerLabelClasses(speakerLowConfidence, speakerOverlap) {
  const classes = ['speaker-label'];
  if (speakerLowConfidence) classes.push('speaker-uncertain');
  if (speakerOverlap) classes.push('speaker-overlap');
  return classes.filter(Boolean).join(' ');
}
