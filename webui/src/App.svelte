<script lang="ts">
    import { onMount } from "svelte";
    import Sidebar from "./lib/components/Sidebar.svelte";
    import Editor from "./lib/components/Editor.svelte";
    import Preview from "./lib/components/Preview.svelte";
    import Terminal from "./lib/components/Terminal.svelte";
    import Toast from "./lib/components/Toast.svelte";
    import ConnectionStatus from "./lib/components/ConnectionStatus.svelte";
    import Login from "./lib/components/Login.svelte";
    import AdminUsers from "./lib/components/AdminUsers.svelte";
    import AdminStatus from "./lib/components/AdminStatus.svelte";
    import ChangePassword from "./lib/components/ChangePassword.svelte";
    import { appState } from "./lib/state.svelte";
    import vfs from "./lib/vfs";
    import sseClient, { type SSEMessage } from "./lib/sse";
    import { inferType, resolvePath } from "./lib/utils";

    // Determine what to show in main area
    let showPreview = $derived.by(() => {
        if (!appState.currentFile) return false;
        const type = inferType(appState.currentFile.name);
        return (
            !type.startsWith("text/") &&
            type !== "application/json" &&
            type !== "application/javascript" &&
            type !== "application/typescript" &&
            type !== "application/octet-stream"
        );
    });

    // Drag and Drop Logic
    let isDragging = $state(false);

    function handleDragOver(e: DragEvent) {
        e.preventDefault();
        isDragging = true;
    }

    function handleDragLeave(e: DragEvent) {
        e.preventDefault();
        isDragging = false;
    }

    async function handleDrop(e: DragEvent) {
        e.preventDefault();
        isDragging = false;

        if (!e.dataTransfer?.files?.length) return;

        const files = Array.from(e.dataTransfer.files);
        const currentPath = appState.currentPath;

        for (const file of files) {
            const targetPath = resolvePath(currentPath, file.name);
            try {
                await vfs.create(targetPath, file);
            } catch (err: any) {
                console.error(`Failed to upload ${file.name}:`, err);
                alert(`업로드 실패 (${file.name}): ${err.message}`);
            }
        }
    }

    onMount(() => {
        sseClient.on("subscribe", (message: SSEMessage) => {
            const { id, data } = message;
            // 1. Initial connection success message
            if (data.status && id) {
                console.log("Registered Client ID:", id);
                appState.setClientId(id);
                appState.addToast("Connected to server", "success");
            }
        });

        // Handle initial connection info
        // Handle "publish" events which now cover both subscription and VFS updates
        sseClient.on("publish", (message: SSEMessage) => {
            const data = message.data;
            if (!data.status) {
                appState.addToast(
                    data.message ?? "File operation failed",
                    "error",
                );
                return;
            }
            // VFS changes
            if (data?.meta?.action?.startsWith("vfs.")) {
                // Refresh logic
                console.log(
                    "VFS change detected, handling update...",
                    data?.meta?.action,
                );

                // Centralized handling
                appState.handleVFSUpdate(data.meta);

                // Toast logic
                if (data?.meta?.path) {
                    appState.addToast(
                        `File changed: ${data.meta.path} (${data.meta.action})`,
                        "info",
                    );
                }
            }
        });

        // Start SSE connection if logged in
        appState.checkAuth().then((loggedIn) => {
            if (loggedIn) {
                sseClient.connect();
            }
        });

        return () => {
            sseClient.disconnect();
        };
    });
</script>

{#if !appState.authInitialized}
    <div
        class="h-screen w-screen flex items-center justify-center bg-gray-900 text-white z-[200]"
    >
        Loading...
    </div>
{:else if !appState.isLoggedIn}
    <Login />
{:else}
<div class="bg-gray-900 text-gray-300 h-screen w-screen flex overflow-hidden">
    <!-- SideBar -->
    <Sidebar />

    <div class="flex-1 flex flex-col min-w-0">
        {#if appState.adminPage === "users"}
            <AdminUsers />
        {:else if appState.adminPage === "server"}
            <AdminStatus />
        {:else if appState.adminPage === "password"}
            <ChangePassword />
        {:else}
        <!-- Main Editor Area -->
        <!-- svelte-ignore a11y_no_static_element_interactions -->
        <div
            class="flex-1 overflow-hidden relative border-b border-gray-700 relative"
            ondragover={handleDragOver}
            ondragleave={handleDragLeave}
            ondrop={handleDrop}
        >
            {#if isDragging}
                <div
                    class="absolute inset-0 bg-blue-500/30 z-50 flex items-center justify-center pointer-events-none backdrop-blur-sm border-2 border-blue-500 border-dashed m-2 rounded-lg"
                >
                    <div
                        class="bg-gray-900/80 px-6 py-4 rounded-xl shadow-xl text-white font-bold text-lg animate-pulse"
                    >
                        Drop files to upload
                    </div>
                </div>
            {/if}

            {#if appState.currentFile}
                {#if showPreview}
                    <Preview />
                {:else}
                    <Editor />
                {/if}
            {:else}
                <div
                    class="h-full flex items-center justify-center text-gray-600"
                >
                    <div class="text-center">
                        <h1 class="text-2xl font-bold mb-2">govfs</h1>
                        <p>Select a file to view</p>
                        <p class="text-sm mt-2 text-gray-500">
                            or drop files here to upload
                        </p>
                    </div>
                </div>
            {/if}
        </div>

        <!-- Terminal -->
        <div class="h-48 flex-shrink-0">
            <Terminal />
        </div>
        {/if}

        <!-- Status Bar Footer -->
        <div
            class="h-6 bg-gray-800 border-t border-gray-700 flex items-center px-2 justify-end"
        >
            <ConnectionStatus />
        </div>
    </div>

    <Toast />
</div>
{/if}
