import { defineConfig, loadEnv } from 'vite';
import vue from '@vitejs/plugin-vue';

export default defineConfig(({ mode, command }) => {
  const env = loadEnv(mode, process.cwd(), '');
  if (!['http', 'mock'].includes(env.VITE_DATA_SOURCE || 'http')) {
    throw new Error('VITE_DATA_SOURCE 必须为 http 或 mock');
  }
  if (command === 'build' && (env.VITE_DATA_SOURCE || 'http') !== 'http') {
    throw new Error('生产构建必须使用真实 HTTP API，请设置 VITE_DATA_SOURCE=http');
  }
  return {
    root: 'frontend',
    envDir: process.cwd(),
    plugins: [vue()],
    build: { outDir: '../dist', emptyOutDir: true },
    server: { proxy: { '/api': env.API_PROXY_TARGET || 'http://127.0.0.1:8080' } },
  };
});
