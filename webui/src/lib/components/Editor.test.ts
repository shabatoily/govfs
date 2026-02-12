import { render, screen, waitFor, fireEvent } from '@testing-library/svelte';
import { describe, it, expect, vi, beforeEach } from 'vitest';
import Editor from './Editor.svelte';
import vfs from '../vfs';
import { appState } from '../state.svelte';
import OverType from 'overtype';

// Mock OverType
vi.mock('overtype', () => {
    return {
        default: {
            init: vi.fn(),
        }
    };
});

vi.mock('../vfs', () => ({
    default: {
        read: vi.fn(),
        write: vi.fn(),
    }
}));

describe('Editor Component', () => {
    let mockEditorInstance: any;

    beforeEach(() => {
        vi.clearAllMocks();
        appState.setCurrentFile(null);

        // Setup mock editor instance
        mockEditorInstance = {
            setValue: vi.fn(),
            getValue: vi.fn(() => 'current content'),
        };

        // Mock init to return our instance
        (OverType.init as any).mockReturnValue([mockEditorInstance]);
    });

    it('should initialize OverType on mount', () => {
        render(Editor);
        expect(OverType.init).toHaveBeenCalled();
    });

    it('should load content when file is selected', async () => {
        const mockFile = { id: '1', name: 'test.txt', isDir: false, path: '/test.txt', size: 0, modified: '', comments: '', extension: 'txt' };
        (vfs.read as any).mockResolvedValue('file content');

        render(Editor);

        // Trigger file selection
        appState.setCurrentFile(mockFile);

        await waitFor(() => {
            expect(vfs.read).toHaveBeenCalledWith('1');
        });

        // setValue should be called with content
        expect(mockEditorInstance.setValue).toHaveBeenCalledWith('file content');
    });

    it('should save content', async () => {
        const mockFile = { id: '1', name: 'test.txt', isDir: false, path: '/test.txt', size: 0, modified: '', comments: '', extension: 'txt' };
        (vfs.read as any).mockResolvedValue('initial content');

        render(Editor);
        appState.setCurrentFile(mockFile);

        // Wait for load
        await waitFor(() => {
            expect(vfs.read).toHaveBeenCalled();
        });

        // Simulate dirty state (the component logic sets dirty on change, but we can't easily trigger the onChange callback from here without exposing it)
        // However, the save button logic checks `isDirty`. 
        // We can simulate the onChange callback if we captured it from the init call options.
        const initCall = (OverType.init as any).mock.calls[0];
        const options = initCall[1];

        // Simulate change
        options.onChange('new content'); // This sets isDirty = true

        await waitFor(() => {
            expect(screen.getByText('저장 (Cmd+S)')).not.toBeDisabled();
        });

        // Click save
        await fireEvent.click(screen.getByText('저장 (Cmd+S)'));

        expect(vfs.write).toHaveBeenCalledWith('1', 'current content');
    });
});
