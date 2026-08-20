<script lang="ts">
    import { onMount } from "svelte";
    import { appState } from "../state.svelte";

    interface Status {
        users: number;
        openDrives: number;
        system: { items: number; size: number };
        drives: { userId: string; username: string; open: boolean; online: boolean; sseCount: number; items: number; size: number }[];
    }

    let status = $state<Status | null>(null);
    let diagnostics = $state<Record<string, unknown>>({});
    let error = $state("");

    async function optionalJSON(path: string) {
        const res = await fetch(path);
        return res.ok ? await res.json() : "Unavailable";
    }

    async function load() {
        const res = await fetch("/admin/status");
        if (!res.ok) throw new Error(await res.text());
        status = await res.json();
        const [config, routes, variables] = await Promise.all([
            optionalJSON("/config"),
            optionalJSON("/routes"),
            optionalJSON("/debug/vars"),
        ]);
        diagnostics = { config, routes, variables };
    }

    function formatBytes(value: number) {
        if (value < 1024) return `${value} B`;
        if (value < 1024 * 1024) return `${(value / 1024).toFixed(2)} KiB`;
        return `${(value / 1024 / 1024).toFixed(2)} MiB`;
    }

    onMount(() => load().catch((e) => error = e.message));
</script>

<div class="absolute inset-0 z-[150] bg-black/70 flex items-center justify-center">
    <div class="w-[56rem] max-h-[85vh] overflow-auto rounded-xl bg-gray-800 border border-gray-700 p-6">
        <div class="flex justify-between items-center mb-5">
            <h2 class="text-xl font-bold text-white">Server status</h2>
            <button class="text-gray-300 hover:text-white" onclick={() => appState.showServerStatus = false}>Close</button>
        </div>
        {#if error}<p class="mb-3 text-sm text-red-300">{error}</p>{/if}
        {#if status}
            <div class="grid grid-cols-4 gap-3 mb-5">
                <div class="rounded bg-gray-900 p-3">Users<br /><strong>{status.users}</strong></div>
                <div class="rounded bg-gray-900 p-3">Open drives<br /><strong>{status.openDrives}</strong></div>
                <div class="rounded bg-gray-900 p-3">System items<br /><strong>{status.system.items}</strong></div>
                <div class="rounded bg-gray-900 p-3">System size<br /><strong>{formatBytes(status.system.size)}</strong></div>
            </div>
            <h3 class="mb-2 font-semibold text-white">User drives</h3>
            <div class="space-y-1 mb-5 text-sm">
                {#each status.drives as drive}
                    <div class="grid grid-cols-[1fr_auto_auto_auto_auto] gap-5 rounded bg-gray-900 px-3 py-2">
                        <span>{drive.username}</span>
						<span class={drive.online ? "text-green-300" : "text-gray-400"}>{drive.online ? `Online (${drive.sseCount})` : "Offline"}</span>
						<span>{drive.open ? "DB open" : "DB closed"}</span>
						<span>{drive.items} items</span><span>{formatBytes(drive.size)}</span>
                    </div>
                {/each}
            </div>
            <div class="flex gap-3 mb-3 text-sm">
                <a class="text-blue-300" href="/debug/pprof/" target="_blank">pprof</a>
                <a class="text-blue-300" href="/debug/vars" target="_blank">expvar</a>
                <a class="text-blue-300" href="/config" target="_blank">config</a>
                <a class="text-blue-300" href="/routes" target="_blank">routes</a>
            </div>
            {#each Object.entries(diagnostics) as [name, value]}
                <details class="mb-2 rounded bg-gray-900 p-3">
                    <summary class="cursor-pointer text-white">{name}</summary>
                    <pre class="mt-2 overflow-auto text-xs">{JSON.stringify(value, null, 2)}</pre>
                </details>
            {/each}
        {/if}
    </div>
</div>
