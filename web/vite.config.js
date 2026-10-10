import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import AutoImport from 'unplugin-auto-import/vite'
import Components from 'unplugin-vue-components/vite'
import { ElementPlusResolver } from 'unplugin-vue-components/resolvers'
import { fileURLToPath, URL } from 'node:url'

// Vite 配置：Vue3 + Element Plus 按需引入 + 路径别名 + 开发代理
export default defineConfig({
  plugins: [
    vue(),
    // 自动导入 Vue / VueRouter / Pinia API
    AutoImport({
      imports: ['vue', 'vue-router', 'pinia'],
      resolvers: [ElementPlusResolver()],
    }),
    // 自动注册 Element Plus 组件
    Components({
      resolvers: [ElementPlusResolver()],
    }),
  ],
  resolve: {
    alias: {
      '@': fileURLToPath(new URL('./src', import.meta.url)),
    },
  },
  server: {
    port: 5173,
    open: true,
    proxy: {
      // 开发环境代理到网关管理面 8081
      '/api': {
        target: 'http://127.0.0.1:8081',
        changeOrigin: true,
      },
      '/healthz': {
        target: 'http://127.0.0.1:8081',
        changeOrigin: true,
      },
      '/readyz': {
        target: 'http://127.0.0.1:8081',
        changeOrigin: true,
      },
    },
  },
  build: {
    outDir: 'dist',
    sourcemap: false,
    chunkSizeWarningLimit: 1500,
    rollupOptions: {
      output: {
        // 第三方库分包，提升缓存命中率
        manualChunks: {
          vue: ['vue', 'vue-router', 'pinia'],
          element: ['element-plus'],
          echarts: ['echarts', 'vue-echarts'],
        },
      },
    },
  },
})
