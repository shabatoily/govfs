import { describe, it, expect, vi, beforeEach, afterEach, type Mock } from 'vitest';
import defaultClient, { SSEClient } from './sse';

describe('SSEClient', () => {
    let mockEventSource: any;

    beforeEach(() => {
        // Mock EventSource
        mockEventSource = {
            close: vi.fn(),
            addEventListener: vi.fn(),
            removeEventListener: vi.fn(),
            onopen: null,
            onerror: null,
            onmessage: null,
        };

        vi.stubGlobal('EventSource', vi.fn(function () {
            return mockEventSource;
        }));
    });

    afterEach(() => {
        vi.unstubAllGlobals();
        defaultClient.disconnect();
    });

    it('should connect to the correct URL', () => {
        const client = new SSEClient('/test-sse');
        client.connect();

        expect(EventSource).toHaveBeenCalledWith('/test-sse');
    });

    it('should not connect if already connected', () => {
        const client = new SSEClient('/test-sse');
        client.connect();
        client.connect();

        expect(EventSource).toHaveBeenCalledTimes(1);
    });

    it('should disconnect correctly', () => {
        const client = new SSEClient('/test-sse');
        client.connect();
        client.disconnect();

        expect(mockEventSource.close).toHaveBeenCalled();
    });

    it('should handle open event', () => {
        const client = new SSEClient();
        const handler = vi.fn();
        client.on('open', handler);
        client.connect();

        // Simulate onopen
        if (mockEventSource.onopen) {
            mockEventSource.onopen(new Event('open'));
        }

        expect(handler).toHaveBeenCalledWith(expect.objectContaining({
            event: 'open',
            data: expect.objectContaining({ status: true })
        }));
    });

    it('should handle error event', () => {
        const client = new SSEClient();
        const handler = vi.fn();
        client.on('error', handler);
        client.connect();

        // Simulate onerror
        if (mockEventSource.onerror) {
            mockEventSource.onerror(new Event('error'));
        }

        expect(handler).toHaveBeenCalledWith(expect.objectContaining({
            event: 'error',
            data: expect.objectContaining({ status: false })
        }));
    });

    it('should handle messages for subscribed events', () => {
        const client = new SSEClient();
        const handler = vi.fn();
        client.on('publish', handler);
        client.connect();

        // Find the event listener for 'publish'
        const addListenerMock = mockEventSource.addEventListener as Mock;
        const publishCall = addListenerMock.mock.calls.find(call => call[0] === 'publish');
        expect(publishCall).toBeDefined();

        const [, listener] = publishCall!;

        // Simulate event
        const mockData = { timestamp: '2024-01-01', status: true, message: 'hello' };
        const mockEvent = {
            data: JSON.stringify(mockData),
            lastEventId: '123'
        };

        listener(mockEvent);

        expect(handler).toHaveBeenCalledWith({
            id: '123',
            event: 'publish',
            data: mockData
        });
    });

    it('should remove listeners with off()', () => {
        const client = new SSEClient();
        const handler = vi.fn();

        client.on('test', handler);
        client.off('test', handler);

        // We can't easily test internal Map state without exposing it, 
        // but we can verify it doesn't fire if we could trigger it.
        // Instead, let's just ensure no error is thrown and cover the path.
        // A better test would be to emit an event and ensure handler is NOT called.
        // But since 'test' isn't natively bound in connect(), we need to mock internal dispatch.

        // Let's use a standard event 'publish'
        client.on('publish', handler);
        client.off('publish', handler);
        client.connect();

        // Find the event listener for 'publish'
        const addListenerMock = mockEventSource.addEventListener as Mock;
        const publishCall = addListenerMock.mock.calls.find(call => call[0] === 'publish');
        const [, listener] = publishCall!;

        const mockData = { timestamp: '2024-01-01', status: true };
        const mockEvent = { data: JSON.stringify(mockData) };

        listener(mockEvent);

        expect(handler).not.toHaveBeenCalled();
    });
});
