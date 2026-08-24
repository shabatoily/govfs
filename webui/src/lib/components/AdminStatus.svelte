<script lang="ts">
    import { onMount } from "svelte";

    interface Status {
        users: number;
        openDrives: number;
        system: { items: number; size: number };
    }

    interface SystemEntry {
        key: string;
        kind: string;
        value: unknown;
    }

    interface SystemEntryPage {
        items: SystemEntry[];
        page: number;
        pageSize: number;
        total: number;
    }

    let status = $state<Status | null>(null);
    let diagnostics = $state<Record<string, unknown>>({});
    let error = $state("");
    let entries = $state<SystemEntry[]>([]);
    let entriesVisible = $state(false);
    let entryPage = $state(1);
    let entryTotal = $state(0);
    const entryPageSize = 20;

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

    async function loadEntries(page: number) {
        const res = await fetch(`/admin/system/entries?page=${page}&pageSize=${entryPageSize}`);
        if (!res.ok) throw new Error(await res.text());
        const data: SystemEntryPage = await res.json();
        entries = data.items;
        entryPage = data.page;
        entryTotal = data.total;
        entriesVisible = true;
    }

    onMount(() => load().catch((e) => error = e.message));
</script>

<div class="h-full overflow-auto bg-gray-800 p-6">
    <div class="mx-auto max-w-5xl">
        <h2 class="mb-5 text-xl font-bold text-white">Server status</h2>
        {#if error}<p class="mb-3 text-sm text-red-300">{error}</p>{/if}
        {#if status}
            <div class="grid grid-cols-4 gap-3 mb-5">
                <div class="rounded bg-gray-900 p-3">Users<br /><strong>{status.users}</strong></div>
                <div class="rounded bg-gray-900 p-3">Open drives<br /><strong>{status.openDrives}</strong></div>
                <div class="rounded bg-gray-900 p-3">System items<br /><strong>{status.system.items}</strong></div>
                <div class="rounded bg-gray-900 p-3">System size<br /><strong>{formatBytes(status.system.size)}</strong></div>
            </div>
            <div class="mb-5">
                <button class="text-sm text-blue-300 hover:text-blue-200" onclick={() => loadEntries(1).catch((e) => error = e.message)}>System DB details</button>
            </div>
            {#if entriesVisible}
                <div class="mb-5 overflow-hidden rounded bg-gray-900 text-sm">
                    {#each entries as entry}
                        <div class="border-b border-gray-700 p-3 last:border-0">
                            <div class="mb-1 flex gap-3"><span class="text-blue-300">{entry.kind}</span><code class="break-all">{entry.key}</code></div>
                            <pre class="overflow-auto text-xs text-gray-400">{JSON.stringify(entry.value, null, 2)}</pre>
                        </div>
                    {/each}
                    <div class="flex items-center justify-end gap-3 p-3">
                        <span class="text-gray-400">{entryTotal} keys · page {entryPage}</span>
                        <button class="text-blue-300 disabled:text-gray-600" disabled={entryPage <= 1} onclick={() => loadEntries(entryPage - 1).catch((e) => error = e.message)}>Previous</button>
                        <button class="text-blue-300 disabled:text-gray-600" disabled={entryPage * entryPageSize >= entryTotal} onclick={() => loadEntries(entryPage + 1).catch((e) => error = e.message)}>Next</button>
                    </div>
                </div>
            {/if}
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
