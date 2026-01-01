// Unified navigation bar for all pages
function createNavbar() {
  const navbar = document.createElement('nav');
  navbar.className = 'unified-navbar';

  const currentPath = window.location.pathname;
  const isActive = (path) => currentPath.endsWith(path);

  navbar.innerHTML = `
    <div class="navbar-container">
      <div class="navbar-brand">
        <span class="navbar-logo">🌐</span>
        <span class="navbar-title">Audio Translator</span>
      </div>
      <div class="navbar-links">
        <a href="/index.html" class="${isActive('/index.html') ? 'active' : ''}">
          🏠 Home
        </a>
        <a href="/features/meeting/meeting-create.html" class="${isActive('/features/meeting/meeting-create.html') ? 'active' : ''}">
          👥 Meetings
        </a>
        <a href="/features/streaming/streaming.html" class="${isActive('/features/streaming/streaming.html') ? 'active' : ''}">
          🎙️ Live Streaming
        </a>
        <a href="/features/recording/recording.html" class="${isActive('/features/recording/recording.html') ? 'active' : ''}">
          🎵 Audio Upload
        </a>
        <a href="/features/video/video.html" class="${isActive('/features/video/video.html') ? 'active' : ''}">
          🎬 Video Upload
        </a>
        <a href="/features/home/overview.html" class="${isActive('/features/home/overview.html') ? 'active' : ''}">
          🧭 How It Works
        </a>
      </div>
      <button class="theme-toggle" type="button" aria-label="Toggle theme">
        <span class="theme-toggle-icon" aria-hidden="true">🌙</span>
        <span class="theme-toggle-label">Dark</span>
      </button>
    </div>
  `;

  // Create content wrapper for all existing body content
  const contentWrapper = document.createElement('div');
  contentWrapper.className = 'page-content-wrapper';

  // Move all existing body children into the wrapper
  while (document.body.firstChild) {
    contentWrapper.appendChild(document.body.firstChild);
  }

  // Add navbar and wrapped content to body
  document.body.appendChild(navbar);
  document.body.appendChild(contentWrapper);

  setupThemeToggle();
}

function setupThemeToggle() {
  const toggle = document.querySelector('.theme-toggle');
  if (!toggle) return;

  const root = document.documentElement;
  const storedTheme = localStorage.getItem('theme');
  const prefersDark = window.matchMedia && window.matchMedia('(prefers-color-scheme: dark)').matches;
  const initialTheme = storedTheme || (prefersDark ? 'dark' : 'light');
  root.setAttribute('data-theme', initialTheme);
  updateThemeToggle(toggle, initialTheme);

  toggle.addEventListener('click', () => {
    const current = root.getAttribute('data-theme') || 'light';
    const next = current === 'dark' ? 'light' : 'dark';
    root.setAttribute('data-theme', next);
    localStorage.setItem('theme', next);
    updateThemeToggle(toggle, next);
  });
}

function updateThemeToggle(toggle, theme) {
  const icon = toggle.querySelector('.theme-toggle-icon');
  const label = toggle.querySelector('.theme-toggle-label');
  if (theme === 'dark') {
    icon.textContent = '☀️';
    label.textContent = 'Light';
  } else {
    icon.textContent = '🌙';
    label.textContent = 'Dark';
  }
}

// Auto-initialize when DOM is ready
if (document.readyState === 'loading') {
  document.addEventListener('DOMContentLoaded', createNavbar);
} else {
  createNavbar();
}
