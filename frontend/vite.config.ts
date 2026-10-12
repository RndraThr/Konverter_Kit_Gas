import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';
import path from 'node:path';
import tailwindcss from '@tailwindcss/vite';

export default defineConfig(({ command }) => ({
  // Build disajikan Go di /static/app/; base ini dipakai Vite untuk URL aset
  // runtime (mis. worker pdf.js lewat ?url dan preload chunk lazy).
  base: command === 'build' ? '/static/app/' : '/',
  plugins: [react(), tailwindcss()],
  resolve: {
    alias: {
      '@': path.resolve(__dirname, './src'),
    },
  },
  test: {
    environment: 'jsdom',
    setupFiles: './src/test/setup.ts',
    include: ['src/**/*.test.{ts,tsx}'],
  },
  build: {
    outDir: '../web/static/app',
    emptyOutDir: false,
    manifest: true,
  },
}));
