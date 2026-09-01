import { render, screen, fireEvent, waitFor } from '@testing-library/svelte';
import { describe, it, expect, vi, beforeEach } from 'vitest';
import Terminal from './Terminal.svelte';
import vfs from '../vfs';
import { appState } from '../state.svelte';

// Mock vfs
vi.mock('../vfs', () => ({
    default: {
        list: vi.fn(),
        mkdir: vi.fn(),
        delete: vi.fn(),
        tree: vi.fn(),
        search: vi.fn(),
    }
}));

describe('Terminal Component', () => {
    beforeEach(() => {
        vi.clearAllMocks();
        appState.setCurrentPath('/');
    });

    it('should render input prompt', () => {
        render(Terminal);
        expect(screen.getByPlaceholderText('Type help for commands...')).toBeInTheDocument();
    });

    it('should handle "pwd" command', async () => {
        render(Terminal);
        const input = screen.getByRole('textbox');

        await fireEvent.input(input, { target: { value: 'pwd' } });
        await fireEvent.keyDown(input, { key: 'Enter' });

        await waitFor(() => {
            expect(screen.getByText('/')).toBeInTheDocument();
        });
    });

    it('should handle "help" command', async () => {
        render(Terminal);
        const input = screen.getByRole('textbox');

        await fireEvent.input(input, { target: { value: 'help' } });
        await fireEvent.keyDown(input, { key: 'Enter' });

        await waitFor(() => {
            expect(screen.getByText(/ls <path> show file list/)).toBeInTheDocument();
        });
    });

    it('should handle "mkdir" command', async () => {
        (vfs.mkdir as any).mockResolvedValue({});
        (vfs.list as any).mockResolvedValue([]);

        render(Terminal);
        const input = screen.getByRole('textbox');

        await fireEvent.input(input, { target: { value: 'mkdir testdir' } });
        await fireEvent.keyDown(input, { key: 'Enter' });

        await waitFor(() => {
            expect(vfs.mkdir).toHaveBeenCalledWith('/testdir');
            expect(screen.getByText('creating directory: testdir')).toBeInTheDocument();
        });
    });

    it('should handle "ls" command', async () => {
        (vfs.list as any).mockResolvedValue([
            { id: '1', name: 'file1', isDir: false, size: 100, modified: 'now' }
        ]);

        render(Terminal);
        const input = screen.getByRole('textbox');

        await fireEvent.input(input, { target: { value: 'ls' } });
        await fireEvent.keyDown(input, { key: 'Enter' });

        await waitFor(() => {
            expect(vfs.list).toHaveBeenCalledWith('/');
            expect(screen.getByText(/- 100 now file1/)).toBeInTheDocument();
        });
    });

    it('should handle "find" command', async () => {
        (vfs.search as any).mockResolvedValue([
            { name: 'report.txt', path: '/docs/report.txt' },
        ]);
        render(Terminal);
        const input = screen.getByRole('textbox');

        await fireEvent.input(input, { target: { value: 'find report' } });
        await fireEvent.keyDown(input, { key: 'Enter' });

        await waitFor(() => {
            expect(vfs.search).toHaveBeenCalledWith('report');
            expect(screen.getByText('/docs/report.txt')).toBeInTheDocument();
        });
    });
});
