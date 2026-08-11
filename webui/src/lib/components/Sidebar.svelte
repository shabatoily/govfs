<script lang="ts">
    import { onMount, onDestroy } from "svelte";
    import {
        FilePlus,
        FolderPlus,
        RefreshCw,
        Home,
        LogOut,
    } from "lucide-svelte";
    import { appState } from "../state.svelte";
    import vfs, { type FileInfo } from "../vfs";
    import { resolvePath } from "../utils";
    import FileTreeItem from "./FileTreeItem.svelte";

    let rootFiles = $state<FileInfo[]>([]);
    let loading = $state(false);

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
            const file = await vfs.create(path, "# New Document");

            // We need to refresh the folder where this was created.
            // If it's root, refresh root.
            if (currentPath === "/") {
                await loadRootFiles();
            } else {
                // Trigger global refresh for specific path
                appState.triggerRefreshPath(currentPath);
            }
            appState.setCurrentFile(file);
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
            if (currentPath === "/") {
                await loadRootFiles();
            } else {
                appState.triggerRefreshPath(currentPath);
            }
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
        <span class="text-xs font-bold text-gray-400">EXPLORER</span>
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
    <div class="flex-1 overflow-y-auto p-2">
        <ul class="space-y-0.5">
            {#each rootFiles as file (file.id)}
                <FileTreeItem {file} onRefresh={loadRootFiles} />
            {/each}
        </ul>

        {#if rootFiles.length === 0 && !loading}
            <div class="text-center mt-10 text-xs text-gray-500">
                No files found
            </div>
        {/if}
    </div>

    <!-- Path Status -->
    <div
        class="border-t border-gray-700 px-2 py-1 text-xs text-gray-500 truncate bg-gray-950"
        title={appState.currentPath}
    >
        {appState.currentPath || "/"}
    </div>
</div>
