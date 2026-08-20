<script lang="ts">
    import { onMount } from "svelte";
    import { appState } from "../state.svelte";

    interface User {
        id: string;
        username: string;
        role: "admin" | "user";
        disabled: boolean;
    }

    interface UserEvent {
        id: string;
        username: string;
        action: string;
        status: number;
        createdAt: string;
    }

	interface DriveStatus {
		userId: string;
		open: boolean;
		online: boolean;
		sseCount: number;
		items: number;
		size: number;
	}

    let users = $state<User[]>([]);
	let events = $state<UserEvent[]>([]);
	let drives = $state<DriveStatus[]>([]);
	let selected = $state<User | null>(null);
    let username = $state("");
    let password = $state("");
    let role = $state<"admin" | "user">("user");
    let error = $state("");

    async function load() {
		const [usersRes, statusRes] = await Promise.all([fetch("/admin/users"), fetch("/admin/status")]);
		if (!usersRes.ok) throw new Error(await usersRes.text());
		if (!statusRes.ok) throw new Error(await statusRes.text());
		users = await usersRes.json();
		drives = (await statusRes.json()).drives;
    }

	async function showDetails(user: User) {
		selected = user;
		const res = await fetch(`/admin/events?userId=${user.id}`);
		if (!res.ok) throw new Error(await res.text());
		events = await res.json();
	}

	function formatBytes(value: number) {
		if (value < 1024) return `${value} B`;
		if (value < 1024 * 1024) return `${(value / 1024).toFixed(2)} KiB`;
		return `${(value / 1024 / 1024).toFixed(2)} MiB`;
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
					<div class="flex gap-3">
						<button class="text-sm text-blue-300" onclick={() => showDetails(user).catch((e) => error = e.message)}>Details</button>
						<button class="text-sm {user.disabled ? 'text-green-300' : 'text-red-300'}" onclick={() => toggleUser(user)}>
							{user.disabled ? "Enable" : "Disable"}
						</button>
					</div>
				</div>
			{/each}
		</div>
		{#if selected}
		{@const drive = drives.find((item) => item.userId === selected?.id)}
		<h3 class="mt-6 mb-2 font-semibold text-white">{selected.username} details</h3>
		<div class="mb-3 grid grid-cols-4 gap-2 text-sm">
			<div class="rounded bg-gray-900 p-3">Items<br /><strong>{drive?.items ?? 0}</strong></div>
			<div class="rounded bg-gray-900 p-3">Data size<br /><strong>{formatBytes(drive?.size ?? 0)}</strong></div>
			<div class="rounded bg-gray-900 p-3">Badger drive<br /><strong class={drive?.open ? "text-green-300" : "text-gray-400"}>{drive?.open ? "Open" : "Closed"}</strong></div>
			<div class="rounded bg-gray-900 p-3">SSE<br /><strong class={drive?.online ? "text-green-300" : "text-gray-400"}>{drive?.online ? `Online (${drive.sseCount})` : "Offline"}</strong></div>
		</div>
		<h3 class="mb-2 font-semibold text-white">Recent activity</h3>
		<div class="space-y-1 text-sm">
			{#each events as event}
				<div class="grid grid-cols-[10rem_1fr_auto] gap-3 rounded bg-gray-900 px-3 py-2">
					<span class="text-gray-400">{new Date(event.createdAt).toLocaleString()}</span>
					<span>{event.username} · {event.action}</span>
					<span class={event.status < 400 ? "text-green-300" : "text-red-300"}>{event.status}</span>
				</div>
			{/each}
		</div>
		{/if}
    </div>
</div>
