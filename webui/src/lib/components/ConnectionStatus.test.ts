import { render, screen, cleanup } from '@testing-library/svelte';
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import ConnectionStatus from './ConnectionStatus.svelte';
import sseClient from '../sse';

// Mock sseClient
vi.mock('../sse', () => ({
    default: {
        on: vi.fn(),
        off: vi.fn(),
        connect: vi.fn(),
        disconnect: vi.fn(),
    }
}));

describe('ConnectionStatus Component', () => {
    let handlers: Record<string, Function> = {};

    beforeEach(() => {
        handlers = {};
        // Capture handlers
        (sseClient.on as any).mockImplementation((event: string, handler: Function) => {
            handlers[event] = handler;
        });
        vi.clearAllMocks();
    });

    afterEach(() => {
        cleanup();
    });

    it('should be offline initially', () => {
        render(ConnectionStatus);
        expect(screen.getByText('Offline')).toBeInTheDocument();
        // Check for red color class
        const indicator = screen.getByText('Offline').previousElementSibling;
        expect(indicator).toHaveClass('bg-red-500');
    });

    it('should switch to online on "open" event', async () => {
        render(ConnectionStatus);

        // Simulate open event
        if (handlers['open']) {
            handlers['open']({ data: { status: true } });
        } else {
            throw new Error('Handler for "open" not registered');
        }

        // Svelte 5 state updates are fine-grained, testing-library should pick it up
        // Need to wait for update? usually fine with sync updates in svelte 5 unless async.
        // Let's use await screen.findByText if unsure, but data update is sync.
        expect(await screen.findByText('Online')).toBeInTheDocument();

        const indicator = screen.getByText('Online').previousElementSibling;
        expect(indicator).toHaveClass('bg-green-500');
    });

    it('should switch to offline on "error" event', async () => {
        render(ConnectionStatus);

        // Set to online first
        if (handlers['open']) handlers['open']({ data: { status: true } });
        await screen.findByText('Online');

        // Trigger error
        if (handlers['error']) handlers['error']({ data: { status: false } });

        expect(await screen.findByText('Offline')).toBeInTheDocument();
        const indicator = screen.getByText('Offline').previousElementSibling;
        expect(indicator).toHaveClass('bg-red-500');
    });

    it('should switch to online on "heartbeat" event', async () => {
        render(ConnectionStatus);

        // trigger heartbeat
        if (handlers['heartbeat']) handlers['heartbeat']();

        expect(await screen.findByText('Online')).toBeInTheDocument();
    });

    it('should cleanup listeners on destroy', () => {
        const { unmount } = render(ConnectionStatus);
        unmount();

        expect(sseClient.off).toHaveBeenCalledWith('open', expect.any(Function));
        expect(sseClient.off).toHaveBeenCalledWith('error', expect.any(Function));
    });
});
