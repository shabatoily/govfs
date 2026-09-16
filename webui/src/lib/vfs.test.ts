import { afterEach, expect, it, vi } from 'vitest';
import vfs from './vfs';
import { inferType, isPreviewType } from './utils';

afterEach(() => vi.unstubAllGlobals());

it('explicitly requests conflict protection for WebUI transfers', async () => {
    const fetch = vi.fn().mockResolvedValue({ ok: true, status: 200, json: async () => ({ id: 'source' }) });
    vi.stubGlobal('fetch', fetch);
    await vfs.transfer('move', 'source', '/target');
    const [url, request] = fetch.mock.calls[0];
    expect(url).toBe('/vfs/source?wait=true');
    expect(JSON.parse(request.body)).toEqual({ name: '/target', checkConflict: true });
});


it.each([
    ['photo.PNG', 'image/png', true],
    ['clip.mp4', 'video/mp4', true],
    ['clip.mpg', 'video/mpeg', true],
    ['song.mp3', 'audio/mpeg', true],
    ['song.wav', 'audio/wav', true],
    ['document.pdf', 'application/pdf; charset=binary', true],
    ['data.json', 'application/json', false],
    ['data.xml', 'application/xml', false],
    ['script.js', 'text/javascript', false],
    ['notes.txt', 'text/plain; charset=utf-8', false],
    ['main.go', 'application/octet-stream', false],
    ['archive.zip', 'application/zip', false],
    ['README', '', false],
])('reads %s in the format required by its renderer', async (name, contentType, preview) => {
    const blob = new Blob(['content'], { type: contentType });
    const response = {
        ok: true,
        headers: new Headers({ 'content-type': contentType }),
        blob: vi.fn().mockResolvedValue(blob),
        text: vi.fn().mockResolvedValue('content'),
    };
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(response));

    expect(isPreviewType(inferType(name))).toBe(preview);
    expect(await vfs.read('file')).toBe(preview ? blob : 'content');
    expect(preview ? response.text : response.blob).not.toHaveBeenCalled();
});
