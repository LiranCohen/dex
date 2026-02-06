/// <reference types="vitest" />
import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';

export default defineConfig({
  plugins: [react()],
  test: {
    globals: true,
    environment: 'jsdom',
    setupFiles: ['./src/test/setup.ts'],
    include: ['src/**/*.test.{ts,tsx}'],
    coverage: {
      provider: 'v8',
      reporter: ['text', 'json', 'html'],
      include: [
        'src/app/components/**/*.{ts,tsx}',
        'src/app/hooks/**/*.{ts,tsx}',
        'src/app/pages/**/*.{ts,tsx}',
        'src/app/utils/**/*.{ts,tsx}',
        'src/components/QuestChat/utils.ts',
        'src/hooks/useWebSocket.ts',
      ],
      exclude: [
        'src/**/*.test.{ts,tsx}',
        'src/test/**/*',
        'src/**/index.ts',
        'src/**/*.d.ts',
      ],
      thresholds: {
        // Per-file thresholds for tested components
        'src/app/components/*.tsx': {
          lines: 65,
          functions: 65,
          branches: 55,
          statements: 65,
        },
        'src/app/components/chat/*.tsx': {
          lines: 65,
          functions: 65,
          branches: 55,
          statements: 65,
        },
        'src/app/hooks/*.ts': {
          lines: 90,
          functions: 90,
          branches: 90,
          statements: 90,
        },
        'src/components/QuestChat/utils.ts': {
          lines: 90,
          functions: 90,
          branches: 85,
          statements: 90,
        },
      },
    },
  },
});
