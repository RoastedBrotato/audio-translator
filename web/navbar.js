// Unified navigation bar for all pages
function createNavbar() {
  const navbar = document.createElement('nav');
  navbar.className = 'unified-navbar';

  const currentPath = window.location.pathname.split('/').pop() || 'index.html';

  navbar.innerHTML = `
    <div class="navbar-container">
      <div class="navbar-brand">
        <span class="navbar-logo">🌐</span>
        <span class="navbar-title">Audio Translator</span>
      </div>
      <div class="navbar-links">
        <a href="index.html" class="${currentPath === 'index.html' ? 'active' : ''}">
          🏠 Home
        </a>
        <a href="streaming.html" class="${currentPath === 'streaming.html' ? 'active' : ''}">
          🎙️ Live Streaming
        </a>
        <a href="recording.html" class="${currentPath === 'recording.html' ? 'active' : ''}">
          🎵 Audio Upload
        </a>
        <a href="video.html" class="${currentPath === 'video.html' ? 'active' : ''}">
          🎬 Video Upload
        </a>
        <a href="debug.html" class="${currentPath === 'debug.html' ? 'active' : ''}">
          🔧 Debug
        </a>
      </div>
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

  // Add styles
  addNavbarStyles();
}

function addNavbarStyles() {
  if (document.getElementById('navbar-styles')) return;

  const style = document.createElement('style');
  style.id = 'navbar-styles';
  style.textContent = `
    /* Remove default body margins */
    body {
      margin: 0 !important;
      padding: 0 !important;
    }

    /* Add padding to page content wrapper */
    .page-content-wrapper {
      padding: 0 40px;
      max-width: 1600px;
      margin: 0 auto;
    }

    /* Full-width navbar */
    .unified-navbar {
      background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
      box-shadow: 0 2px 10px rgba(0,0,0,0.1);
      position: sticky;
      top: 0;
      left: 0;
      right: 0;
      z-index: 1000;
      width: 100%;
      margin: 0;
      padding: 0;
    }

    .navbar-container {
      display: flex;
      justify-content: space-between;
      align-items: center;
      padding: 15px 40px;
      margin: 0;
      width: 100%;
      box-sizing: border-box;
    }

    .navbar-brand {
      display: flex;
      align-items: center;
      gap: 10px;
      color: white;
      font-weight: bold;
      font-size: 1.2rem;
    }

    .navbar-logo {
      font-size: 1.5rem;
    }

    .navbar-links {
      display: flex;
      gap: 5px;
    }

    .navbar-links a {
      color: white;
      text-decoration: none;
      padding: 8px 16px;
      border-radius: 6px;
      transition: all 0.2s ease;
      font-size: 0.95rem;
      display: flex;
      align-items: center;
      gap: 5px;
    }

    .navbar-links a:hover {
      background: rgba(255, 255, 255, 0.2);
    }

    .navbar-links a.active {
      background: rgba(255, 255, 255, 0.3);
      font-weight: 600;
    }

    /* Mobile responsive */
    @media (max-width: 768px) {
      .navbar-container {
        flex-direction: column;
        gap: 15px;
      }

      .navbar-links {
        flex-wrap: wrap;
        justify-content: center;
      }

      .navbar-links a {
        font-size: 0.85rem;
        padding: 6px 12px;
      }
    }
  `;

  document.head.appendChild(style);
}

// Auto-initialize when DOM is ready
if (document.readyState === 'loading') {
  document.addEventListener('DOMContentLoaded', createNavbar);
} else {
  createNavbar();
}
