import {defineConfig} from 'vite'
import vue from '@vitejs/plugin-vue'

export default defineConfig({
  base:'./',
  plugins:[vue()],
  server:{
    proxy:{
      '/api':{
        target:'http://127.0.0.1:18780',
        // Keep Host aligned with the browser's Origin for backend CSRF checks.
        changeOrigin:false
      }
    }
  }
})
