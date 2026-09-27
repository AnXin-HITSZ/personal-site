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
    plugins: [vue()],
    build: { outDir: 'dist', emptyOutDir: true },
    server: {
      proxy: {
        /* 字符串写法会被 Vite 补上 changeOrigin: true，于是 Host 被改写成
           后端自己的地址。后端的同源判断拿 Host 和浏览器发来的 Origin 比，
           对不上就把所有写操作挡成 403。保持 Host 原样，两边自然同源。 */
        '/api': { target: env.API_PROXY_TARGET || 'http://127.0.0.1:8080', changeOrigin: false },
      },
    },
  };
});
