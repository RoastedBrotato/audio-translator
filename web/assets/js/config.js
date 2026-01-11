const DEFAULT_CONFIG = {
    keycloak: {
        issuer: 'http://localhost:8180/realms/audio-transcriber',
        clientId: 'audio-translator-client',
        scope: 'openid profile email'
    },
    services: {
        asrBaseUrl: 'http://localhost:8003',
        translationBaseUrl: 'http://localhost:8004',
        ttsBaseUrl: 'http://localhost:8005',
        embeddingBaseUrl: 'http://localhost:8006',
        llmBaseUrl: 'http://localhost:8007'
    }
};

let cachedConfig = null;

function isObject(value) {
    return value && typeof value === 'object' && !Array.isArray(value);
}

function deepMerge(target, source) {
    const output = { ...target };
    if (!isObject(source)) {
        return output;
    }
    Object.keys(source).forEach((key) => {
        if (isObject(source[key]) && isObject(output[key])) {
            output[key] = deepMerge(output[key], source[key]);
        } else {
            output[key] = source[key];
        }
    });
    return output;
}

async function loadConfig() {
    let config = DEFAULT_CONFIG;
    try {
        const response = await fetch('/config.json', { cache: 'no-store' });
        if (response.ok) {
            const data = await response.json();
            config = deepMerge(config, data);
        }
    } catch (error) {
        console.warn('Failed to load /config.json, using defaults.', error);
    }

    if (window.APP_CONFIG && isObject(window.APP_CONFIG)) {
        config = deepMerge(config, window.APP_CONFIG);
    }

    return config;
}

export async function getConfig() {
    if (!cachedConfig) {
        cachedConfig = await loadConfig();
    }
    return cachedConfig;
}
