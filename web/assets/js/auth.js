import { getConfig } from '/assets/js/config.js';

const AUTH_STORAGE_KEY = 'keycloak_auth';
const AUTH_STATE_KEY = 'keycloak_auth_state';
const AUTH_VERIFIER_KEY = 'keycloak_auth_verifier';

let cachedAuthConfig = null;

async function getAuthConfig() {
    if (cachedAuthConfig) {
        return cachedAuthConfig;
    }

    const appConfig = await getConfig();
    const keycloak = appConfig.keycloak || {};
    cachedAuthConfig = {
        issuer: keycloak.issuer || 'http://localhost:8180/realms/audio-transcriber',
        clientId: keycloak.clientId || 'audio-translator-client',
        redirectUri: window.location.origin + window.location.pathname,
        scope: keycloak.scope || 'openid profile email'
    };

    return cachedAuthConfig;
}

function base64UrlEncode(bytes) {
    return btoa(String.fromCharCode(...bytes))
        .replace(/\+/g, '-')
        .replace(/\//g, '_')
        .replace(/=+$/, '');
}

function randomString(length) {
    const bytes = new Uint8Array(length);
    crypto.getRandomValues(bytes);
    return base64UrlEncode(bytes);
}

async function sha256(value) {
    const data = new TextEncoder().encode(value);
    const hash = await crypto.subtle.digest('SHA-256', data);
    return base64UrlEncode(new Uint8Array(hash));
}

function getStoredAuth() {
    const raw = localStorage.getItem(AUTH_STORAGE_KEY);
    if (!raw) {
        return null;
    }
    try {
        return JSON.parse(raw);
    } catch (err) {
        console.warn('Failed to parse auth storage', err);
        return null;
    }
}

function setStoredAuth(auth) {
    localStorage.setItem(AUTH_STORAGE_KEY, JSON.stringify(auth));
}

function clearStoredAuth() {
    localStorage.removeItem(AUTH_STORAGE_KEY);
}

function parseJwt(token) {
    if (!token) return null;
    const parts = token.split('.');
    if (parts.length !== 3) return null;
    const payload = parts[1].replace(/-/g, '+').replace(/_/g, '/');
    const decoded = atob(payload + '==='.slice((payload.length + 3) % 4));
    try {
        return JSON.parse(decoded);
    } catch (err) {
        return null;
    }
}

async function authEndpoint() {
    const config = await getAuthConfig();
    return `${config.issuer}/protocol/openid-connect/auth`;
}

async function tokenEndpoint() {
    const config = await getAuthConfig();
    return `${config.issuer}/protocol/openid-connect/token`;
}

async function buildAuthUrl(state, codeChallenge) {
    const config = await getAuthConfig();
    const params = new URLSearchParams({
        client_id: config.clientId,
        redirect_uri: config.redirectUri,
        response_type: 'code',
        response_mode: 'query',
        scope: config.scope,
        state: state,
        code_challenge: codeChallenge,
        code_challenge_method: 'S256'
    });

    const endpoint = await authEndpoint();
    return `${endpoint}?${params.toString()}`;
}

async function exchangeCode(code, verifier) {
    const config = await getAuthConfig();
    const body = new URLSearchParams({
        grant_type: 'authorization_code',
        code: code,
        client_id: config.clientId,
        redirect_uri: config.redirectUri,
        code_verifier: verifier
    });

    const endpoint = await tokenEndpoint();
    const response = await fetch(endpoint, {
        method: 'POST',
        headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
        body: body
    });

    if (!response.ok) {
        throw new Error(`Token exchange failed: ${response.status}`);
    }

    return response.json();
}

async function refreshToken(refreshTokenValue) {
    const config = await getAuthConfig();
    const body = new URLSearchParams({
        grant_type: 'refresh_token',
        refresh_token: refreshTokenValue,
        client_id: config.clientId
    });

    const endpoint = await tokenEndpoint();
    const response = await fetch(endpoint, {
        method: 'POST',
        headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
        body: body
    });

    if (!response.ok) {
        throw new Error(`Refresh failed: ${response.status}`);
    }

    return response.json();
}

function withExpiry(auth) {
    const expiresAt = Date.now() + (auth.expires_in * 1000);
    return { ...auth, expires_at: expiresAt };
}

export async function initAuth() {
    await getAuthConfig();
    await handleAuthCallback();

    const auth = getStoredAuth();
    if (!auth) {
        return null;
    }

    if (auth.expires_at && auth.expires_at - Date.now() < 30_000 && auth.refresh_token) {
        try {
            const refreshed = await refreshToken(auth.refresh_token);
            const updated = withExpiry(refreshed);
            setStoredAuth(updated);
            return parseJwt(updated.id_token);
        } catch (err) {
            console.warn('Token refresh failed', err);
            clearStoredAuth();
            return null;
        }
    }

    return parseJwt(auth.id_token);
}

export async function login() {
    const state = randomString(16);
    const verifier = randomString(32);
    const challenge = await sha256(verifier);

    sessionStorage.setItem(AUTH_STATE_KEY, state);
    sessionStorage.setItem(AUTH_VERIFIER_KEY, verifier);
    // Persist as a fallback in case sessionStorage is cleared on redirect.
    localStorage.setItem(AUTH_STATE_KEY, state);
    localStorage.setItem(AUTH_VERIFIER_KEY, verifier);

    const url = await buildAuthUrl(state, challenge);
    window.location.assign(url);
}

export async function logout() {
    const config = await getAuthConfig();
    const auth = getStoredAuth();
    clearStoredAuth();

    if (!auth || !auth.id_token) {
        return;
    }

    const params = new URLSearchParams({
        client_id: config.clientId,
        id_token_hint: auth.id_token,
        post_logout_redirect_uri: config.redirectUri
    });

    window.location.assign(`${config.issuer}/protocol/openid-connect/logout?${params.toString()}`);
}

async function handleAuthCallback() {
    await getAuthConfig();
    const params = new URLSearchParams(window.location.search);
    const hashParams = new URLSearchParams(window.location.hash.replace(/^#/, ''));
    const code = params.get('code') || hashParams.get('code');
    const state = params.get('state') || hashParams.get('state');
    if (!code) {
        return;
    }

    const expectedState = sessionStorage.getItem(AUTH_STATE_KEY) || localStorage.getItem(AUTH_STATE_KEY);
    const verifier = sessionStorage.getItem(AUTH_VERIFIER_KEY) || localStorage.getItem(AUTH_VERIFIER_KEY);
    sessionStorage.removeItem(AUTH_STATE_KEY);
    sessionStorage.removeItem(AUTH_VERIFIER_KEY);
    localStorage.removeItem(AUTH_STATE_KEY);
    localStorage.removeItem(AUTH_VERIFIER_KEY);

    if (!state || !expectedState || state !== expectedState || !verifier) {
        console.warn('Auth state mismatch');
        return;
    }

    try {
        const token = await exchangeCode(code, verifier);
        setStoredAuth(withExpiry(token));
        const url = new URL(window.location.href);
        url.searchParams.delete('code');
        url.searchParams.delete('state');
        url.hash = '';
        window.history.replaceState({}, document.title, url.toString());
    } catch (err) {
        console.error('Auth callback failed', err);
    }
}

export function getAuthProfile() {
    const auth = getStoredAuth();
    return auth ? parseJwt(auth.id_token) : null;
}
