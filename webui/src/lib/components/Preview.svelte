<script lang="ts">
    import { appState } from "../state.svelte";
    import vfs from "../vfs";
    import { inferType } from "../utils";
    import { Save } from "lucide-svelte";

    let url = $state("");
    let type = $state("");

    async function loadPreview() {
        if (!appState.currentFile) return;

        const fileId = appState.currentFile.id;
        type = inferType(appState.currentFile.name);

        if (url) URL.revokeObjectURL(url);

        const content = await vfs.read(fileId);
        if (content instanceof Blob) {
            url = URL.createObjectURL(content);
        } else if (typeof content === "string") {
            // Should not happen for preview types usually, but just in case
            const blob = new Blob([content], { type: "text/plain" });
            url = URL.createObjectURL(blob);
        }
    }

    import { untrack } from "svelte";

    $effect(() => {
        // Explicitly track the file ID
        const fileId = appState.currentFile?.id;

        // Untrack the execution of loadPreview so it doesn't track 'url' or 'type' reads
        untrack(() => {
            if (fileId) {
                loadPreview();
            }
        });
    });

    // We'll trust inline logic for simplicity in this replacement or use a local let.
    let commentText = $state("");

    $effect(() => {
        commentText = appState.currentFile?.comments || "";
    });

    async function handleSave() {
        if (!appState.currentFile) return;
        try {
            await vfs.writeComments(appState.currentFile.id, commentText);
        } catch (err: any) {
            alert(err.message);
        }
    }
</script>

<div
    class="h-full flex items-center justify-center p-4 bg-gray-900 text-white overflow-auto pb-32"
>
    <!-- Added padding bottom to prevent content being hidden behind comment box -->
    {#if !appState.currentFile}
        <div class="text-gray-500">No file selected</div>
    {:else if type.startsWith("image/")}
        <img
            src={url}
            alt={appState.currentFile.name}
            class="max-w-full max-h-full object-contain"
        />
    {:else if type.startsWith("video/")}
        <!-- svelte-ignore a11y_media_has_caption -->
        <video src={url} controls class="max-w-full max-h-full"></video>
    {:else if type.startsWith("audio/")}
        <audio src={url} controls class="w-full"></audio>
    {:else if type === "application/pdf"}
        <iframe src={url} title="PDF Preview" class="w-full h-full border-0"
        ></iframe>
    {:else}
        <div class="text-center">
            <p class="mb-2 text-xl">Preview not available</p>
            <p class="text-gray-500">{appState.currentFile.name} ({type})</p>
        </div>
    {/if}

    <!-- Comment Section -->
    {#if appState.currentFile}
        <div
            class="absolute bottom-0 left-0 right-0 p-3 bg-gray-800 border-t border-gray-700 flex gap-3 items-start shadow-lg"
        >
            <span class="text-xs text-gray-400 font-bold mt-2">COMMENT</span>
            <div class="flex-1 flex gap-2 relative">
                <textarea
                    class="w-full bg-gray-900 border border-gray-700 rounded px-3 py-2 text-sm text-gray-200 placeholder-gray-500 focus:outline-none focus:border-blue-500 resize-none"
                    rows="3"
                    placeholder="Add a comment... (Ctrl+Enter to save)"
                    bind:value={commentText}
                    onkeydown={(e: KeyboardEvent) => {
                        if (e.key === "Enter" && (e.ctrlKey || e.metaKey)) {
                            handleSave();
                        }
                    }}
                ></textarea>
                <button
                    class="absolute bottom-2 right-2 p-1.5 bg-blue-600 hover:bg-blue-500 text-white rounded transition-colors"
                    title="Save Comment (Ctrl+Enter)"
                    onclick={handleSave}
                >
                    <Save size={16} />
                </button>
            </div>
        </div>
    {/if}
</div>
