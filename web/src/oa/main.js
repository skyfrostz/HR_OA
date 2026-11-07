import { createApp } from 'vue'
import { createPinia } from 'pinia'
import 'element-plus/es/components/alert/style/css'
import 'element-plus/es/components/button/style/css'
import 'element-plus/es/components/tag/style/css'
import App from './app.vue'
import './style.css'

createApp(App).use(createPinia()).mount('#oa-app')
