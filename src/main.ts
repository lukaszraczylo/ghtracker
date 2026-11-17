import '@fontsource-variable/jetbrains-mono'
import '@fontsource-variable/schibsted-grotesk'
import { configureStrictMode, useThemeStore } from '@perfect-design/ui'
import { createPinia } from 'pinia'
import { createApp } from 'vue'
import App from './App.vue'
import './style.css'

configureStrictMode(import.meta.env.DEV ? 'throw' : 'warn')

const app = createApp(App).use(createPinia())
useThemeStore().init()
app.mount('#app')
