<script lang="ts">
    import { tick } from "svelte";
    import { appState } from "../state.svelte";
    import vfs from "../vfs";
    import { resolvePath, formatTreeToString } from "../utils"; // Need config for formatTreeToString?

    // Log history
    let history = $state<string[]>([]);
    let inputValue = $state("");
    let outputEl: HTMLDivElement;

    // Command Logic Reuse
    // We can just implement the command handling here for simplicity as it interacts with local UI state (history)

    function log(msg: string) {
        history = [...history, msg];
        tick().then(() => {
            if (outputEl) outputEl.scrollTop = outputEl.scrollHeight;
        });
    }

    async function execute(cmdLine: string) {
        if (!cmdLine.trim()) return;
        log(`> ${cmdLine}`);

        const [cmd, ...args] = cmdLine.trim().split(/\s+/);

        try {
            switch (cmd) {
                case "help":
                    log("  ls <path> show file list");
                    log("  tree <path> show file tree");
                    log("  mkdir <name> create new dir");
                    log("  new <name> create new file on current path");
                    log("  open <name> open file from current path");
                    log("  save save current file");
                    log("  rm <name> remove file");
                    log("  cp <src> <dst> copy file");
                    log("  mv <src> <dst> move file");
                    log("  clear clear terminal");
                    log("  pwd print current path");
                    break;
                case "pwd":
                    log(appState.currentPath);
                    break;
                case "clear":
                    history = [];
                    break;
                case "ls":
                    const listPath = args[0]
                        ? resolvePath(appState.currentPath, args[0])
                        : appState.currentPath;
                    const files = await vfs.list(listPath);
                    files.forEach((f) => {
                        log(
                            `${f.isDir ? "D" : "-"} ${f.size} ${f.modified} ${f.name}`,
                        );
                    });
                    break;
                case "cd":
                    if (!args[0]) break;
                    const newPath = resolvePath(appState.currentPath, args[0]);
                    // Update file list in global state
                    const newFiles = await vfs.list(newPath);
                    // Verify it exists/is dir? VFS doesn't enforce stat check on cd usually in this simple shell,
                    // but we should probably update state.
                    appState.setCurrentPath(newPath);
                    appState.setFileList(newFiles);
                    break;
                case "mkdir":
                    if (!args[0]) {
                        log("usage: mkdir <name>");
                        break;
                    }
                    const mkPath = resolvePath(appState.currentPath, args[0]);
                    await vfs.mkdir(mkPath);
                    log(`created directory: ${args[0]}`);
                    // Refresh
                    appState.setFileList(await vfs.list(appState.currentPath));
                    break;
                case "rm":
                    if (!args[0]) {
                        log("usage: rm <id|name>");
                        break;
                    }
                    // This is tricky, rm usually takes name, but vfs.delete takes ID.
                    // We need to resolve name to ID.
                    const targetName = args[0];
                    const filesInDir = await vfs.list(appState.currentPath);
                    const target = filesInDir.find(
                        (f) => f.name === targetName,
                    );
                    if (!target) {
                        log(`file not found: ${targetName}`);
                    } else {
                        await vfs.delete(target.id);
                        log(`deleted: ${targetName}`);
                        appState.setFileList(
                            await vfs.list(appState.currentPath),
                        );
                    }
                    break;
                case "tree":
                    const treePath = args[0]
                        ? resolvePath(appState.currentPath, args[0])
                        : appState.currentPath;
                    const treeNode = await vfs.tree(treePath);
                    // Use formatTreeToString from utils
                    log(formatTreeToString(treeNode));
                    break;

                case "new":
                    if (!args[0]) {
                        log("usage: new <name>");
                        break;
                    }
                    const newFilePath = resolvePath(
                        appState.currentPath,
                        args[0],
                    );
                    const newFile = await vfs.create(
                        newFilePath,
                        "# New Document",
                    );
                    log(`created file: ${args[0]}`);
                    appState.setFileList(await vfs.list(appState.currentPath));
                    appState.setCurrentFile(newFile);
                    break;

                case "open":
                    if (!args[0]) {
                        log("usage: open <name>");
                        break;
                    }
                    const openTarget = (
                        await vfs.list(appState.currentPath)
                    ).find((f) => f.name === args[0]);
                    if (!openTarget) {
                        log(`file not found: ${args[0]}`);
                    } else {
                        appState.setCurrentFile(openTarget);
                        log(`opened: ${args[0]}`);
                    }
                    break;

                case "save":
                    if (appState.saveHandler) {
                        try {
                            await appState.saveHandler();
                            log(`saved: ${appState.currentFile?.name}`);
                        } catch (e: any) {
                            log(`error saving: ${e.message}`);
                        }
                    } else {
                        log("no active editor to save");
                    }
                    break;

                case "cp":
                    if (!args[0] || !args[1]) {
                        log("usage: cp <src> <dst>");
                        break;
                    }
                    const cpSrcName = args[0];
                    const cpDst = resolvePath(appState.currentPath, args[1]);
                    const cpSrcFile = (
                        await vfs.list(appState.currentPath)
                    ).find((f) => f.name === cpSrcName);

                    if (!cpSrcFile) {
                        log(`file not found: ${cpSrcName}`);
                    } else {
                        await vfs.copy(cpSrcFile.id, cpDst);
                        log(`copied ${cpSrcName} to ${args[1]}`);
                        appState.setFileList(
                            await vfs.list(appState.currentPath),
                        );
                    }
                    break;

                case "mv":
                    if (!args[0] || !args[1]) {
                        log("usage: mv <src> <dst>");
                        break;
                    }
                    const mvSrcName = args[0];
                    const mvDst = resolvePath(appState.currentPath, args[1]);
                    const mvSrcFile = (
                        await vfs.list(appState.currentPath)
                    ).find((f) => f.name === mvSrcName);

                    if (!mvSrcFile) {
                        log(`file not found: ${mvSrcName}`);
                    } else {
                        await vfs.move(mvSrcFile.id, mvDst);
                        log(`moved ${mvSrcName} to ${args[1]}`);
                        appState.setFileList(
                            await vfs.list(appState.currentPath),
                        );
                    }
                    break;

                default:
                    log(`command not found: ${cmd}`);
                    break;
            }
        } catch (e: any) {
            log(`Error: ${e.message}`);
        }

        inputValue = "";
    }

    function handleKeydown(e: KeyboardEvent) {
        if (e.key === "Enter") {
            execute(inputValue);
        }
    }
</script>

<div
    class="flex flex-col h-full bg-black text-xs font-mono p-2 border-t border-gray-700"
>
    <div
        class="flex-1 overflow-y-auto whitespace-pre-wrap text-gray-300"
        bind:this={outputEl}
    >
        {#each history as line}
            <div>{line}</div>
        {/each}
    </div>
    <div class="flex items-center gap-2 mt-2 border-t border-gray-800 pt-1">
        <span class="text-blue-400">$</span>
        <input
            class="flex-1 bg-transparent border-none outline-none text-white placeholder-gray-600"
            bind:value={inputValue}
            onkeydown={handleKeydown}
            placeholder="Type help for commands..."
            type="text"
        />
    </div>
</div>
