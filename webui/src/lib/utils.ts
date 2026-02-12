export function joinPath(...paths: string[]): string {
    const joined = paths
        .map((p) => p.trim())        // 공백 제거
        .filter((p) => p.length > 0) // 빈 문자열 제외
        .join("/");                   // 단순 join

    // 중복 슬래시 제거
    return joined.replace(/\/{2,}/g, "/");
}

export function resolvePath(currentPath: string, inputPath: string): string {
    const base = currentPath.endsWith("/")
        ? currentPath.slice(0, -1)
        : currentPath;

    // 절대 경로면 바로 normalize
    if (inputPath.startsWith("/")) {
        return normalizePath(inputPath);
    }

    // ".", "./"
    if (inputPath === "." || inputPath === "./") {
        return normalizePath(base);
    }

    // 단순 결합 후 정규화
    const combined = joinPath(base, inputPath);
    let resolved = normalizePath(combined);
    // 무조건 절대경로 반환 보장
    if (!resolved.startsWith("/")) {
        resolved = "/" + resolved;
    }
    return resolved;
}

// 경로 정규화: ".", ".." 처리
export function normalizePath(path: string): string {
    const parts = path.split("/");
    const stack: string[] = [];

    for (const part of parts) {
        if (part === "" || part === ".") continue;
        if (part === "..") {
            stack.pop();
        } else {
            stack.push(part);
        }
    }

    // 루트 경로 처리
    if (path.startsWith("/")) {
        return "/" + stack.join("/");
    }
    return stack.join("/");
}

export function inferType(filename: string) {
    const ext = filename.split('.').pop();
    const map: { [key: string]: string } = {
        ts: 'application/typescript', js: 'application/javascript',
        md: 'text/markdown',
        txt: 'text/plain',
        json: 'application/json',
        jpg: 'image/jpeg', jpeg: 'image/jpeg', png: 'image/png',
        gif: 'image/gif', webp: 'image/webp', svg: 'image/svg+xml',
        mp4: 'video/mp4', webm: 'video/webm', mp3: 'audio/mpeg', wav: 'audio/wav',
        avi: 'video/x-msvideo', mov: 'video/quicktime', mkv: 'video/x-matroska',
        pdf: 'application/pdf'
    };
    return map[ext ? ext.toLowerCase() : ''] || 'application/octet-stream';
}

import { type TreeNode } from './vfs';

/**
 * TreeNode 구조를 트리 형태의 문자열로 변환합니다.
 * @param node 현재 노드
 * @param prefix 들여쓰기 및 줄기 문자열
 * @param isLast 마지막 자식 노드인지 여부
 */
export function formatTreeToString(
    node: TreeNode,
    prefix: string = "",
    isLast: boolean = true
): string {
    // 1. 현재 노드의 이름 결정 (디렉토리면 뒤에 / 추가)
    const name = node.meta.name;

    // 2. 현재 노드 앞에 붙을 마커 결정
    const marker = isLast ? "└── " : "├── ";

    // 3. 현재 행 구성 (루트 노드인 경우 마커 없이 이름만 출력 가능)
    let result = prefix + marker + name + "\n";

    // 4. 자식 노드 처리
    if (node.children && node.children.length > 0) {
        // 부모의 줄기(Vertical line)를 유지할지 결정
        const nextPrefix = prefix + (isLast ? "    " : "│   ");

        node.children.forEach((child, index) => {
            const lastChild = index === node.children.length - 1;
            result += formatTreeToString(child, nextPrefix, lastChild);
        });
    }

    return result;
}

export function getParentPath(path: string): string {
    if (path === "/" || path === "") return "/";
    // remove trailing slash if exists (unless root)
    const p = path.length > 1 && path.endsWith('/') ? path.slice(0, -1) : path;
    const lastSlash = p.lastIndexOf("/");
    if (lastSlash <= 0) return "/";
    return p.slice(0, lastSlash);
}

export function isAncestorOrSame(ancestor: string, path: string): boolean {
    const a = ancestor.length > 1 && !ancestor.endsWith("/") ? ancestor + "/" : ancestor;
    const p = path.length > 1 && !path.endsWith("/") ? path + "/" : path;

    // Root handling
    if (a === "/" || a === "") return true; // Root is ancestor of everything

    return p.startsWith(a);
}
