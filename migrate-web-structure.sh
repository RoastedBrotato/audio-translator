#!/bin/bash
set -e

echo "🚀 Starting web directory restructuring..."
echo ""

# Safety check
if [ ! -d "web" ]; then
    echo "❌ Error: web directory not found. Run this script from project root."
    exit 1
fi

# Create backup
echo "📦 Creating backup..."
BACKUP_DIR="web-backup-$(date +%Y%m%d-%H%M%S)"
cp -r web "$BACKUP_DIR"
echo "✅ Backup created: $BACKUP_DIR"
echo ""

# Create new directory structure
echo "📁 Creating new directory structure..."
mkdir -p web/assets/{css,js,images}
mkdir -p web/components/navbar
mkdir -p web/features/{home,streaming,recording,video,meeting}
echo "✅ Directories created"
echo ""

# Move files to features
echo "📦 Moving files to feature folders..."

# Meeting feature
if [ -f "web/meeting.html" ]; then
    mv web/meeting.html web/features/meeting/meeting-create.html
    echo "  ✓ meeting.html → features/meeting/meeting-create.html"
fi

if [ -f "web/meeting-join.html" ]; then
    mv web/meeting-join.html web/features/meeting/
    echo "  ✓ meeting-join.html → features/meeting/"
fi

if [ -f "web/meeting-room.html" ]; then
    mv web/meeting-room.html web/features/meeting/
    echo "  ✓ meeting-room.html → features/meeting/"
fi

if [ -f "web/meeting.js" ]; then
    mv web/meeting.js web/features/meeting/meeting-room.js
    echo "  ✓ meeting.js → features/meeting/meeting-room.js"
fi

# Streaming feature
if [ -f "web/streaming.html" ]; then
    mv web/streaming.html web/features/streaming/
    echo "  ✓ streaming.html → features/streaming/"
fi

if [ -f "web/streaming.js" ]; then
    mv web/streaming.js web/features/streaming/
    echo "  ✓ streaming.js → features/streaming/"
fi

if [ -f "web/pcm-worklet.js" ]; then
    mv web/pcm-worklet.js web/features/streaming/
    echo "  ✓ pcm-worklet.js → features/streaming/"
fi

# Recording feature
if [ -f "web/recording.html" ]; then
    mv web/recording.html web/features/recording/
    echo "  ✓ recording.html → features/recording/"
fi

if [ -f "web/recording.js" ]; then
    mv web/recording.js web/features/recording/
    echo "  ✓ recording.js → features/recording/"
fi

# Video feature
if [ -f "web/video.html" ]; then
    mv web/video.html web/features/video/
    echo "  ✓ video.html → features/video/"
fi

if [ -f "web/video.js" ]; then
    mv web/video.js web/features/video/
    echo "  ✓ video.js → features/video/"
fi

# Home feature
if [ -f "web/overview.html" ]; then
    mv web/overview.html web/features/home/
    echo "  ✓ overview.html → features/home/"
fi

if [ -f "web/home-demo.js" ]; then
    mv web/home-demo.js web/features/home/
    echo "  ✓ home-demo.js → features/home/"
fi

# Components
if [ -f "web/navbar.js" ]; then
    mv web/navbar.js web/components/navbar/
    echo "  ✓ navbar.js → components/navbar/"
fi

# Assets
if [ -f "web/app.js" ]; then
    mv web/app.js web/assets/js/
    echo "  ✓ app.js → assets/js/"
fi

if [ -f "web/styles.css" ]; then
    mv web/styles.css web/assets/css/
    echo "  ✓ styles.css → assets/css/"
fi

echo ""
echo "✅ Files moved to features"
echo ""

# Update HTML file paths
echo "🔧 Updating file paths in HTML files..."

# Function to update paths in HTML files
update_html_paths() {
    local file=$1
    local depth=$2

    # Calculate relative path to root based on depth
    local rel_path=""
    for ((i=0; i<depth; i++)); do
        rel_path="../$rel_path"
    done

    # Update stylesheet paths
    sed -i 's|href="styles\.css"|href="'"$rel_path"'assets/css/styles.css"|g' "$file"

    # Update script paths (common ones)
    sed -i 's|src="navbar\.js"|src="'"$rel_path"'components/navbar/navbar.js"|g' "$file"
    sed -i 's|src="app\.js"|src="'"$rel_path"'assets/js/app.js"|g' "$file"
}

# Update meeting feature files (depth 2: features/meeting/)
for file in web/features/meeting/*.html; do
    if [ -f "$file" ]; then
        update_html_paths "$file" 2
        # Update meeting-specific script paths
        sed -i 's|src="meeting\.js"|src="meeting-room.js"|g' "$file"
        echo "  ✓ Updated paths in $(basename $file)"
    fi
done

# Update other feature files (depth 2)
for feature in streaming recording video home; do
    for file in web/features/$feature/*.html; do
        if [ -f "$file" ]; then
            update_html_paths "$file" 2
            echo "  ✓ Updated paths in $(basename $file)"
        fi
    done
done

# Update root-level HTML files (depth 0)
for file in web/*.html; do
    if [ -f "$file" ]; then
        update_html_paths "$file" 0
        echo "  ✓ Updated paths in $(basename $file)"
    fi
done

echo ""
echo "✅ File paths updated"
echo ""

# Create utility files
echo "📝 Creating shared utility files..."

# Create audio-processor.js
cat > web/assets/js/audio-processor.js << 'EOF'
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
EOF
echo "  ✓ Created audio-processor.js"

# Create utils.js
cat > web/assets/js/utils.js << 'EOF'
/**
 * General Utility Functions
 */

/**
 * Debounce function calls
 */
export function debounce(func, wait) {
    let timeout;
    return function executedFunction(...args) {
        const later = () => {
            clearTimeout(timeout);
            func(...args);
        };
        clearTimeout(timeout);
        timeout = setTimeout(later, wait);
    };
}

/**
 * Format timestamp for display
 */
export function formatTimestamp(date) {
    return date.toLocaleTimeString('en-US', {
        hour12: false,
        hour: '2-digit',
        minute: '2-digit',
        second: '2-digit'
    });
}

/**
 * Escape HTML to prevent XSS
 */
export function escapeHtml(text) {
    const div = document.createElement('div');
    div.textContent = text;
    return div.innerHTML;
}

/**
 * Get language display name from code
 */
export function getLanguageName(code) {
    const languages = {
        'en': 'English',
        'ar': 'Arabic',
        'ur': 'Urdu',
        'es': 'Spanish',
        'fr': 'French',
        'de': 'German',
        'zh': 'Chinese',
        'ja': 'Japanese',
        'ko': 'Korean',
        'hi': 'Hindi'
    };
    return languages[code] || code;
}

/**
 * Download a file from blob
 */
export function downloadBlob(blob, filename) {
    const url = URL.createObjectURL(blob);
    const link = document.createElement('a');
    link.href = url;
    link.download = filename;
    document.body.appendChild(link);
    link.click();
    document.body.removeChild(link);
    URL.revokeObjectURL(url);
}
EOF
echo "  ✓ Created utils.js"

# Create websocket-manager.js
cat > web/assets/js/websocket-manager.js << 'EOF'
/**
 * WebSocket Manager with Auto-Reconnect
 */

export class WebSocketManager {
    constructor(url, options = {}) {
        this.url = url;
        this.ws = null;
        this.reconnectAttempts = 0;
        this.maxReconnectAttempts = options.maxReconnectAttempts || 5;
        this.reconnectDelay = options.reconnectDelay || 1000;
        this.handlers = {
            open: [],
            message: [],
            close: [],
            error: []
        };
    }

    connect() {
        this.ws = new WebSocket(this.url);

        this.ws.onopen = (event) => {
            console.log('WebSocket connected');
            this.reconnectAttempts = 0;
            this.handlers.open.forEach(handler => handler(event));
        };

        this.ws.onmessage = (event) => {
            this.handlers.message.forEach(handler => handler(event));
        };

        this.ws.onerror = (error) => {
            console.error('WebSocket error:', error);
            this.handlers.error.forEach(handler => handler(error));
        };

        this.ws.onclose = (event) => {
            console.log('WebSocket closed');
            this.handlers.close.forEach(handler => handler(event));
            this.attemptReconnect();
        };
    }

    attemptReconnect() {
        if (this.reconnectAttempts >= this.maxReconnectAttempts) {
            console.error('Max reconnection attempts reached');
            return;
        }

        this.reconnectAttempts++;
        const delay = this.reconnectDelay * Math.pow(2, this.reconnectAttempts - 1);

        console.log(`Reconnecting in ${delay}ms (attempt ${this.reconnectAttempts}/${this.maxReconnectAttempts})`);

        setTimeout(() => {
            this.connect();
        }, delay);
    }

    send(data) {
        if (this.ws && this.ws.readyState === WebSocket.OPEN) {
            this.ws.send(data);
            return true;
        }
        return false;
    }

    onOpen(handler) {
        this.handlers.open.push(handler);
    }

    onMessage(handler) {
        this.handlers.message.push(handler);
    }

    onClose(handler) {
        this.handlers.close.push(handler);
    }

    onError(handler) {
        this.handlers.error.push(handler);
    }

    close() {
        if (this.ws) {
            this.maxReconnectAttempts = 0; // Prevent reconnection
            this.ws.close();
        }
    }
}
EOF
echo "  ✓ Created websocket-manager.js"

echo ""
echo "✅ Utility files created"
echo ""

# Create CSS variable file
echo "📝 Creating CSS organization structure..."

cat > web/assets/css/variables.css << 'EOF'
/**
 * CSS Custom Properties (Design Tokens)
 */

:root {
    /* Colors - Primary */
    --primary-color: #14b8a6;
    --primary-dark: #0f766e;
    --primary-light: #5eead4;

    /* Colors - Secondary */
    --secondary-color: #f97316;
    --secondary-dark: #c2410c;
    --secondary-light: #fb923c;

    /* Colors - Neutrals */
    --text-primary: #0f172a;
    --text-secondary: #475569;
    --text-tertiary: #94a3b8;
    --bg-white: #ffffff;
    --bg-light: #f8fafc;
    --bg-hover: #f1f5f9;
    --border-color: #cbd5e1;
    --border-light: #e2e8f0;

    /* Colors - Semantic */
    --success-color: #22c55e;
    --warning-color: #f59e0b;
    --error-color: #ef4444;
    --info-color: #3b82f6;

    /* Typography */
    --font-sans: 'Segoe UI', Tahoma, Geneva, Verdana, sans-serif;
    --font-display: 'Poppins', -apple-system, sans-serif;
    --font-mono: 'Courier New', monospace;

    /* Font Sizes */
    --text-xs: 0.75rem;
    --text-sm: 0.875rem;
    --text-base: 1rem;
    --text-lg: 1.125rem;
    --text-xl: 1.25rem;
    --text-2xl: 1.5rem;
    --text-3xl: 1.875rem;

    /* Spacing */
    --spacing-xs: 0.25rem;
    --spacing-sm: 0.5rem;
    --spacing-md: 1rem;
    --spacing-lg: 1.5rem;
    --spacing-xl: 2rem;
    --spacing-2xl: 3rem;

    /* Shadows */
    --shadow-sm: 0 1px 2px 0 rgb(0 0 0 / 0.05);
    --shadow-md: 0 4px 6px -1px rgb(0 0 0 / 0.1);
    --shadow-lg: 0 10px 15px -3px rgb(0 0 0 / 0.1);
    --shadow-xl: 0 20px 25px -5px rgb(0 0 0 / 0.1);

    /* Border Radius */
    --radius-sm: 0.25rem;
    --radius-md: 0.5rem;
    --radius-lg: 0.75rem;
    --radius-xl: 1rem;
    --radius-full: 9999px;

    /* Transitions */
    --transition-fast: 150ms ease;
    --transition-base: 300ms ease;
    --transition-slow: 500ms ease;

    /* Z-index */
    --z-dropdown: 1000;
    --z-sticky: 1020;
    --z-fixed: 1030;
    --z-modal: 1040;
    --z-tooltip: 1050;
}
EOF
echo "  ✓ Created variables.css"

# Create README for the new structure
cat > web/README.md << 'EOF'
# Web Directory Structure

## Organization

```
web/
├── index.html              # Landing page
├── assets/                 # Shared assets
│   ├── css/               # Stylesheets
│   │   ├── variables.css  # Design tokens
│   │   └── styles.css     # Main styles (to be split)
│   ├── js/                # Shared utilities
│   │   ├── audio-processor.js
│   │   ├── utils.js
│   │   └── websocket-manager.js
│   └── images/            # Images, logos, icons
├── components/            # Reusable UI components
│   └── navbar/
│       └── navbar.js
└── features/              # Feature-based modules
    ├── home/
    ├── streaming/
    ├── recording/
    ├── video/
    └── meeting/
```

## Usage

### Importing Shared Utilities

```javascript
// In any feature JS file:
import { convertToPCM16, hasVoiceActivity } from '../../assets/js/audio-processor.js';
import { debounce, escapeHtml } from '../../assets/js/utils.js';
import { WebSocketManager } from '../../assets/js/websocket-manager.js';
```

### Adding Styles

```html
<!-- In feature HTML files: -->
<link rel="stylesheet" href="../../assets/css/variables.css">
<link rel="stylesheet" href="../../assets/css/styles.css">
```

## Next Steps

1. **Refactor meeting-room.js** - Use shared utilities
2. **Split styles.css** - Extract components, layouts
3. **Add feature-specific CSS** - Create meeting.css, streaming.css, etc.
4. **Update other JS files** - Use shared audio-processor.js
5. **Add tests** - Test shared utilities

## File Naming Conventions

- HTML: `feature-name.html` (kebab-case)
- JS: `feature-name.js` (kebab-case)
- CSS: `feature-name.css` (kebab-case)
- Components: `component-name/` (folder with same-named files)
EOF
echo "  ✓ Created README.md"

echo ""
echo "✅ CSS organization created"
echo ""

# Print summary
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "✨ Migration Complete!"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""
echo "📊 Summary:"
echo "  • Backup created: $BACKUP_DIR"
echo "  • New structure: web/features/, web/assets/, web/components/"
echo "  • Shared utilities: audio-processor.js, utils.js, websocket-manager.js"
echo "  • CSS variables: variables.css created"
echo "  • Documentation: web/README.md"
echo ""
echo "🔍 Verify the migration:"
echo "  1. Check file structure: tree web/"
echo "  2. Test the app: http://localhost:8080"
echo "  3. Check browser console for errors"
echo ""
echo "⚠️  Next Steps:"
echo "  1. Update feature JS files to import shared utilities"
echo "  2. Split styles.css into modular files"
echo "  3. Test all features thoroughly"
echo ""
echo "💡 If something breaks, restore from: $BACKUP_DIR"
echo ""
