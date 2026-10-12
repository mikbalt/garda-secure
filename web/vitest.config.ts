import { defineConfig, mergeConfig } from 'vitest/config'
import viteConfig from './vite.config'

// Unit tests (WEB-5): jsdom + Testing Library. End-to-end tests live in e2e/ (Playwright).
export default mergeConfig(
  viteConfig,
  defineConfig({
    test: {
      environment: 'jsdom',
      include: ['src/**/*.test.{ts,tsx}'],
      setupFiles: ['src/test/setup.ts'],
      restoreMocks: true,
    },
  })
)
