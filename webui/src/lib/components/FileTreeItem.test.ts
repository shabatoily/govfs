import { render, screen, fireEvent, waitFor } from '@testing-library/svelte';
import { describe, it, expect, vi, beforeEach } from 'vitest';
import FileTreeItem from './FileTreeItem.svelte';
import vfs from '../vfs';
import { appState } from '../state.svelte';

// Mock vfs
vi.mock('../vfs', () => ({
    default: {
        list: vi.fn(),
        move: vi.fn(),
        delete: vi.fn(),
        stat: vi.fn(),
    }
}));

describe('FileTreeItem Component', () => {
    const mockFile = {
        id: '1',
        name: 'test-file.txt',
        path: '/test-file.txt',
        isDir: false,
        size: 100,
        modified: '2024-01-01',
        comments: '',
        extension: 'txt'
    };

    const mockFolder = {
        id: '2',
        name: 'test-folder',
        path: '/test-folder',
        isDir: true,
        size: 0,
        modified: '2024-01-01',
        comments: '',
        extension: ''
    };

    beforeEach(() => {
        vi.clearAllMocks();
        appState.setCurrentFile(null);
        appState.setCurrentPath('/');
    });

    it('should render file name', () => {
        render(FileTreeItem, { props: { file: mockFile } });
        expect(screen.getByText('test-file.txt')).toBeInTheDocument();
    });

    it('should select file on click', async () => {
        render(FileTreeItem, { props: { file: mockFile } });

        const item = screen.getByText('test-file.txt');
        await fireEvent.click(item);

        expect(appState.currentFile).toEqual(mockFile);
    });

    it('should expand folder on click and load children', async () => {
        const mockChildren = [
            { ...mockFile, id: '3', name: 'child.txt', path: '/test-folder/child.txt' }
        ];
        (vfs.list as any).mockResolvedValue(mockChildren);

        render(FileTreeItem, { props: { file: mockFolder } });

        const folder = screen.getByText('test-folder');
        await fireEvent.click(folder);

        expect(vfs.list).toHaveBeenCalledWith(mockFolder.path);

        await waitFor(() => {
            expect(screen.getByText('child.txt')).toBeInTheDocument();
        });
    });

    it('should handle rename', async () => {
        // Mock prompt
        vi.spyOn(window, 'prompt').mockReturnValue('renamed.txt');
        (vfs.move as any).mockResolvedValue(true);
        const onRefresh = vi.fn();

        render(FileTreeItem, { props: { file: mockFile, onRefresh } });

        // Hover group to see actions? Testing library doesn't easily simulate CSS hover visibility, 
        // but typically elements are in DOM just hidden.
        // Or we can find by role/title directly if they are rendered but hidden.
        // Code says: class="hidden group-hover:flex ..."
        // So they are in DOM.

        const renameBtn = screen.getByTitle('Rename'); // "Rename" title on button
        await fireEvent.click(renameBtn);

        expect(window.prompt).toHaveBeenCalledWith("Rename to:", "test-file.txt");
        expect(vfs.move).toHaveBeenCalledWith('1', '/renamed.txt'); // Assuming root parent
        expect(onRefresh).toHaveBeenCalled();
    });

    it('should handle delete', async () => {
        vi.spyOn(window, 'confirm').mockReturnValue(true);
        (vfs.delete as any).mockResolvedValue(true);
        const onRefresh = vi.fn();

        render(FileTreeItem, { props: { file: mockFile, onRefresh } });

        const deleteBtn = screen.getByTitle('Delete');
        await fireEvent.click(deleteBtn);

        expect(window.confirm).toHaveBeenCalled();
        expect(vfs.delete).toHaveBeenCalledWith('1');
        expect(onRefresh).toHaveBeenCalled();
    });
});
