import { render, screen, fireEvent, waitFor } from '@testing-library/svelte';
import { describe, it, expect, vi, afterEach } from 'vitest';
import ChangePassword from './ChangePassword.svelte';
import { appState } from '../state.svelte';

vi.mock('../sse', () => ({ default: { disconnect: vi.fn() } }));

describe('ChangePassword', () => {
    afterEach(() => vi.unstubAllGlobals());

    it('ends the old session after a successful password change', async () => {
        const fetchMock = vi.fn().mockResolvedValue(new Response(null, { status: 204 }));
        vi.stubGlobal('fetch', fetchMock);
        appState.isLoggedIn = true;
        appState.currentUser = { id: 'user', username: 'user', role: 'user' };
        render(ChangePassword);
        await fireEvent.input(screen.getByLabelText('Current password'), { target: { value: 'old-password' } });
        await fireEvent.input(screen.getByLabelText('New password'), { target: { value: 'new-password' } });
        await fireEvent.input(screen.getByLabelText('Confirm new password'), { target: { value: 'new-password' } });
        await fireEvent.click(screen.getByRole('button', { name: 'Change password' }));
        await waitFor(() => expect(appState.isLoggedIn).toBe(false));
        expect(appState.currentUser).toBeNull();
        expect(fetchMock).toHaveBeenCalledWith('/auth/logout', expect.objectContaining({ method: 'POST' }));
    });
});
