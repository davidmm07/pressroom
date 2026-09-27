/// <reference types="vitest/config" />
import react from '@vitejs/plugin-react';
import { defineConfig } from 'vite';

export default defineConfig({
  plugins: [react()],
  server: {
    port: 5173,
    // In development the dashboard talks to the local API through Vite, so
    // no CORS setup is needed.
    proxy: { '/graphql': process.env.PRESSROOM_API ?? 'http://localhost:8080' },
  },
  test: {
    environment: 'node',
    include: ['src/**/*.test.ts'],
  },
});
