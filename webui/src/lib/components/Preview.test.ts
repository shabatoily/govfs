import { render, screen, fireEvent, waitFor } from '@testing-library/svelte';
import { describe, it, expect, vi, beforeEach } from 'vitest';
import Preview from './Preview.svelte';
import vfs from '../vfs';
import { appState } from '../state.svelte';

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

        render(Preview);
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
});
