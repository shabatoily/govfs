<script lang="ts">
    import { onMount } from "svelte";
    import { appState } from "../state.svelte";

    interface User {
        id: string;
        username: string;
        role: "admin" | "user";
        disabled: boolean;
    }

    let users = $state<User[]>([]);
    let username = $state("");
    let password = $state("");
    let role = $state<"admin" | "user">("user");
    let error = $state("");

    async function load() {
        const res = await fetch("/admin/users");
        if (!res.ok) throw new Error(await res.text());
        users = await res.json();
    }

    async function createUser() {
        error = "";
        const res = await fetch("/admin/users", {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify({ username, password, role }),
        });
        if (!res.ok) {
            error = await res.text();
            return;
        }
        username = "";
        password = "";
        role = "user";
        await load();
        appState.addToast("User created", "success");
    }

    async function toggleUser(user: User) {
        const res = await fetch(`/admin/users/${user.id}`, {
            method: "PATCH",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify({ disabled: !user.disabled }),
        });
        if (!res.ok) {
            error = await res.text();
            return;
        }
        await load();
    }

    onMount(() => {
        load().catch((e) => error = e.message);
    });
</script>

<div class="absolute inset-0 z-[150] bg-black/70 flex items-center justify-center">
    <div class="w-[40rem] max-h-[80vh] overflow-auto rounded-xl bg-gray-800 border border-gray-700 p-6">
        <div class="flex justify-between items-center mb-5">
            <h2 class="text-xl font-bold text-white">Users</h2>
            <button class="text-gray-300 hover:text-white" onclick={() => appState.showUserAdmin = false}>Close</button>
        </div>
        {#if error}<p class="mb-3 text-sm text-red-300">{error}</p>{/if}
        <form class="grid grid-cols-[1fr_1fr_auto_auto] gap-2 mb-5" onsubmit={(e) => { e.preventDefault(); createUser(); }}>
            <input class="bg-gray-900 rounded px-3 py-2" placeholder="Username" bind:value={username} required />
            <input class="bg-gray-900 rounded px-3 py-2" placeholder="Password" type="password" bind:value={password} required />
            <select class="bg-gray-900 rounded px-3 py-2" bind:value={role}>
                <option value="user">User</option>
                <option value="admin">Admin</option>
            </select>
            <button class="bg-blue-600 rounded px-4 py-2 text-white">Add</button>
        </form>
        <div class="space-y-2">
            {#each users as user}
                <div class="flex items-center justify-between rounded bg-gray-900 px-4 py-3">
                    <div>
                        <span class="text-white">{user.username}</span>
                        <span class="ml-2 text-xs text-gray-400">{user.role}</span>
                    </div>
                    <button class="text-sm {user.disabled ? 'text-green-300' : 'text-red-300'}" onclick={() => toggleUser(user)}>
                        {user.disabled ? "Enable" : "Disable"}
                    </button>
                </div>
            {/each}
        </div>
    </div>
</div>
