import '@fontsource-variable/jetbrains-mono'
import '@fontsource-variable/schibsted-grotesk'
import { createPinia } from 'pinia'
import { createApp } from 'vue'
import App from './App.vue'
import './style.css'

createApp(App).use(createPinia()).mount('#app')
