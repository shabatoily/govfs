export interface FileInfo {
    id: string;
    path: string;
    name: string;
    extension: string;
    size: number;
    isDir: boolean;
    modified: string;
    comments: string;
}

export interface TreeNode {
    meta: FileInfo;
    children: TreeNode[];
}

export interface VFS {
    list: (path: string) => Promise<FileInfo[]>;
    tree: (path: string) => Promise<TreeNode>;
    stat: (id: string) => Promise<FileInfo | null>;
    read: (id: string) => Promise<string | Blob | null>;
    create: (path: string, fileOrText: File | string) => Promise<FileInfo>;
    mkdir: (path: string) => Promise<FileInfo>;
    write: (id: string, content: string) => Promise<boolean>;
    delete: (id: string) => Promise<boolean>;
    move: (id: string, destPath: string) => Promise<boolean>;
    writeComments: (id: string, comment: string) => Promise<boolean>;
    copy: (id: string, destPath: string) => Promise<boolean>;
}

/// ----------- VFS API (Server is generic, client decides type) -----------------
import { appState } from "./state.svelte";

function getHeaders(contentType?: string): HeadersInit {
    const headers: HeadersInit = {};
    if (contentType) {
        headers['Content-Type'] = contentType;
    }
    const clientId = appState.clientId;
    if (clientId) {
        headers['X-Client-ID'] = clientId;
    }
    const token = appState.token;
    if (token) {
        headers['Authorization'] = `Bearer ${token}`;
    }
    return headers;
}

async function vfsFetch(input: RequestInfo | URL, init?: RequestInit): Promise<Response> {
    const res = await fetch(input, init);
    if (res.status === 401) {
        appState.setRequireLogin(true);
        appState.logout();
    }
    return res;
}

/// ----------- VFS API (Server is generic, client decides type) -----------------
const vfs: VFS = {
    // 파일 목록
    async list(path: string = '/'): Promise<FileInfo[]> {
        const res = await vfsFetch(`/vfs?q=${path}`, {
            headers: getHeaders()
        });
        if (!res.ok) throw new Error(await res.text());
        return (await res.json()).payload as FileInfo[]; // [{id, name, size, modified}, ...]
    },

    async tree(path: string = '/'): Promise<TreeNode> {
        const res = await vfsFetch(`vfs?q=${path}&viewType=tree`, {
            headers: getHeaders()
        });
        if (!res.ok) throw new Error(await res.text());
        return (await res.json()).payload as TreeNode;
    },

    // 파일 메타 정보
    async stat(id: string): Promise<FileInfo | null> {
        const res = await vfsFetch(`/vfs/${encodeURIComponent(id)}/stat`, {
            headers: getHeaders()
        });
        if (res.status === 404) return null;
        if (!res.ok) throw new Error(await res.text());
        return await res.json() as FileInfo; // {id, name, size, modified}
    },

    // 파일 읽기 (텍스트/바이너리 구분은 클라이언트가 확장자 기반으로 처리)
    async read(id: string) {
        const res = await vfsFetch(`/vfs/${encodeURIComponent(id)}`, {
            headers: getHeaders()
        });
        if (!res.ok) {
            if (res.status === 404) return null;
            throw new Error(await res.text());
        }

        const blobMimeTypes = ["image", "video", "audio", "application/pdf", "application/octet-stream"];
        const contentType = res.headers.get("content-type");
        if (contentType && blobMimeTypes.some(c => contentType.includes(c))) {
            return await res.blob();
        }

        return await res.text();
    },

    // 🔹 새 파일 생성 (multipart/form-data)
    async create(path: string, fileOrText: File | string): Promise<FileInfo> {
        const form = new FormData();
        form.append("name", path);

        if (fileOrText instanceof File) {
            form.append("file", fileOrText, fileOrText.name);
        } else {
            // text를 Blob으로 감싸서 파일처럼 전송
            form.append("file", new Blob([fileOrText], { type: "text/plain" }), path);
        }

        const res = await vfsFetch(`/vfs`, {
            method: "POST",
            headers: getHeaders(), // Note: fetch automatically adds Content-Type for FormData, we assume getHeaders doesn't override if not passed
            body: form,
        });

        if (!res.ok) throw new Error(await res.text());

        return await res.json(); // {id, name, ...}
    },

    async mkdir(path: string): Promise<FileInfo> {
        const form = new FormData();
        form.append("name", path);
        form.append("isDir", "true");

        const res = await vfsFetch(`/vfs`, {
            method: "POST",
            headers: getHeaders(),
            body: form,
        });

        if (!res.ok) throw new Error(await res.text());

        return await res.json();
    },

    // 파일 쓰기
    async write(id: string, content: string): Promise<boolean> {
        const res = await vfsFetch(`/vfs/${encodeURIComponent(id)}`, {
            method: 'PUT',
            headers: getHeaders('application/json'),
            body: JSON.stringify({ content: content }),
        });

        if (res.status !== 202) throw new Error(await res.text());

        return true;
    },

    // 파일 삭제
    async delete(id: string) {
        const res = await vfsFetch(`/vfs/${encodeURIComponent(id)}`, {
            method: 'DELETE',
            headers: getHeaders()
        });
        if (res.status === 404) return false;
        if (res.status !== 202) throw new Error(await res.text());
        return true;
    },

    // 🔹 파일 이름/경로 변경
    async move(id: string, destPath: string): Promise<boolean> {
        const res = await vfsFetch(`/vfs/${encodeURIComponent(id)}`, {
            method: "PATCH",
            headers: getHeaders('application/json'),
            body: JSON.stringify({ name: destPath }),
        });
        if (res.status !== 202) throw new Error(await res.text());
        return true;
    },

    // 🔹 파일 이름 변경
    async writeComments(id: string, comment: string): Promise<boolean> {
        const res = await vfsFetch(`/vfs/${encodeURIComponent(id)}/comments`, {
            method: "PATCH",
            headers: getHeaders('application/json'),
            body: JSON.stringify({ comment: comment }),
        });
        if (res.status !== 202) throw new Error(await res.text());
        return true;
    },

    // 파일 복사
    async copy(id: string, destPath: string): Promise<boolean> {
        const res = await vfsFetch(`/vfs/${encodeURIComponent(id)}/copy`, {
            method: 'POST',
            headers: getHeaders('application/json'),
            body: JSON.stringify({ name: destPath })
        })
        if (res.status !== 202) throw new Error(await res.text());
        return true;
    }
};

export default vfs;
