import { render, screen, fireEvent, waitFor } from '@testing-library/svelte';
import { describe, it, expect, vi, beforeEach } from 'vitest';
import Preview from './Preview.svelte';
import EntryComment from './EntryComment.svelte';
import vfs from '../vfs';
import { appState } from '../state.svelte';

class ResizeObserverMock {
    observe() {}
    unobserve() {}
    disconnect() {}
}
globalThis.ResizeObserver = ResizeObserverMock;
// Mock URLs
globalThis.URL.createObjectURL = vi.fn(() => 'blob:mock-url');
globalThis.URL.revokeObjectURL = vi.fn();

vi.mock('../vfs', () => ({
    default: {
        read: vi.fn(),
        writeComments: vi.fn(),
    }
}));

describe('Preview Component', () => {
    const mockFile = {
        id: '1',
        name: 'image.png',
        isDir: false,
        path: '/image.png',
        size: 100,
        modified: '',
        comments: 'Initial comment',
        extension: 'png'
    };

    beforeEach(() => {
        vi.clearAllMocks();
        appState.setCurrentFile(null);
    });

    it('should render "No file selected" when no file', () => {
        render(Preview);
        expect(screen.getByText('No file selected')).toBeInTheDocument();
    });

    it('should load and display image preview', async () => {
        (vfs.read as any).mockResolvedValue(new Blob(['mock-image-data']));

        // mount
        render(Preview);

        // set file
        appState.setCurrentFile(mockFile);

        await waitFor(() => {
            expect(vfs.read).toHaveBeenCalledWith('1');
        });

        const img = screen.getByRole('img');
        expect(img).toHaveAttribute('src', 'blob:mock-url');
        expect(img).toHaveAttribute('alt', 'image.png');
    });

    it('should display comments and allow saving', async () => {
        (vfs.read as any).mockResolvedValue(new Blob(['data']));
        (vfs.writeComments as any).mockResolvedValue({ ...mockFile, comments: 'Updated comment' });

        render(EntryComment);
        appState.setCurrentFile(mockFile);

        await waitFor(() => {
            expect(screen.getByPlaceholderText('Add a comment... (Ctrl+Enter to save)')).toBeInTheDocument();
        });

        const textarea = screen.getByPlaceholderText('Add a comment... (Ctrl+Enter to save)');
        expect(textarea).toHaveValue('Initial comment');

        // Update comment
        await fireEvent.input(textarea, { target: { value: 'Updated comment' } });

        // Save via button
        const saveButton = screen.getByTitle('Save Comment (Ctrl+Enter)');
        await fireEvent.click(saveButton);

        expect(vfs.writeComments).toHaveBeenCalledWith('1', 'Updated comment');
    });
    it('zooms only the image and resets when another file is selected', async () => {
        vi.mocked(vfs.read).mockResolvedValue(new Blob(['image']));
        appState.setCurrentFile(mockFile);
        render(Preview);
        await fireEvent.click(screen.getByRole('button', { name: 'Zoom in' }));
        expect(screen.getByText('125%')).toBeInTheDocument();
        await fireEvent.click(screen.getByRole('button', { name: 'Fit' }));
        expect(screen.getByText('100%')).toBeInTheDocument();
        await fireEvent.click(screen.getByRole('button', { name: 'Zoom out' }));
        expect(screen.getByText('80%')).toBeInTheDocument();
        appState.setCurrentFile({ ...mockFile, id: '2' });
        await waitFor(() => expect(screen.getByText('100%')).toBeInTheDocument());
    });

    it('pans the image viewport while the pointer is held', async () => {
        vi.mocked(vfs.read).mockResolvedValue(new Blob(['image']));
        appState.setCurrentFile(mockFile);
        render(Preview);
        const region = await screen.findByRole('region', { name: 'Image preview' });
        region.setPointerCapture = vi.fn();
        region.scrollLeft = 100;
        region.scrollTop = 100;
        const pointer = (type: string, x: number, y: number) => {
            const event = new Event(type, { bubbles: true });
            Object.assign(event, { button: 0, pointerId: 1, clientX: x, clientY: y });
            return fireEvent(region, event);
        };
        await pointer('pointerdown', 50, 50);
        await pointer('pointermove', 30, 20);
        expect(region.scrollLeft).toBe(120);
        expect(region.scrollTop).toBe(130);
        expect(region).not.toHaveAttribute('tabindex');
        await fireEvent.keyDown(screen.getByRole('button', { name: 'Zoom in' }), { key: 'ArrowRight' });
        expect(region.scrollLeft).toBe(160);
        await pointer('pointerup', 30, 20);
        await pointer('pointermove', 0, 0);
        expect(region.scrollLeft).toBe(160);
    });

});
