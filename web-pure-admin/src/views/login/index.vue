<script setup lang="ts">
import { useRouter } from "vue-router";
import LoginForm from "@/oa/login-form.vue";
import illustration from "@/assets/login/illustration.svg?url";

const router = useRouter();
</script>

<template>
  <main class="oa-pure-login">
    <aside class="oa-pure-login-intro" aria-label="产品介绍">
      <h1>HR OA 人事工作台</h1>
      <p>管理员工档案与劳动合同，按账号权限访问人事业务。</p>
      <img :src="illustration" alt="" />
    </aside>
    <div class="oa-pure-login-content">
      <LoginForm @authenticated="router.replace({ name: 'OaEmployees' })" />
      <a class="oa-navbar-link" href="./h5.html">前往移动审批</a>
      <small>基于 Vue Pure Admin</small>
    </div>
  </main>
</template>
