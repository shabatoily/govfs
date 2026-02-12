export type SSEEventType = 'subscribe' | 'unsubscribe' | 'publish' | 'error' | 'heartbeat' | 'open';

export interface SSEMeta {
    id?: string; // 파일 ID
    path?: string; // 파일 경로
    action?: string; // 액션
}

export interface SSEData {
    timestamp: string;
    status: boolean;
    message?: string; // event type이 error인 경우 에러 메시지가 존재합니다.
    meta?: SSEMeta;
}

export interface SSEMessage {
    id: string | null;
    event: SSEEventType;
    data: SSEData;
    retry?: number;
}

export type SSEHandler = (message: SSEMessage) => void;

export class SSEClient {
    private eventSource: EventSource | null = null;
    private listeners: Map<string, Set<SSEHandler>> = new Map();
    private url: string;

    constructor(url: string = '/sse/subscribe') {
        this.url = url;
    }

    /**
     * Connect to the SSE stream.
     */
    public connect(): void {
        if (this.eventSource) {
            console.warn('SSEClient already connected');
            return;
        }

        console.log(`Connecting to SSE at ${this.url}`);
        this.eventSource = new EventSource(this.url);

        this.eventSource.onopen = (event) => {
            console.log('SSE connection opened:', event);
            this.dispatchEvent('open', {
                id: null,
                event: 'open',
                data: { timestamp: new Date().toISOString(), status: true }
            });
        };

        this.eventSource.onerror = (event) => {
            console.error('SSE connection error:', event);
            // EventSource automatically attempts to reconnect on error.
            // We can dispatch a global error event if needed.
            this.dispatchEvent('error', {
                id: null,
                event: 'error',
                data: { timestamp: new Date().toISOString(), status: false }
            });
        };

        // Listen for standard events we know about
        const events: SSEEventType[] = ['subscribe', 'unsubscribe', 'publish', 'heartbeat'];

        events.forEach(eventType => {
            this.eventSource?.addEventListener(eventType, (event: MessageEvent) => {
                this.handleMessage(eventType, event);
            });
        });

        // Also listen to default 'message' event if any
        this.eventSource.onmessage = (event: MessageEvent) => {
            this.handleMessage('message' as any, event);
        };
    }

    /**
     * Disconnect from the SSE stream.
     */
    public disconnect(): void {
        if (this.eventSource) {
            this.eventSource.close();
            this.eventSource = null;
            console.log('SSE connection closed');
        }
    }

    /**
     * Subscribe to a specific event type.
     * @param event The event name (e.g., 'publish', 'subscribe')
     * @param callback The function to call when event is received
     */
    public on(event: string, callback: SSEHandler): void {
        if (!this.listeners.has(event)) {
            this.listeners.set(event, new Set());
        }
        this.listeners.get(event)?.add(callback);

        // If we want to support dynamic event types that aren't in the default list,
        // we might need to addEventListener dynamically.
        // However, EventSource doesn't support removing listeners easily without the exact wrapper if we use a wrapper.
        // For simplicity, we assume the server sends known event types or we add a generic listener.
        // But since we did addEventListener for the known types above, this works for those.
        // If the user adds a custom event listener that the server sends but we didn't register in connect(), it won't fire.
        // To fix this, we should add the listener to eventSource if it's connected.

        if (this.eventSource && !['message', 'error', 'open', 'subscribe', 'unsubscribe', 'publish', 'heartbeat'].includes(event)) {
            // Check if we already added this event type to EventSource? 
            // The approach in `connect` was to add specific ones. 
            // Let's change `connect` to be more generic or just add raw listener here.
            // Use a specific internal handler to route it.
            this.eventSource.addEventListener(event, (e: MessageEvent) => this.handleMessage(event, e));
        }
    }

    /**
     * Unsubscribe from a specific event type.
     * @param event The event name
     * @param callback The callback to remove
     */
    public off(event: string, callback: SSEHandler): void {
        if (this.listeners.has(event)) {
            this.listeners.get(event)?.delete(callback);
            if (this.listeners.get(event)?.size === 0) {
                this.listeners.delete(event);
            }
        }
        // Note: We don't remove the native EventSource listener because we can't easily identify the wrapper closure 
        // without storing it. Given low number of event types, this is usually strictly fine.
        // If this becomes a memory issue, we would need to store the bound handlers.
    }

    private handleMessage(type: string, event: MessageEvent) {
        if (type === 'heartbeat') {
            // Just a keep-alive, potentially ignore or log
            console.debug('heartbeat received');
            return;
        }

        try {
            // The server sends the data as JSON structure compatible with SSEData
            const rawData = event.data;
            const parsedData: SSEData = JSON.parse(rawData);

            // DEBUG: Trace missing ID
            if (!event.lastEventId) {
                console.warn("SSE: Received subscription success but missing lastEventId (ID field)!", event);
            }

            const message: SSEMessage = {
                id: event.lastEventId,
                event: type as SSEEventType,
                data: parsedData
            }

            this.dispatchEvent(type, message);

        } catch (err) {
            console.error(`Failed to parse SSE message for event ${type}:`, err);
        }
    }

    private dispatchEvent(type: string, message: SSEMessage) {
        if (this.listeners.has(type)) {
            this.listeners.get(type)?.forEach(handler => handler(message));
        }
    }
}

const defaultClient = new SSEClient();

export default defaultClient;
