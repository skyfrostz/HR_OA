<script setup>
import { useWeComLogin } from '../oa/use-wecom-login'
defineProps({ disabled: Boolean })
const emit = defineEmits(['busy'])
const { mode, error, busy } = useWeComLogin(() => {}, value => emit('busy', value))
</script>
<template>
  <section class="h5-wecom-login" aria-label="企业微信登录">
    <p v-if="busy" role="status">正在完成企业微信登录…</p>
    <p v-if="error" class="h5-message h5-message-error" role="alert">{{ error }}</p>
    <a v-if="['mock', 'configured'].includes(mode)" class="h5-wecom-entry" :href="disabled || busy ? undefined : '/api/oa/v1/auth/wecom/start?entry=h5'" :aria-disabled="disabled || busy">{{ mode === 'mock' ? '企业微信模拟登录' : '企业微信登录' }}</a>
    <p class="h5-muted">{{ mode === 'mock' ? '当前使用模拟授权服务。' : mode === 'configured' ? '仅限已绑定的企业成员；配置就绪不代表完成企业验收。' : mode === 'loading' ? '正在检查企业微信登录状态…' : mode === 'disabled' ? '企业微信登录尚未配置，可先使用账号登录。' : '暂时无法读取企业微信状态，可先使用账号登录。' }}</p>
  </section>
</template>
