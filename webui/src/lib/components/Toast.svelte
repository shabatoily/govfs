<script lang="ts">
    import {
        CircleCheck,
        CircleX,
        Info,
        X,
        TriangleAlert,
    } from "lucide-svelte";

    import { appState } from "../state.svelte";

    // We just display what's in appState
</script>

<div class="fixed bottom-4 right-4 z-50 flex flex-col gap-2">
    {#each appState.toasts as toast (toast.id)}
        <div
            class="flex items-center gap-3 px-4 py-3 rounded shadow-lg min-w-[300px] border animate-slide-up bg-gray-800 border-gray-700 text-white"
        >
            {#if toast.type === "success"}
                <CircleCheck size={20} class="text-green-400" />
            {:else if toast.type === "error"}
                <CircleX size={20} class="text-red-400" />
            {:else if toast.type === "warning"}
                <TriangleAlert size={20} class="text-yellow-400" />
            {:else}
                <Info size={20} class="text-blue-400" />
            {/if}

            <div class="flex-1 text-sm">{toast.message}</div>

            <button
                onclick={() => appState.removeToast(toast.id)}
                class="text-gray-400 hover:text-white"
            >
                <X size={16} />
            </button>
        </div>
    {/each}
</div>

<style>
    @keyframes slide-up {
        from {
            transform: translateY(100%);
            opacity: 0;
        }
        to {
            transform: translateY(0);
            opacity: 1;
        }
    }
    .animate-slide-up {
        animation: slide-up 0.3s ease-out;
    }
</style>
