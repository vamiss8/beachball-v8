import { defineConfig } from 'vite';

export default defineConfig({
  server: {
    // proxying the socket keeps the client on a single origin in dev, so the
    // page never needs to know the go server sits on a different port
    proxy: {
      '/ws': {
        target: 'ws://localhost:8080',
        ws: true,
        changeOrigin: true,
      },
    },
  },
});
