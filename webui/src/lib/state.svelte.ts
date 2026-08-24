import vfs, { type FileInfo } from './vfs';
import { getParentPath } from './utils';
import sseClient from './sse';

export type ToastType = 'success' | 'error' | 'info' | 'warning';

export interface ToastItem {
    id: number;
    message: string;
    type: ToastType;
}

export interface CurrentUser {
    id: string;
    username: string;
    role: 'admin' | 'user';
}

export class AppState {
    currentPath = $state('/');
    currentFile = $state<FileInfo | null>(null);
    fileList = $state<FileInfo[]>([]);
    clientId = $state<string | null>(null);
    toasts = $state<ToastItem[]>([]); // We hold toast data here
    refreshSignal = $state<{ type: 'PATH' | 'ID'; value: string; timestamp: number } | null>(null);
    isLoggedIn = $state<boolean>(false);
    authInitialized = $state<boolean>(false);
    currentUser = $state<CurrentUser | null>(null);
    adminPage = $state<"server" | "users" | "password" | null>(null);

    setClientId(id: string) {
        this.clientId = id;
    }

    async checkAuth() {
        try {
            const res = await fetch("/auth/me", {
                method: "GET",
                headers: {
                    "Content-Type": "application/json",
                },
            });
            this.isLoggedIn = res.status === 200;
            this.currentUser = this.isLoggedIn ? await res.json() : null;
        } catch (e) {
            this.isLoggedIn = false;
            this.currentUser = null;
        } finally {
            this.authInitialized = true;
        }
        return this.isLoggedIn;
    }

    async logout() {
        await fetch("/auth/logout", {
            method: "POST",
            headers: {
                "Content-Type": "application/json",
            },
        });
        this.isLoggedIn = false;
        this.currentUser = null;
		this.adminPage = null;
        sseClient.disconnect();
    }

    triggerRefreshPath(path: string) {
        this.refreshSignal = { type: 'PATH', value: path, timestamp: Date.now() };
    }

    triggerRefreshId(id: string) {
        this.refreshSignal = { type: 'ID', value: id, timestamp: Date.now() };
    }

    addToast(message: string, type: ToastType = 'info') {
        const id = Date.now() + Math.random();
        this.toasts.push({ id, message, type });
        setTimeout(() => {
            this.removeToast(id);
        }, 3000);
    }

    removeToast(id: number) {
        const index = this.toasts.findIndex(t => t.id === id);
        if (index !== -1) {
            this.toasts.splice(index, 1);
        }
    }

    setCurrentPath(path: string) {
        this.currentPath = path;
    }

    setCurrentFile(file: FileInfo | null) {
        this.currentFile = file;
    }

    updateFileInList(updatedFile: FileInfo) {
        const index = this.fileList.findIndex(f => f.id === updatedFile.id);
        if (index !== -1) {
            this.fileList[index] = updatedFile;
        }
    }

    setFileList(list: FileInfo[]) {
        this.fileList = list;
    }

    async refresh() {
        if (!this.currentPath) return;

        // Signal the UI tree to refresh this path
        this.triggerRefreshPath(this.currentPath);

        try {
            const list = await vfs.list(this.currentPath);
            this.setFileList(list);

            // Also refresh current file stats if open
            if (this.currentFile) {
                const refreshedFile = list.find(f => f.id === this.currentFile!.id);
                // If file is deleted, it won't be in list, handle?? 
                // For now, if modified, update it.
                // Actually stat() might be better if we want precise details, but list has basics.
                // Let's stick to simple list refresh for now.
                // Note: If we really want to be robust, we should check if current file still exists.
                if (!refreshedFile) {
                    // Optionally close or mark as deleted?
                    // Let's just leave it for now to avoid jarring UX.
                } else {
                    // Check if modified changed?
                    if (refreshedFile.modified !== this.currentFile.modified) {
                        // Maybe update content? No, that might overwrite user edits.
                        // Ideally we warn. But for Explorer view, just updating meta is fine.
                    }
                }
            }
        } catch (e) {
            console.error("Failed to refresh file list:", e);
        }
    }

    saveHandler: (() => Promise<void>) | null = null;

    async handleVFSUpdate(meta: any) {
        if (!meta || !meta.action || !meta.action.startsWith("vfs.")) return;

        const { id, action, path } = meta;

        // 1. Handle Deletion
        if (action === "vfs.delete") {
            this.triggerRefreshId(id);
            // If we can determine parent from path (if available) or if we just want to be safe, 
            // we might want to refresh currentPath if the deleted item was inside it?
            // But ID-based refresh is what Sidebar uses.
            // Also, if the deleted file is the current file:
            if (this.currentFile?.id === id) {
                this.setCurrentFile(null);
            }
            // If deleted file is current path?
            if (this.currentPath === path) { // Path might strict equality?
                // Should move up?
                // Logic from FileTreeItem:
                // if (isAncestorOrSame(file.path, appState.currentPath)) ...
                // But we might not have file info if it's already deleted and we only have ID.
                // If 'path' is provided in meta, we can use it.
            }
        }

        // 2. Handle Creation / Update / Move
        let filePath = path;
        // If path missing, try to stat (unless it was deleted)
        if (!filePath && action !== "vfs.delete") {
            try {
                const stat = await vfs.stat(id);
                if (stat) filePath = stat.path;
            } catch (e) {
                console.error("Failed to stat file for SSE refresh:", e);
            }
        }

        if (filePath) {
            // Determine parent to refresh
            // We need to import getParentPath. 
            // Since we can't easily import from utils inside class file without circular deps if utils uses state?
            // checking imports... state imports vfs. utils imports state. Circular dependency risk!
            // Let's implement simple getParentPath here or resolve it.
            // utils.ts imports state.svelte.ts? No, I checked utils.ts imports? 
            // Let's check imports in state.svelte.ts -> imports ./vfs.
            // Sidebar imports utils.
            // I should check utils imports.

            // Assuming simple string manipulation is fine:
            const parentPath = getParentPath(filePath);
            // Actually standard getParentPath logic handles root '/' -> '/' case.

            this.triggerRefreshPath(parentPath);

            // Also if this is current file, update stats
            if (this.currentFile && this.currentFile.id === id) {
                const stat = await vfs.stat(id);
                if (stat) this.setCurrentFile(stat);
            }
        }
    }
}

export const appState = new AppState();
