<script setup>
import { shallowRef } from 'vue'
import { useRouteWriteLock } from './use-route-write-lock'
import LeaveTypes from './settings/leave-types.vue'
import WorkCalendar from './settings/work-calendar.vue'
import ApprovalRules from './settings/approval-rules.vue'
const selected = shallowRef('types')
const locked = shallowRef(false)
const tabs = [{ id: 'types', label: '请假类型', component: LeaveTypes }, { id: 'calendar', label: '工作日历', component: WorkCalendar }, { id: 'rules', label: '审批规则', component: ApprovalRules }]
useRouteWriteLock(locked)
</script>
<template>
  <section class="oa-pure" aria-labelledby="workflow-settings-heading">
    <header class="oa-page-heading"><h1 id="workflow-settings-heading">请假与审批配置</h1><p class="oa-muted">配置面向以后提交的申请，历史计算和审批路径保持不变。</p></header>
    <nav class="oa-segmented oa-settings-tabs" aria-label="配置分组"><button v-for="tab in tabs" :key="tab.id" type="button" :aria-pressed="selected === tab.id" :disabled="locked" @click="selected = tab.id">{{ tab.label }}</button></nav>
    <div class="oa-surface"><component :is="tabs.find(tab => tab.id === selected).component" @locked="locked = $event" /></div>
  </section>
</template>
