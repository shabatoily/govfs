<script lang="ts">
    import { onMount, onDestroy } from "svelte";
    import {
        FilePlus,
        FolderPlus,
        RefreshCw,
        Home,
        LogOut,
        Trash2,
        ServerCog,
        Users,
    } from "lucide-svelte";
    import { appState } from "../state.svelte";
    import vfs, { type FileInfo } from "../vfs";
    import { getParentPath, isAncestorOrSame, resolvePath } from "../utils";
    import FileTreeItem from "./FileTreeItem.svelte";

    let rootFiles = $state<FileInfo[]>([]);
    let loading = $state(false);
    let treeElement = $state<HTMLDivElement>();
    let selectedItems = $state<Map<string, Pick<FileInfo, "id" | "path" | "name" | "isDir">>>(new Map());
    let focusedId = $state<string | null>(null);
    let selectionAnchorId = $state<string | null>(null);

    const selectedIds = $derived(new Set(selectedItems.keys()));

    function visibleItems() {
        return Array.from(
            treeElement?.querySelectorAll<HTMLElement>("[data-tree-item]") ?? [],
        );
    }

    function itemFromElement(element: HTMLElement) {
        return {
            id: element.dataset.fileId!,
            path: element.dataset.path!,
            name: element.dataset.name!,
            isDir: element.dataset.isDir === "true",
        };
    }

    function focusItem(id: string) {
        focusedId = id;
        requestAnimationFrame(() => {
            const item = visibleItems().find((element) => element.dataset.fileId === id);
            item?.focus();
            item?.scrollIntoView({ block: "nearest" });
        });
    }

    function selectItem(file: FileInfo, event: MouseEvent) {
        const toggle = event.metaKey || event.ctrlKey;

        if (event.shiftKey && selectionAnchorId) {
            const items = visibleItems();
            const start = items.findIndex((item) => item.dataset.fileId === selectionAnchorId);
            const end = items.findIndex((item) => item.dataset.fileId === file.id);
            if (start !== -1 && end !== -1) {
                const next = toggle ? new Map(selectedItems) : new Map();
                for (const item of items.slice(Math.min(start, end), Math.max(start, end) + 1)) {
                    const selected = itemFromElement(item);
                    next.set(selected.id, selected);
                }
                selectedItems = next;
            }
        } else if (toggle) {
            const next = new Map(selectedItems);
            if (next.has(file.id)) next.delete(file.id);
            else next.set(file.id, file);
            selectedItems = next;
            selectionAnchorId = file.id;
        } else {
            selectedItems = new Map([[file.id, file]]);
            selectionAnchorId = file.id;
        }

        focusItem(file.id);
    }

    function selectElement(element: HTMLElement, extend: boolean) {
        const item = itemFromElement(element);
        if (extend && selectionAnchorId) {
            const items = visibleItems();
            const start = items.findIndex((row) => row.dataset.fileId === selectionAnchorId);
            const end = items.indexOf(element);
            const next = new Map<string, typeof item>();
            for (const row of items.slice(Math.min(start, end), Math.max(start, end) + 1)) {
                const selected = itemFromElement(row);
                next.set(selected.id, selected);
            }
            selectedItems = next;
        } else {
            selectedItems = new Map([[item.id, item]]);
            selectionAnchorId = item.id;
        }
        focusItem(item.id);
    }

    async function deleteSelected(fallback?: FileInfo) {
        const candidates = fallback && !selectedItems.has(fallback.id)
            ? [fallback]
            : Array.from(selectedItems.values());
        if (candidates.length === 0) return;

        const targets = candidates.filter(
            (item) => !candidates.some(
                (parent) => parent.isDir && parent.id !== item.id && isAncestorOrSame(parent.path, item.path),
            ),
        );
        const label = candidates.length === 1 ? candidates[0].name : `${candidates.length} items`;
        if (!confirm(`Delete ${label}?`)) return;

        try {
            await Promise.all(targets.map((item) => vfs.delete(item.id)));
            const deletedIds = new Set(candidates.map((item) => item.id));
            if (appState.currentFile && deletedIds.has(appState.currentFile.id)) {
                appState.setCurrentFile(null);
            }
            for (const item of targets) {
                if (item.isDir && isAncestorOrSame(item.path, appState.currentPath)) {
                    appState.setCurrentPath(getParentPath(item.path));
                }
            }
            selectedItems = new Map();
            focusedId = null;
            selectionAnchorId = null;
            await loadRootFiles();
        } catch (error: any) {
            appState.addToast(error.message ?? "Failed to delete items", "error");
        }
    }

    function handleTreeKeydown(event: KeyboardEvent) {
        const items = visibleItems();
        if (items.length === 0) return;
        const current = Math.max(0, items.findIndex((item) => item.dataset.fileId === focusedId));

        if (event.key === "ArrowUp" || event.key === "ArrowDown") {
            event.preventDefault();
            const offset = event.key === "ArrowUp" ? -1 : 1;
            selectElement(items[Math.max(0, Math.min(items.length - 1, current + offset))], event.shiftKey);
        } else if (event.key === "Enter") {
            event.preventDefault();
            items[current].click();
        } else if (event.key === "ArrowRight") {
            event.preventDefault();
            if (items[current].dataset.isDir === "true" && items[current].dataset.expanded !== "true") {
                items[current].querySelector<HTMLElement>("[data-expand]")?.click();
            }
        } else if (event.key === "ArrowLeft") {
            event.preventDefault();
            if (items[current].dataset.expanded === "true") {
                items[current].querySelector<HTMLElement>("[data-expand]")?.click();
            } else {
                const depth = Number(items[current].dataset.depth);
                const parent = items.slice(0, current).reverse().find((item) => Number(item.dataset.depth) < depth);
                if (parent) selectElement(parent, false);
            }
        } else if (event.key === "Delete" || event.key === "Backspace") {
            event.preventDefault();
            deleteSelected();
        }
    }

    // Track component instances to call refresh on them if needed
    // Actually, for root, we just reload rootFiles.
    // For deeper updates, we rely on the parent FileTreeItem's onRefresh callback.

    async function handleRefresh() {
        loading = true;
        try {
            await Promise.all([loadRootFiles(), appState.refresh()]);
        } finally {
            loading = false;
        }
    }

    function handleLogout() {
        appState.logout();
    }

    async function loadRootFiles() {
        try {
            const files = await vfs.list("/");
            files.sort((a, b) => {
                if (a.isDir === b.isDir) return a.name.localeCompare(b.name);
                return a.isDir ? -1 : 1;
            });
            rootFiles = files;
            if (!focusedId && files.length > 0) focusedId = files[0].id;
        } catch (e) {
            console.error(e);
            appState.addToast("Failed to load root files", "error");
        }
    }

    async function goHome() {
        appState.setCurrentPath("/");
        await loadRootFiles();
    }

    async function createNewFile() {
        // Create in current path
        const currentPath =
            appState.currentPath === "" ? "/" : appState.currentPath;

        const name = prompt("New File Name:", "untitled.md");
        if (!name) return;
        const path = resolvePath(currentPath, name);
        try {
            await vfs.create(path, "# New Document");
        } catch (e: any) {
            alert(e.message);
        }
    }

    async function createNewDir() {
        const currentPath =
            appState.currentPath === "" ? "/" : appState.currentPath;

        const name = prompt("New Folder Name:", "new-folder");
        if (!name) return;
        const path = resolvePath(currentPath, name);
        try {
            await vfs.mkdir(path);
        } catch (e: any) {
            alert(e.message);
        }
    }

    onMount(() => {
        loadRootFiles();
    });

    // Remove SSE direct handling
    // We rely on appState signals now.

    $effect(() => {
        const signal = appState.refreshSignal;
        if (!signal) return;

        // If root path needs refresh
        if (signal.type === "PATH") {
            if (signal.value === "/") {
                loadRootFiles();
            }
        } else if (signal.type === "ID") {
            // If the deleted ID is one of root files?
            if (rootFiles.some((f) => f.id === signal.value)) {
                loadRootFiles();
            }
        }
    });
    const refreshIconClass = $derived(loading ? "animate-spin" : "");
</script>

<div
    class="h-full flex flex-col bg-gray-900 border-r border-gray-700 w-64 flex-shrink-0 select-none"
>
    <!-- Header -->
    <div
        class="h-10 border-b border-gray-700 flex items-center px-4 justify-between bg-gray-800"
    >
        <button class="text-xs font-bold text-gray-400 hover:text-white" onclick={() => appState.adminPage = null}>EXPLORER</button>
        <div class="flex gap-1">
            <button
                onclick={goHome}
                class="p-1 hover:bg-gray-700 rounded text-gray-400 hover:text-white transition-colors"
                title="Go to Root"
            >
                <Home size={16} />
            </button>
            <button
                onclick={createNewFile}
                class="p-1 hover:bg-gray-700 rounded text-gray-400 hover:text-white transition-colors"
                title="New File"
            >
                <FilePlus size={16} />
            </button>
            <button
                onclick={createNewDir}
                class="p-1 hover:bg-gray-700 rounded text-gray-400 hover:text-white transition-colors"
                title="New Folder"
            >
                <FolderPlus size={16} />
            </button>
            <button
                onclick={handleRefresh}
                class="p-1 hover:bg-gray-700 rounded text-gray-400 hover:text-white transition-colors"
                title="Refresh"
            >
                <RefreshCw size={16} class={refreshIconClass} />
            </button>
            <button
                onclick={() => deleteSelected()}
                disabled={selectedItems.size === 0}
                class="p-1 hover:bg-red-500 rounded text-gray-400 hover:text-white transition-colors disabled:opacity-30 disabled:pointer-events-none"
                title="Delete Selected"
            >
                <Trash2 size={16} />
            </button>
            <div class="w-px h-4 bg-gray-600 mx-1 self-center"></div>
            <button
                onclick={handleLogout}
                class="p-1 hover:bg-red-500 rounded text-gray-400 hover:text-white transition-colors"
                title="Logout"
            >
                <LogOut size={16} />
            </button>
        </div>
    </div>

    <!-- File List Tree -->
    <div bind:this={treeElement} onkeydown={handleTreeKeydown} class="flex-1 overflow-y-auto p-2" role="tree" aria-multiselectable="true" tabindex="-1">
        <ul class="space-y-0.5">
            {#each rootFiles as file (file.id)}
                <FileTreeItem
                    {file}
                    {selectedIds}
                    {focusedId}
                    onSelect={selectItem}
                    onDelete={deleteSelected}
                    onRefresh={loadRootFiles}
                />
            {/each}
        </ul>

        {#if rootFiles.length === 0 && !loading}
            <div class="text-center mt-10 text-xs text-gray-500">
                No files found
            </div>
        {/if}
    </div>

    {#if appState.currentUser?.role === "admin"}
        <div class="border-t border-gray-700 bg-gray-800 p-2">
            <div class="mb-1 px-2 text-[10px] font-bold tracking-wider text-gray-500">ADMIN</div>
            <button
                onclick={() => appState.adminPage = "server"}
                class="flex w-full items-center gap-2 rounded px-2 py-1.5 text-left text-xs text-gray-300 hover:bg-gray-700 hover:text-white"
            >
                <ServerCog size={15} /> Server status
            </button>
            <button
                onclick={() => appState.adminPage = "users"}
                class="flex w-full items-center gap-2 rounded px-2 py-1.5 text-left text-xs text-gray-300 hover:bg-gray-700 hover:text-white"
            >
                <Users size={15} /> User management
            </button>
        </div>
    {/if}

    <!-- Path Status -->
    <div
        class="border-t border-gray-700 px-2 py-1 text-xs text-gray-500 truncate bg-gray-950"
        title={appState.currentPath}
    >
        {appState.currentPath || "/"}
    </div>
</div>
