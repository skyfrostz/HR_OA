<script setup>
  import { onMounted, useTemplateRef } from 'vue'
  import { shallowRef, watch } from 'vue'
  import { Button } from 'vant'
  import { useSession } from '../oa/session'
  import { useRemote } from '../oa/use-remote'
  import PageState from './page-state.vue'
  import Pager from './pager.vue'
  import LeaveLedger from './leave-ledger.vue'
  const emit = defineEmits(['open'])
  const session = useSession()
  const heading = useTemplateRef('heading')
  onMounted(() => heading.value?.focus())
  const employeeId = session.account?.user.employeeId
  const page = shallowRef(1)
  const { result, loading, error, load } = useRemote((signal) =>
    session.authorized(
      '/employees/' +
        employeeId +
        '/annual-leave?page=' +
        page.value +
        '&pageSize=20',
      { signal }
    )
  )
  watch(
    page,
    () => {
      if (employeeId) void load()
    },
    { immediate: true }
  )
</script>
<template>
  <section aria-labelledby="h5-balance-heading">
    <header class="h5-heading h5-list-heading">
      <h1 id="h5-balance-heading" ref="heading" tabindex="-1">我的假期</h1>
      <Button v-if="employeeId" :disabled="loading" @click="load">刷新</Button>
    </header>
    <p v-if="!employeeId" class="h5-card">
      当前账号未绑定员工档案，请联系管理员。
    </p>
    <template v-else>
      <PageState :loading="loading" :error="error" @retry="load" />
      <p v-if="result && !result.list.length" class="h5-card">
        暂无年度额度，请联系管理员核对。
      </p>
      <ul v-if="!loading && result" class="h5-list">
        <li v-for="row in result.list" :key="row.id" class="h5-card">
          <div class="h5-balance-summary">
            <h2>{{ row.year }} 年</h2>
            <p>
              <span>可用余额</span
              ><strong
                >{{ row.remainingHalfDays / 2 }}<small> 天</small></strong
              >
            </p>
          </div>
          <dl class="h5-details h5-balance-details">
            <div>
              <dt>应休</dt>
              <dd>{{ row.entitledHalfDays / 2 }} 天</dd>
            </div>
            <div>
              <dt>人工已用</dt>
              <dd>{{ row.usedHalfDays / 2 }} 天</dd>
            </div>
            <div>
              <dt>审批已用</dt>
              <dd>{{ (row.approvalUsedHalfDays ?? 0) / 2 }} 天</dd>
            </div>
            <div>
              <dt>预占</dt>
              <dd>{{ (row.reservedHalfDays ?? 0) / 2 }} 天</dd>
            </div>
          </dl>
        </li>
      </ul>
      <Pager
        v-if="result"
        :page="page"
        :total="result.total"
        :page-size="result.pageSize"
        :disabled="loading"
        @change="page = $event"
      />
      <LeaveLedger :employee-id="employeeId" @open="emit('open', $event)" />
    </template>
  </section>
</template>
