import { render, screen } from '@testing-library/svelte';
import { afterEach, describe, expect, it, vi } from 'vitest';
import AdminStatus from './AdminStatus.svelte';

describe('AdminStatus Component', () => {
    afterEach(() => vi.unstubAllGlobals());

    it('should render open Badger drive resources', async () => {
        vi.stubGlobal('fetch', vi.fn(async (input: string) => ({
            ok: input === '/admin/status',
            json: async () => ({
                users: 1,
                openDrives: 1,
                system: { items: 2, size: 3 },
                badgerDrives: [{
                    userId: 'user-1',
                    lsmSize: 1024 * 1024,
                    vlogSize: 2 * 1024 * 1024,
                    blockCacheMaxCost: 256 * 1024 * 1024,
                    indexCacheMaxCost: 100 * 1024 * 1024,
                }],
            }),
        })));

        render(AdminStatus);

        expect(await screen.findByText('user-1')).toBeInTheDocument();
        expect(screen.getByText('1.00 MiB')).toBeInTheDocument();
        expect(screen.getByText('256.00 MiB')).toBeInTheDocument();
        expect(screen.getByText('100.00 MiB')).toBeInTheDocument();
    });
});
