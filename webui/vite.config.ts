/// <reference types="vitest" />
import { defineConfig, mergeConfig } from "vite";
import { defineConfig as defineTestConfig } from "vitest/config";
import { svelte } from '@sveltejs/vite-plugin-svelte'

// https://vitejs.dev/config/
const viteConfig = defineConfig({
    plugins: [svelte()],
    build: {
        outDir: 'dist',
        emptyOutDir: true
    },
    server: {
        proxy: {
            '/vfs': {
                target: 'http://localhost:3000',
                changeOrigin: true,
                rewrite: (path: string) => path
            },
            '/sse': {
                target: 'http://localhost:3000',
                changeOrigin: true,
                rewrite: (path: string) => path
            }
        }
    },
    resolve: {
        conditions: ['browser']
    },
});

const testConfig = defineTestConfig({
    test: {
        globals: true,
        environment: 'jsdom',
        setupFiles: './src/setupTests.ts',
        include: ['src/**/*.{test,spec}.{js,ts}'],
    }
});

export default mergeConfig(viteConfig, testConfig);
