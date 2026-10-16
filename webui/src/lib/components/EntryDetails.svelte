<script lang="ts">
    import { appState } from "../state.svelte";
    import vfs, { type FileInfo } from "../vfs";
    import { formatBytes, inferType } from "../utils";

    let entry = $state<FileInfo | null>(null);
    let error = $state("");
    $effect(() => {
        const id = appState.selectedEntryId ?? appState.currentFile?.id;
        appState.refreshSignal;
        let active = true;
        entry = null;
        error = "";
        if (id) vfs.stat(id).then(value => {
            if (active) entry = value;
        }).catch(() => {
            if (active) error = "Unable to load entry details";
        });
        return () => { active = false; };
    });
</script>

<section aria-label="Entry details" class="min-h-0 flex items-center overflow-x-auto whitespace-nowrap bg-gray-800 px-4 text-xs border-t border-gray-700">
    {#if entry}
        <dl class="flex items-center gap-6">
            <div class="flex items-center gap-2"><dt class="text-gray-400">Path</dt><dd>{entry.path}</dd></div>
            <div class="flex items-center gap-2"><dt class="text-gray-400">Type</dt><dd>{entry.isDir ? "Directory" : inferType(entry.name)}</dd></div>
            <div class="flex items-center gap-2"><dt class="text-gray-400">Size</dt><dd>{entry.isDir ? "—" : `${formatBytes(entry.size)} (${entry.size} B)`}</dd></div>
            <div class="flex items-center gap-2"><dt class="text-gray-400">Modified</dt><dd>{entry.modified || "—"}</dd></div>
            <div class="flex items-center gap-2"><dt class="text-gray-400">ID</dt><dd>{entry.id}</dd></div>
        </dl>
    {:else}
        <p class="text-gray-400">{error || "Select an entry to view its details"}</p>
    {/if}
</section>
