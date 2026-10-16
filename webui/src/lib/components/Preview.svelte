<script lang="ts">
    import { appState } from "../state.svelte";
    import vfs from "../vfs";
    import { inferType } from "../utils";

    let url = $state("");
    let type = $state("");

    let zoom = $state(1);
    let viewport = $state<HTMLDivElement>();
    let width = $state(0);
    let height = $state(0);
    let naturalWidth = $state(0);
    let naturalHeight = $state(0);
    let dragging = $state(false);
    let pointerX = 0;
    let pointerY = 0;
    const fit = $derived(naturalWidth && naturalHeight
        ? Math.min(1, width / naturalWidth, height / naturalHeight) : 1);

    function panWithKeyboard(e: KeyboardEvent) {
        if (!viewport) return;
        const offsets: Record<string, [number, number]> = {
            ArrowLeft: [-40, 0], ArrowRight: [40, 0],
            ArrowUp: [0, -40], ArrowDown: [0, 40],
        };
        const offset = offsets[e.key];
        if (!offset) return;
        e.preventDefault();
        viewport.scrollLeft += offset[0];
        viewport.scrollTop += offset[1];
    }

    function setZoom(value: number) {
        zoom = Math.max(0.25, Math.min(8, value));
    }

    $effect(() => {
        const file = appState.currentFile;
        if (!file) return;
        type = inferType(file.name);
        zoom = 1;
        naturalWidth = naturalHeight = 0;
        url = "";
        let active = true;
        let objectURL = "";
        vfs.read(file.id).then(content => {
            if (!active || content === null) return;
            objectURL = URL.createObjectURL(content instanceof Blob ? content : new Blob([content]));
            url = objectURL;
        }).catch(err => {
            if (active) appState.addToast(`Failed to load preview: ${err.message}`, "error");
        });
        return () => {
            active = false;
            if (objectURL) URL.revokeObjectURL(objectURL);
        };
    });

</script>

<div
    class="h-full flex flex-col bg-gray-900 text-white"
>
    {#if !appState.currentFile}
        <div class="text-gray-500">No file selected</div>
    {:else if type.startsWith("image/")}
        <div class="flex items-center justify-center gap-3 p-2 shrink-0 bg-gray-800 text-sm">
            <button onkeydown={panWithKeyboard} aria-label="Zoom out" disabled={zoom <= 0.25} onclick={() => setZoom(zoom / 1.25)}>−</button>
            <span aria-live="polite">{Math.round(zoom * 100)}%</span>
            <button onkeydown={panWithKeyboard} aria-label="Zoom in" disabled={zoom >= 8} onclick={() => setZoom(zoom * 1.25)}>+</button>
            <button onkeydown={panWithKeyboard} onclick={() => { setZoom(1); if (viewport) { viewport.scrollLeft = 0; viewport.scrollTop = 0; } }}>Fit</button>
        </div>
        <!-- svelte-ignore a11y_no_static_element_interactions -->
        <div bind:this={viewport} bind:clientWidth={width} bind:clientHeight={height}
            role="region" aria-label="Image preview"
            class="flex-1 min-h-0 overflow-auto" style:cursor={dragging ? "grabbing" : "grab"}
            style:touch-action="none"
            onpointerdown={(e) => {
                if (e.button !== 0) return;
                e.preventDefault();
                e.currentTarget.setPointerCapture(e.pointerId);
                pointerX = e.clientX; pointerY = e.clientY; dragging = true;
            }}
            onpointermove={(e) => {
                if (!dragging || !viewport) return;
                viewport.scrollLeft -= e.clientX - pointerX;
                viewport.scrollTop -= e.clientY - pointerY;
                pointerX = e.clientX; pointerY = e.clientY;
            }}
            onpointerup={() => dragging = false}
            onpointercancel={() => dragging = false}
            onlostpointercapture={() => dragging = false}
        >
            <div class="grid place-items-center min-w-full min-h-full w-max">
                <img src={url} alt={appState.currentFile.name} draggable="false"
                    onload={(e) => {
                        const image = e.currentTarget;
                        if (!(image instanceof HTMLImageElement)) return;
                        naturalWidth = image.naturalWidth;
                        naturalHeight = image.naturalHeight;
                    }}
                    style:width={naturalWidth ? `${naturalWidth * fit * zoom}px` : "auto"}
                    style:height={naturalHeight ? `${naturalHeight * fit * zoom}px` : "auto"}
                    class="max-w-none shrink-0 select-none" />
            </div>
        </div>
    {:else if type.startsWith("video/")}
        <!-- svelte-ignore a11y_media_has_caption -->
        <video src={url} controls class="max-w-full min-h-0 flex-1"></video>
    {:else if type.startsWith("audio/")}
        <audio src={url} controls class="w-full"></audio>
    {:else if type === "application/pdf"}
        <iframe src={url} title="PDF Preview" class="w-full min-h-0 flex-1 border-0"
        ></iframe>
    {:else}
        <div class="text-center">
            <p class="mb-2 text-xl">Preview not available</p>
            <p class="text-gray-500">{appState.currentFile.name} ({type})</p>
        </div>
    {/if}


</div>
