<script lang="ts">
    import { appState } from "../state.svelte";
    import vfs from "../vfs";
    import { Save } from "lucide-svelte";

    // 파일 선택에 따라 코멘트를 동기화합니다.
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

<!-- Comment Section -->
{#if appState.currentFile}
    <div
        class="shrink-0 px-4 py-2 bg-gray-800 flex gap-3 items-start"
    >
        <span class="text-xs text-gray-400 font-bold mt-2">COMMENT</span>
        <div class="flex-1 flex gap-2 relative">
            <textarea
                class="w-full bg-gray-900 border border-gray-700 rounded px-3 py-2 text-sm text-gray-200 placeholder-gray-500 focus:outline-none focus:border-blue-500 resize-none"
                rows="2"
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
