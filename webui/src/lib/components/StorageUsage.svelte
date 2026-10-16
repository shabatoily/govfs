<script lang="ts">
    import { appState } from "../state.svelte";
    import vfs from "../vfs";
    import { formatBytes, treeSize } from "../utils";

    let size = $state<number | null>(null);
    let failed = $state(false);
    $effect(() => {
        appState.refreshSignal;
        let active = true;
        failed = false;
        // shortcut: 전체 트리로 합산하며, 대규모 드라이브에서는 서버 집계 API로 전환합니다.
        vfs.tree("/").then(tree => {
            if (active) size = treeSize(tree);
        }).catch(() => {
            if (active) { size = null; failed = true; }
        });
        return () => { active = false; };
    });
</script>

<span class="text-xs text-gray-400" title="Total file size in your drive">
    Storage used: {size !== null ? formatBytes(size) : failed ? "Unavailable" : "Loading…"}
</span>
