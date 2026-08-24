<script lang="ts">
    import { appState } from "../state.svelte";

    let currentPassword = $state("");
    let newPassword = $state("");
    let confirmPassword = $state("");
    let error = $state("");

    async function submit(event: SubmitEvent) {
        event.preventDefault();
        error = "";
        if (newPassword !== confirmPassword) {
            error = "New passwords do not match";
            return;
        }
        const res = await fetch("/auth/password", {
            method: "PATCH",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify({ currentPassword, newPassword }),
        });
        if (!res.ok) {
            error = res.status === 401 ? "Current password is incorrect" : await res.text();
            return;
        }
        currentPassword = newPassword = confirmPassword = "";
        appState.addToast("Password changed", "success");
        appState.adminPage = null;
    }
</script>

<main class="flex-1 overflow-auto p-6">
    <form class="mx-auto max-w-md rounded bg-gray-800 p-6" onsubmit={submit}>
        <h2 class="mb-6 text-xl font-semibold text-white">Change password</h2>
        <label class="mb-4 block text-sm">
            Current password
            <input class="mt-1 w-full rounded bg-gray-900 px-3 py-2" type="password" bind:value={currentPassword} required />
        </label>
        <label class="mb-4 block text-sm">
            New password
            <input class="mt-1 w-full rounded bg-gray-900 px-3 py-2" type="password" bind:value={newPassword} required />
        </label>
        <label class="mb-4 block text-sm">
            Confirm new password
            <input class="mt-1 w-full rounded bg-gray-900 px-3 py-2" type="password" bind:value={confirmPassword} required />
        </label>
        {#if error}<p class="mb-4 text-sm text-red-400">{error}</p>{/if}
        <button class="rounded bg-blue-600 px-4 py-2 text-white hover:bg-blue-500" type="submit">Change password</button>
    </form>
</main>
