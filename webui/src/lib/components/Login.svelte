<script lang="ts">
    import { appState } from "../state.svelte";
    import sseClient from "../sse";

    let username = $state("");
    let password = $state("");
    let errorMsg = $state("");

    async function handleLogin() {
        errorMsg = "";
        try {
            const res = await fetch("/auth/login", {
                method: "POST",
                headers: { "Content-Type": "application/json" },
                body: JSON.stringify({ username, password }),
            });

            if (!res.ok) {
                const data = await res.text();
                errorMsg = data || "Login failed Check your credentials.";
                return;
            }

            sseClient.disconnect();
            sseClient.connect();
            appState.setIsLoggedIn(true);
            appState.addToast("Logged in successfully", "success");
            appState.refresh();
        } catch (e: any) {
            errorMsg = e.message;
            appState.addToast(e.message, "error");
        }
    }
</script>

<div
    class="h-full w-full flex items-center justify-center bg-gray-900 absolute inset-0 z-[100]"
>
    <div
        class="bg-gray-800 p-8 rounded-xl shadow-2xl w-96 border border-gray-700"
    >
        <h2 class="text-2xl font-bold text-white mb-6 text-center">govfs</h2>
        {#if errorMsg}
            <div
                class="bg-red-500/20 border border-red-500 text-red-300 px-4 py-2 rounded mb-4 text-sm text-center"
            >
                {errorMsg}
            </div>
        {/if}
        <form
            onsubmit={(e) => {
                e.preventDefault();
                handleLogin();
            }}
            class="space-y-4"
        >
            <div>
                <label for="username" class="block text-gray-400 text-sm mb-1"
                    >Username</label
                >
                <input
                    id="username"
                    type="text"
                    bind:value={username}
                    class="w-full bg-gray-900 border border-gray-700 rounded px-3 py-2 text-white focus:outline-none focus:border-blue-500"
                    required
                />
            </div>
            <div>
                <label for="password" class="block text-gray-400 text-sm mb-1"
                    >Password</label
                >
                <input
                    id="password"
                    type="password"
                    bind:value={password}
                    class="w-full bg-gray-900 border border-gray-700 rounded px-3 py-2 text-white focus:outline-none focus:border-blue-500"
                    required
                />
            </div>
            <button
                type="submit"
                class="w-full bg-blue-600 hover:bg-blue-500 text-white font-bold py-2 px-4 rounded transition-colors mt-6"
            >
                Login
            </button>
        </form>
    </div>
</div>
