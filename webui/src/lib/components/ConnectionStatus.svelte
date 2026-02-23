<script lang="ts">
    import sseClient, { type SSEMessage } from "../sse";
    import { onMount } from "svelte";

    let isConnected = $state(false);

    onMount(() => {
        // Handle connection events
        const updateStatus = (msg: SSEMessage) => {
            isConnected = msg.data.status;
        };

        sseClient.on("open", updateStatus);
        sseClient.on("error", updateStatus);

        // Also if we receive a heartbeat, we are definitely connected
        sseClient.on("heartbeat", () => (isConnected = true));

        return () => {
            sseClient.off("open", updateStatus);
            sseClient.off("error", updateStatus);
        };
    });
    const indicatorClass = $derived(
        `w-2 h-2 rounded-full transition-colors duration-300 ${
            isConnected
                ? "bg-green-500 shadow-[0_0_5px_rgba(34,197,94,0.5)]"
                : "bg-red-500 shadow-[0_0_5px_rgba(239,68,68,0.5)]"
        }`,
    );
    const statusText = $derived(isConnected ? "Online" : "Offline");
</script>

<div class="flex items-center gap-2 text-xs">
    <div class={indicatorClass}></div>
    <span class="text-gray-400">{statusText}</span>
</div>
