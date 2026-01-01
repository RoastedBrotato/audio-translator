# Web Directory Structure

## Organization

```
web/
├── index.html              # Landing page
├── assets/                 # Shared assets
│   ├── css/               # Stylesheets
│   │   ├── variables.css  # Design tokens
│   │   ├── styles.css     # Style entrypoint (imports modules)
│   │   ├── base.css       # Resets + typography
│   │   ├── layout.css     # Layout primitives
│   │   ├── forms.css      # Form controls
│   │   ├── buttons.css    # Button styles
│   │   ├── upload.css     # Upload UI
│   │   ├── progress.css   # Progress UI
│   │   ├── content.css    # Results + content
│   │   ├── navigation.css # Links + nav sections
│   │   ├── messages.css   # Alerts
│   │   ├── debug.css      # Debug log
│   │   ├── utilities.css  # Utility helpers
│   │   ├── navbar.css     # Shared navbar
│   │   ├── home.css       # Homepage sections
│   │   └── responsive.css # Responsive tweaks
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
