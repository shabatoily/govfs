import { render, screen, fireEvent, waitFor } from '@testing-library/svelte';
import { describe, it, expect, vi, beforeEach } from 'vitest';
import Sidebar from './Sidebar.svelte';
import vfs from '../vfs';
import { appState } from '../state.svelte';

// Mock vfs
vi.mock('../vfs', () => ({
    default: {
        list: vi.fn(),
        delete: vi.fn(),
        create: vi.fn(),
        mkdir: vi.fn(),
    }
}));

// Mock appState partially if needed, but we can import the real one and manipulate it since it's a global singleton
// Ideally, appState should be reset between tests.

describe('Sidebar Component', () => {
    const mockFiles = [
        { id: '1', name: 'folder1', isDir: true, path: '/folder1', size: 0, modified: '', comments: '', extension: '' },
        { id: '2', name: 'file1.txt', isDir: false, path: '/file1.txt', size: 100, modified: '', comments: '', extension: 'txt' }
    ];

    beforeEach(() => {
        vi.clearAllMocks();
        // Reset appState
        appState.setCurrentPath('/');
        appState.setCurrentFile(null);
        appState.setFileList([]);

        // Default mock implementation
        (vfs.list as any).mockResolvedValue(mockFiles);
    });

    it('should render file list on mount', async () => {
        render(Sidebar);

        await waitFor(() => {
            expect(vfs.list).toHaveBeenCalledWith('/');
        });

        expect(screen.getByText('folder1')).toBeInTheDocument();
        expect(screen.getByText('file1.txt')).toBeInTheDocument();
    });

    it('should navigate to folder when folder is clicked', async () => {
        render(Sidebar);

        await waitFor(() => {
            expect(screen.getByText('folder1')).toBeInTheDocument();
        });

        // Mock next list call for the folder
        (vfs.list as any).mockResolvedValue([]);

        const folderItem = screen.getByText('folder1');
        await fireEvent.click(folderItem);

        expect(appState.currentPath).toBe('/folder1');
        expect(vfs.list).toHaveBeenCalledWith('/folder1');
    });

    it('should select file when file is clicked', async () => {
        render(Sidebar);

        await waitFor(() => {
            expect(screen.getByText('file1.txt')).toBeInTheDocument();
        });

        const fileItem = screen.getByText('file1.txt');
        await fireEvent.click(fileItem);

        expect(appState.currentFile).toEqual(mockFiles[1]);
    });

    it('should select multiple items and delete them together', async () => {
        vi.spyOn(window, 'confirm').mockReturnValue(true);
        (vfs.delete as any).mockResolvedValue(true);
        render(Sidebar);

        await waitFor(() => expect(screen.getByText('file1.txt')).toBeInTheDocument());
        await fireEvent.click(screen.getByText('folder1'), { ctrlKey: true });
        await fireEvent.click(screen.getByText('file1.txt'), { ctrlKey: true });
        await fireEvent.click(screen.getByTitle('Delete Selected'));

        expect(window.confirm).toHaveBeenCalledWith('Delete 2 items?');
        expect(vfs.delete).toHaveBeenCalledTimes(2);
        expect(vfs.delete).toHaveBeenCalledWith('1');
        expect(vfs.delete).toHaveBeenCalledWith('2');
    });

    it('should move selection with arrow keys without opening a file', async () => {
        render(Sidebar);
        await waitFor(() => expect(screen.getByText('file1.txt')).toBeInTheDocument());

        const tree = screen.getByRole('tree');
        await fireEvent.keyDown(tree, { key: 'ArrowDown' });

        expect(screen.getByText('file1.txt').closest('[role="treeitem"]')).toHaveAttribute('aria-selected', 'true');
        expect(appState.currentFile).toBeNull();

        await fireEvent.keyDown(tree, { key: 'Enter' });
        expect(appState.currentFile).toEqual(mockFiles[1]);
    });

    it('should expand and collapse a folder with horizontal arrow keys', async () => {
        const child = { ...mockFiles[1], id: '3', name: 'child.txt', path: '/folder1/child.txt' };
        (vfs.list as any)
            .mockResolvedValueOnce(mockFiles)
            .mockResolvedValueOnce([child]);
        render(Sidebar);

        await waitFor(() => expect(screen.getByText('folder1')).toBeInTheDocument());
        const tree = screen.getByRole('tree');
        await fireEvent.keyDown(tree, { key: 'ArrowRight' });
        await waitFor(() => expect(screen.getByText('child.txt')).toBeInTheDocument());

        await fireEvent.keyDown(tree, { key: 'ArrowLeft' });
        expect(screen.queryByText('child.txt')).not.toBeInTheDocument();
    });

    it('should handle file deletion', async () => {
        // Mock confirm to return true
        vi.spyOn(window, 'confirm').mockImplementation(() => true);
        (vfs.delete as any).mockResolvedValue(true);

        render(Sidebar);

        await waitFor(() => {
            expect(screen.getByText('file1.txt')).toBeInTheDocument();
        });

        // Find the delete button (trash icon)
        // Since it's hidden with group-hover, we might need to simulate hover or just click it if test env allows
        // querySelector might be easier given the structure
        const deleteButtons = screen.getAllByRole('button').filter(btn => btn.querySelector('svg.lucide-trash-2'));
        // The second one should be for file1.txt (assuming order)
        // Or we can find the list item first

        // Let's rely on the structure: list item contains the button
        const fileItem = screen.getByText('file1.txt').closest('li');
        expect(fileItem).not.toBeNull();

        if (fileItem) {
            const deleteBtn = fileItem.querySelector('button.hover\\:text-red-400');
            expect(deleteBtn).not.toBeNull();
            if (deleteBtn) {
                await fireEvent.click(deleteBtn);
            }
        }

        expect(window.confirm).toHaveBeenCalled();
        expect(vfs.delete).toHaveBeenCalledWith('2');
        expect(vfs.list).toHaveBeenCalledTimes(2); // Initial load + reload after delete
    });
});
