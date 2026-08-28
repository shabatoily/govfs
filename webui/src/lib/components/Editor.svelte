<script lang="ts">
    import { onMount, onDestroy } from "svelte";
    import OverType, { type OverTypeInstance } from "overtype";
    import { appState } from "../state.svelte";
    import vfs from "../vfs";

    let editorEl: HTMLDivElement;
    let instance: OverTypeInstance | null = null;

    let activeFileId = $state<string | null>(null);
    let isProgrammaticUpdate = false;
    let isDirty = $state(false);

    async function loadContent() {
        // 1. Reset state immediately
        activeFileId = null;
        isDirty = false;

        if (!appState.currentFile) {
            if (instance) {
                isProgrammaticUpdate = true;
                instance.setValue("");
                isProgrammaticUpdate = false;
            }
            return;
        }

        const targetId = appState.currentFile.id;

        try {
            const content = await vfs.read(targetId);

            // 2. Verify: Ensure we are still on the *same* file
            if (appState.currentFile.id !== targetId) {
                return;
            }

            if (typeof content === "string" && instance) {
                // 3. Set content safely
                isProgrammaticUpdate = true;
                instance.setValue(content);
                isProgrammaticUpdate = false;

                // 4. Enable saving
                activeFileId = targetId;
                isDirty = false;
            }
        } catch (e) {
            console.error("Failed to load file content:", e);
        }
    }

    async function saveContent() {
        if (!activeFileId || !instance) return;

        // Double security check
        if (appState.currentFile?.id !== activeFileId) {
            console.warn("Save aborted: File mismatch.");
            return;
        }

        const content = instance.getValue();
        try {
            await vfs.write(activeFileId, content);
            console.log("Saved", activeFileId);
            isDirty = false;
        } catch (e) {
            console.error("Save failed", e);
            alert("저장 실패: " + e);
        }
    }

    function handleKeydown(e: KeyboardEvent) {
        if ((e.ctrlKey || e.metaKey) && e.key === "s") {
            e.preventDefault();
            saveContent();
        }
    }

    // Watch for file selection changes
    $effect(() => {
        // Trigger load whenever the ID changes
        if (appState.currentFile?.id) {
            loadContent();
        } else {
            // Reset if no file selected
            activeFileId = null;
            isDirty = false;
            if (instance) {
                isProgrammaticUpdate = true;
                instance.setValue("");
                isProgrammaticUpdate = false;
            }
        }
    });

    onMount(() => {
        window.addEventListener("keydown", handleKeydown);
        appState.saveHandler = saveContent;

        if (!editorEl) return;

        const [ed] = OverType.init(editorEl, {
            toolbar: true,
            theme: "cave",
            value: "",
            onChange: (val: string) => {
                if (isProgrammaticUpdate) return;
                if (!activeFileId) return;

                // Just mark as dirty, don't auto-save
                if (!isDirty) isDirty = true;
            },
        });
        instance = ed;
    });

    onDestroy(() => {
        window.removeEventListener("keydown", handleKeydown);
        appState.saveHandler = null;
        instance?.destroy();
        instance = null;
    });
    const headerText = $derived(
        appState.currentFile?.name ?? "No file selected",
    );
    const canSave = $derived(isDirty && !!activeFileId);
    const saveBtnClass = $derived(
        `px-3 py-1 text-xs font-medium rounded-md transition-colors ${
            isDirty
                ? "bg-blue-600 text-white hover:bg-blue-700 shadow-sm"
                : "bg-neutral-200 text-neutral-400 dark:bg-neutral-700 cursor-not-allowed"
        }`,
    );
</script>

<div class="h-full flex flex-col font-mono">
    <!-- Header / Toolbar -->
    <div
        class="flex items-center justify-between px-4 py-2 bg-neutral-100 border-b border-neutral-200 dark:bg-neutral-800 dark:border-neutral-700 shrink-0"
    >
        <div class="text-sm truncate text-neutral-600 dark:text-neutral-400">
            {headerText}
            {#if isDirty}<span class="text-amber-500 font-bold ml-1">*</span
                >{/if}
        </div>
        <button onclick={saveContent} disabled={!canSave} class={saveBtnClass}>
            저장 (Cmd+S)
        </button>
    </div>

    <!-- Editor Container -->
    <div
        bind:this={editorEl}
        class="flex-1 overflow-hidden relative"
        id="editor-container"
    ></div>
</div>

<style>
    /* CSS hacks to make OverType fit Svelte container if needed */
    :global(.overtype-editor) {
        height: 100%;
        display: flex;
        flex-direction: column;
    }
</style>
