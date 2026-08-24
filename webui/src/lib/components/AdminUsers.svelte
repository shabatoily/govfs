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

    interface EventPage {
        items: UserEvent[];
        page: number;
        pageSize: number;
        total: number;
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
	let drive = $state<DriveStatus | null>(null);
	let selected = $state<User | null>(null);
	let activityVisible = $state(false);
	let eventPage = $state(1);
	let eventTotal = $state(0);
	const eventPageSize = 20;
    let username = $state("");
    let password = $state("");
    let role = $state<"admin" | "user">("user");
    let error = $state("");

    async function load() {
		const usersRes = await fetch("/admin/users");
		if (!usersRes.ok) throw new Error(await usersRes.text());
		users = await usersRes.json();
    }

	async function showDetails(user: User) {
		selected = user;
		activityVisible = true;
		const statusRes = await fetch(`/admin/users/${user.id}/status`);
		if (!statusRes.ok) throw new Error(await statusRes.text());
		drive = await statusRes.json();
		await loadEvents(1);
	}

	async function loadEvents(page: number) {
		const user = selected ? `&userId=${selected.id}` : "";
		const res = await fetch(`/admin/events?page=${page}&pageSize=${eventPageSize}${user}`);
		if (!res.ok) throw new Error(await res.text());
		const data: EventPage = await res.json();
		events = data.items;
		eventPage = data.page;
		eventTotal = data.total;
	}

	async function showAllActivity() {
		selected = null;
		drive = null;
		activityVisible = true;
		await loadEvents(1);
	}

	async function clearEvents() {
		if (!selected || !confirm(`Clear all activity for ${selected.username}?`)) return;
		const res = await fetch(`/admin/users/${selected.id}/events`, { method: "DELETE" });
		if (!res.ok) throw new Error(await res.text());
		await loadEvents(1);
		appState.addToast("Activity cleared", "success");
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

<div class="h-full overflow-auto bg-gray-800 p-6">
    <div class="mx-auto max-w-5xl">
        <div class="mb-5 flex items-center justify-between">
			<h2 class="text-xl font-bold text-white">User management</h2>
			<button class="text-sm text-blue-300 hover:text-blue-200" onclick={() => showAllActivity().catch((e) => error = e.message)}>All activity</button>
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
		<h3 class="mt-6 mb-2 font-semibold text-white">{selected.username} details</h3>
		<div class="mb-3 grid grid-cols-4 gap-2 text-sm">
			<div class="rounded bg-gray-900 p-3">Items<br /><strong>{drive?.items ?? 0}</strong></div>
			<div class="rounded bg-gray-900 p-3">Data size<br /><strong>{formatBytes(drive?.size ?? 0)}</strong></div>
			<div class="rounded bg-gray-900 p-3">Badger drive<br /><strong class={drive?.open ? "text-green-300" : "text-gray-400"}>{drive?.open ? "Open" : "Closed"}</strong></div>
			<div class="rounded bg-gray-900 p-3">SSE<br /><strong class={drive?.online ? "text-green-300" : "text-gray-400"}>{drive?.online ? `Online (${drive.sseCount})` : "Offline"}</strong></div>
		</div>
		{/if}
		{#if activityVisible}
		<div class="mt-6 mb-2 flex items-center justify-between">
			<h3 class="font-semibold text-white">{selected ? `${selected.username} activity` : "All activity"}</h3>
			{#if selected}
				<button class="text-sm text-red-300 hover:text-red-200" onclick={() => clearEvents().catch((e) => error = e.message)}>Clear activity</button>
			{/if}
		</div>
		<div class="space-y-1 text-sm">
			{#each events as event}
				<div class="grid grid-cols-[10rem_1fr_auto] gap-3 rounded bg-gray-900 px-3 py-2">
					<span class="text-gray-400">{new Date(event.createdAt).toLocaleString()}</span>
					<span>{event.username} · {event.action}</span>
					<span class={event.status < 400 ? "text-green-300" : "text-red-300"}>{event.status}</span>
				</div>
			{/each}
		</div>
		<div class="mt-3 flex items-center justify-end gap-3 text-sm">
			<span class="text-gray-400">{eventTotal} events · page {eventPage}</span>
			<button class="text-blue-300 disabled:text-gray-600" disabled={eventPage <= 1} onclick={() => loadEvents(eventPage - 1).catch((e) => error = e.message)}>Previous</button>
			<button class="text-blue-300 disabled:text-gray-600" disabled={eventPage * eventPageSize >= eventTotal} onclick={() => loadEvents(eventPage + 1).catch((e) => error = e.message)}>Next</button>
		</div>
		{/if}
    </div>
</div>
