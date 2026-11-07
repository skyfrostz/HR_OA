<script setup>
import { onMounted, useTemplateRef } from 'vue'
import { useSession } from './session'
import AnnualLeave from './annual-leave.vue'
const session = useSession()
const heading = useTemplateRef('heading')
onMounted(() => heading.value?.focus())
</script>
<template>
  <section class="oa-pure" aria-labelledby="personal-leave-heading">
    <header class="oa-page-heading"><h1 id="personal-leave-heading" ref="heading" tabindex="-1">我的假期</h1><p class="oa-muted">查看年度额度、审批预占与不可变的余额流水。</p></header>
    <AnnualLeave v-if="session.account?.user.employeeId" :employee-id="session.account.user.employeeId" />
    <p v-else class="oa-surface oa-status">当前账号未绑定员工档案，无法查询本人假期余额。请联系管理员。</p>
  </section>
</template>
