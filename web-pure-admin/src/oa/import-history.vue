<script setup>
import { shallowRef, watch } from 'vue'
import { ElButton } from 'element-plus'
import { useRemote } from './use-remote'
import { useSession } from './session'
import PageNav from './page-nav.vue'
import RemoteState from './remote-state.vue'
const props = defineProps({ revision: Number, disabled: Boolean })
const emit = defineEmits(['open'])
const session = useSession()
const page = shallowRef(1)
const { result, loading, error, load } = useRemote(signal => session.authorized('/employee-imports?page=' + page.value + '&pageSize=20', { signal }))
watch([page, () => props.revision], load, { immediate: true })
const status = { PREVIEW: '待确认', EXPIRED: '已过期', COMMITTED: '已导入' }
const date = value => value ? new Date(value).toLocaleString('zh-CN', { hour12: false }) : '—'
</script>
<template>
  <section class="oa-surface oa-import-preview" aria-labelledby="oa-import-history">
    <header class="oa-section-head"><div><h2 id="oa-import-history">我的导入记录</h2><p class="oa-muted">只展示本人创建的预览及导入结果。</p></div><ElButton :disabled="disabled || loading" @click="load">刷新记录</ElButton></header>
    <RemoteState :loading="loading" :error="error" :empty="Boolean(result && !result.list.length)" empty-text="暂无导入记录。" @retry="load" />
    <div v-if="!loading && result?.list.length" class="oa-table-wrap">
      <table class="oa-table" aria-label="我的导入记录"><thead><tr><th>创建时间</th><th>状态</th><th>行数</th><th>导入人数</th><th>操作</th></tr></thead><tbody>
        <tr v-for="row in result.list" :key="row.id"><th scope="row">{{ date(row.createdAt) }}</th><td data-label="状态">{{ status[row.status] || row.status }}</td><td data-label="行数">{{ row.totalRows }}</td><td data-label="导入人数">{{ row.status === 'COMMITTED' ? row.importedCount : '—' }}</td><td><ElButton text type="primary" :disabled="disabled" @click="emit('open', row.id)">{{ row.status === 'COMMITTED' ? '查看结果' : '查看预览' }}</ElButton></td></tr>
      </tbody></table>
    </div>
    <PageNav v-if="result" :page="page" :total="result.total" :page-size="result.pageSize" :disabled="disabled || loading" label="导入记录分页" @change="page = $event" />
  </section>
</template>
