<script lang="ts">
    import {
        Folder,
        File as FileIcon,
        ChevronRight,
        ChevronDown,
        Pencil,
        Trash2,
        LoaderCircle,
    } from "lucide-svelte";
    import vfs, { type FileInfo } from "../vfs";
    import { appState } from "../state.svelte";
    import {
        resolvePath,
        isAncestorOrSame,
        getParentPath,
        normalizePath,
    } from "../utils";
    import FileTreeItem from "./FileTreeItem.svelte";

    interface Props {
        file: FileInfo;
        depth?: number;
        onRefresh?: () => void;
    }

    let { file, depth = 0, onRefresh }: Props = $props();

    let expanded = $state(false);
    let children = $state<FileInfo[]>([]);
    let loading = $state(false);
    let loaded = $state(false); // To know if we already fetched

    // Derived state for padding based on depth
    const paddingLeft = $derived(`${depth * 12 + 8}px`);

    async function toggleExpand(e: Event) {
        e.stopPropagation();
        if (!file.isDir) return;

        expanded = !expanded;

        if (expanded && !loaded) {
            await loadChildren();
        }
    }

    async function loadChildren() {
        loading = true;
        try {
            children = await vfs.list(file.path);
            children.sort((a, b) => {
                if (a.isDir === b.isDir) return a.name.localeCompare(b.name);
                return a.isDir ? -1 : 1;
            });
            loaded = true;
        } catch (e) {
            console.error(e);
            appState.addToast("Failed to load folder", "error");
        } finally {
            loading = false;
        }
    }

    function handleClick(e: Event) {
        e.stopPropagation();
        if (file.isDir) {
            toggleExpand(e);
            // Also set current path to this folder for context
            appState.setCurrentPath(file.path);
            appState.setCurrentFile(null);
        } else {
            appState.setCurrentFile(file);
        }
    }

    async function handleRename(e: Event) {
        e.stopPropagation();
        const newName = prompt("Rename to:", file.name);
        if (!newName || newName === file.name) return;

        if (newName.includes("/")) {
            alert("File name cannot contain slashes");
            return;
        }

        const newPath = resolvePath(
            file.path.split("/").slice(0, -1).join("/") || "/",
            newName,
        );

        try {
            await vfs.move(file.id, newPath);
            onRefresh?.();

            // If currently selected, update selection
            if (appState.currentFile?.id === file.id) {
                const updated = await vfs.stat(file.id);
                if (updated) appState.setCurrentFile(updated);
            }
        } catch (err: any) {
            alert(err.message);
        }
    }

    async function handleDelete(e: Event) {
        e.stopPropagation();
        if (!confirm(`Delete ${file.name}?`)) return;
        try {
            await vfs.delete(file.id);
            onRefresh?.();

            // If deleted file/folder is current file
            if (appState.currentFile?.id === file.id) {
                appState.setCurrentFile(null);
            }
            // If deleted folder is current path (or parent of current path)
            if (
                file.isDir &&
                isAncestorOrSame(file.path, appState.currentPath)
            ) {
                appState.setCurrentPath(getParentPath(file.path));
            }
        } catch (err: any) {
            alert(err.message);
        }
    }

    // Expose refresh method to parent or context?
    // For now, simple prop callback to tell parent to refresh me is not enough if I am the one changed.
    // Actually, if a child is deleted, *I* (this component) need to refresh my children list.
    // If *I* am deleted, my parent needs to refresh.
    // The `onRefresh` prop is "Parent, please refresh your list because I changed or deleted myself".

    // But how to refresh MY children if a new file is created inside me?
    // We can expose a binding or method.
    // Using $effect might be complex.
    // Let's rely on re-expanding or manual refresh trigger.
    // For now, simple interaction:

    export async function refresh() {
        if (file.isDir) {
            await loadChildren();
        }
    }

    $effect(() => {
        const signal = appState.refreshSignal;
        if (!signal) return;

        if (!file.isDir) return;

        // Mode 1: Path based (Create, Move, Update)
        if (signal.type === "PATH") {
            // Check for exact match (this folder should refresh)
            const myPath = normalizePath(file.path);
            const signalPath = normalizePath(signal.value);
            if (signalPath === myPath) {
                console.log("FileTreeItem refreshing:", myPath);
                refresh(); // Fire and forget
            }
        }
        // Mode 2: ID based (Delete)
        else if (signal.type === "ID") {
            // Check if I am the deleted one
            if (signal.value === file.id) {
                if (
                    file.isDir &&
                    isAncestorOrSame(file.path, appState.currentPath)
                ) {
                    appState.setCurrentPath(getParentPath(file.path));
                }
            }

            // Check if one of my children is the deleted ID
            // We use `children` state which contains loaded children
            if (children.some((child) => child.id === signal.value)) {
                refresh();
            }
        }
    });
</script>

<li>
    <div
        class={`w-full flex items-center gap-1 py-1 text-sm rounded text-left cursor-pointer group hover:bg-gray-800 relative
        ${appState.currentFile?.id === file.id ? "bg-blue-900 text-white" : "text-gray-300"}
        ${appState.currentPath === file.path && file.isDir ? "bg-gray-800 ring-1 ring-gray-700" : ""}
        `}
        style="padding-left: {paddingLeft}; padding-right: 8px;"
        role="button"
        tabindex="0"
        onclick={handleClick}
        onkeydown={(e) => e.key === "Enter" && handleClick(e)}
    >
        <!-- Icon / Expand Toggle -->
        <span class="flex-shrink-0 w-4 flex justify-center text-gray-500">
            {#if file.isDir}
                {#if loading}
                    <LoaderCircle size={12} class="animate-spin" />
                {:else if expanded}
                    <ChevronDown size={14} />
                {:else}
                    <ChevronRight size={14} />
                {/if}
            {/if}
        </span>

        <!-- Type Icon -->
        {#if file.isDir}
            <Folder size={16} class="text-yellow-500 flex-shrink-0 mr-1" />
        {:else}
            <FileIcon size={16} class="text-blue-400 flex-shrink-0 mr-1" />
        {/if}

        <!-- Name -->
        <span class="truncate flex-1 select-none">{file.name}</span>

        <!-- Actions (Group Hover) -->
        <div
            class="hidden group-hover:flex items-center gap-1 bg-gray-900/90 rounded px-1 absolute right-1"
        >
            <button
                class="p-1 hover:text-blue-400"
                onclick={handleRename}
                title="Rename"
            >
                <Pencil size={12} />
            </button>
            <button
                class="p-1 hover:text-red-400"
                onclick={handleDelete}
                title="Delete"
            >
                <Trash2 size={12} />
            </button>
        </div>
    </div>

    {#if expanded && file.isDir}
        {#if children.length === 0 && loaded && !loading}
            <div
                style="padding-left: {(depth + 1) * 12 + 24}px"
                class="text-xs text-gray-600 py-1 select-none italic"
            >
                Empty
            </div>
        {/if}
        <ul class="border-l border-gray-800 ml-4">
            <!-- Visual guide line optional -->
            {#each children as child (child.id)}
                <FileTreeItem
                    file={child}
                    depth={depth + 1}
                    onRefresh={loadChildren}
                />
            {/each}
        </ul>
    {/if}
</li>
