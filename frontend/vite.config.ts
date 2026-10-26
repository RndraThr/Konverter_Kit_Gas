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
    alias: [
      { find: '@', replacement: path.resolve(__dirname, './src') },
      // OpenCV.js shipped inside jscanify, served from our own origin by the
      // document scanner (the docs.opencv.org copy sends no CORS header, so
      // a browser blocks it whenever integrity checking is on). A regex so
      // the "?url" suffix is kept.
      { find: /^opencv-js-asset/, replacement: path.resolve(__dirname, './node_modules/jscanify/src/opencv.js') },
    ],
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
