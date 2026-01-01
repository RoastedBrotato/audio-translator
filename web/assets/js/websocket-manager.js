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
