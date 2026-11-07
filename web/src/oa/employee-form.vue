<script setup>
import { reactive, shallowRef, onMounted, useTemplateRef } from 'vue'
import { ElAlert, ElButton } from 'element-plus'
import { parseSalary } from './format'
import { useSession } from './session'

const emit = defineEmits(['created', 'cancel'])
const session = useSession()
const heading = useTemplateRef('heading')
onMounted(() => heading.value.focus())
const form = reactive({ employeeNo: '', name: '', department: '', birthDate: '', hireDate: '', salary: '' })
const busy = shallowRef(false)
const error = shallowRef('')

async function submit() {
  if (busy.value || !session.canWrite) return
  const { birthDate, hireDate } = form
  const employeeNo = form.employeeNo.trim()
  const name = form.name.trim()
  const department = form.department.trim()
  const salaryCents = parseSalary(form.salary)

  if (!employeeNo || !name || !department || !birthDate || !hireDate || salaryCents === null) {
    error.value = '请填写全部字段。工资须为非负金额，最多保留两位小数。'
    return
  }

  busy.value = true
  error.value = ''
  try {
    const employee = await session.authorized('/employees', {
      method: 'POST',
      data: { employeeNo, name, department, birthDate, hireDate, salaryCents }
    })
    emit('created', employee.id)
  } catch (cause) {
    error.value = !cause.status || cause.status >= 500
      ? '无法确认员工是否已创建。请核对员工列表后再提交。'
      : cause.message
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <section class="oa-inline-form" aria-labelledby="new-employee-heading">
    <h2 id="new-employee-heading" ref="heading" tabindex="-1">新增员工</h2>
    <ElAlert v-if="error" :title="error" type="error" :closable="false" show-icon />
    <form class="oa-form" autocomplete="off" @submit.prevent="submit">
      <div class="oa-fields">
        <label for="employee-no">
          工号
          <input id="employee-no" v-model="form.employeeNo" required :disabled="busy" />
        </label>
        <label for="employee-name">
          姓名
          <input id="employee-name" v-model="form.name" required :disabled="busy" />
        </label>
        <label for="employee-department">
          部门
          <input id="employee-department" v-model="form.department" required :disabled="busy" />
        </label>
        <label for="employee-birth">
          出生日期
          <input id="employee-birth" v-model="form.birthDate" type="date" required :disabled="busy">
        </label>
        <label for="employee-hire">
          入职日期
          <input id="employee-hire" v-model="form.hireDate" type="date" required :disabled="busy">
        </label>
        <label for="employee-salary">
          约定工资（元）
          <input id="employee-salary" v-model="form.salary" inputmode="decimal" required :disabled="busy" />
        </label>
      </div>
      <div class="oa-actions">
        <ElButton type="primary" native-type="submit" :loading="busy">保存员工</ElButton>
        <ElButton :disabled="busy" @click="emit('cancel')">取消</ElButton>
      </div>
    </form>
  </section>
</template>
