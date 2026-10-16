import { describe, it, expect } from 'vitest';
import { treeSize, formatBytes } from './utils';

describe('drive size', () => {
    it('counts nested files without counting directory metadata', () => {
        const file = { id: '1', path: '/a', name: 'a', extension: '', size: 1024, isDir: false, modified: '', comments: '' };
        expect(treeSize({ meta: { ...file, isDir: true, size: 4096 }, children: [
            { meta: file, children: [] },
            { meta: { ...file, isDir: true }, children: [{ meta: file, children: [] }] },
        ] })).toBe(2048);
        expect(formatBytes(0)).toBe('0 B');
        expect(formatBytes(2048)).toBe('2 KiB');
    });
});
