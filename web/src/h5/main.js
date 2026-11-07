import { createApp } from 'vue'
import { createPinia } from 'pinia'
import 'vant/lib/style/base.css'
import 'vant/lib/button/index.css'
import 'vant/lib/loading/index.css'
import 'vant/lib/tag/index.css'
import 'vant/lib/tabbar/index.css'
import 'vant/lib/tabbar-item/index.css'
import App from './app.vue'
import './style.css'

createApp(App).use(createPinia()).mount('#h5-app')
