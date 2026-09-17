import { defineConfig } from 'vite';

export default defineConfig({
  server: {
    // pinned to the ipv4 loopback. left to itself, node on windows resolves
    // localhost to ::1 first and vite listens on that address alone, so a
    // browser that reaches localhost over 127.0.0.1 gets a refused connection
    // from a server that just printed it was ready. 127.0.0.1 is still
    // loopback only, unlike exposing the dev server to the whole network
    host: '127.0.0.1',
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
