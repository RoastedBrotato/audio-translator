/**
 * Auth Guard Component
 * Provides authentication protection for pages that require user login.
 */

import { initAuth, login } from '/assets/js/auth.js';

/**
 * AuthGuard class for protecting pages
 */
export class AuthGuard {
  /**
   * Create a new AuthGuard instance
   * @param {object} elements - DOM elements
   * @param {HTMLElement} elements.authOverlay - Auth overlay element
   * @param {HTMLElement} elements.mainContent - Main content element to show after auth
   * @param {HTMLElement} elements.signInBtn - Sign in button element (optional)
   * @param {object} options - Configuration options
   * @param {Function} options.onAuthenticated - Callback when user is authenticated
   * @param {Function} options.onAuthFailed - Callback when authentication fails
   */
  constructor(elements, options = {}) {
    this.elements = elements;
    this.options = options;
    this.isAuthenticated = false;
    this.userProfile = null;

    this.init();
  }

  /**
   * Initialize the auth guard
   */
  init() {
    const { signInBtn } = this.elements;

    // Set up sign in button if provided
    if (signInBtn) {
      signInBtn.addEventListener('click', () => {
        this.signIn();
      });
    }
  }

  /**
   * Check authentication status
   * @returns {Promise<boolean>} True if authenticated
   */
  async checkAuth() {
    try {
      this.userProfile = await initAuth();

      if (this.userProfile) {
        this.isAuthenticated = true;
        this.showMainContent();

        if (this.options.onAuthenticated) {
          this.options.onAuthenticated(this.userProfile);
        }

        return true;
      } else {
        this.isAuthenticated = false;
        this.showAuthRequired();

        if (this.options.onAuthFailed) {
          this.options.onAuthFailed();
        }

        return false;
      }
    } catch (error) {
      console.error('Auth check failed:', error);
      this.isAuthenticated = false;
      this.showAuthRequired();

      if (this.options.onAuthFailed) {
        this.options.onAuthFailed(error);
      }

      return false;
    }
  }

  /**
   * Show authentication required overlay
   */
  showAuthRequired() {
    const { authOverlay, mainContent } = this.elements;

    if (authOverlay) {
      authOverlay.style.display = 'flex';
    }

    if (mainContent) {
      mainContent.style.display = 'none';
    }
  }

  /**
   * Show main content (user is authenticated)
   */
  showMainContent() {
    const { authOverlay, mainContent } = this.elements;

    if (authOverlay) {
      authOverlay.style.display = 'none';
    }

    if (mainContent) {
      mainContent.style.display = 'block';
    }
  }

  /**
   * Trigger sign in flow
   */
  async signIn() {
    try {
      await login();
    } catch (error) {
      console.error('Sign in failed:', error);
    }
  }

  /**
   * Get current user profile
   * @returns {object|null} User profile or null
   */
  getUserProfile() {
    return this.userProfile;
  }

  /**
   * Check if user is authenticated
   * @returns {boolean} True if authenticated
   */
  isUserAuthenticated() {
    return this.isAuthenticated;
  }
}

/**
 * Factory function to create and initialize an auth guard
 * @param {object} elementSelectors - Object with element selectors
 * @param {string} elementSelectors.authOverlay - Auth overlay selector (default: '#authOverlay')
 * @param {string} elementSelectors.mainContent - Main content selector (default: '#mainContent')
 * @param {string} elementSelectors.signInBtn - Sign in button selector (default: '#signInBtn')
 * @param {object} options - Auth guard options
 * @returns {Promise<AuthGuard>} Initialized auth guard instance
 */
export async function createAuthGuard(elementSelectors = {}, options = {}) {
  const {
    authOverlay = '#authOverlay',
    mainContent = '#mainContent',
    signInBtn = '#signInBtn'
  } = elementSelectors;

  const elements = {
    authOverlay: document.querySelector(authOverlay),
    mainContent: document.querySelector(mainContent),
    signInBtn: document.querySelector(signInBtn)
  };

  const guard = new AuthGuard(elements, options);
  await guard.checkAuth();

  return guard;
}

/**
 * Simple auth guard that only checks authentication
 * @param {Function} onAuthenticated - Callback when authenticated (receives user profile)
 * @param {Function} onAuthFailed - Callback when not authenticated
 * @returns {Promise<boolean>} True if authenticated
 */
export async function requireAuth(onAuthenticated, onAuthFailed) {
  try {
    const profile = await initAuth();

    if (profile) {
      if (onAuthenticated) {
        onAuthenticated(profile);
      }
      return true;
    } else {
      if (onAuthFailed) {
        onAuthFailed();
      }
      return false;
    }
  } catch (error) {
    console.error('Auth check failed:', error);
    if (onAuthFailed) {
      onAuthFailed(error);
    }
    return false;
  }
}
