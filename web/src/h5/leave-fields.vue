<script setup>
  import { computed, nextTick, shallowRef, watch } from 'vue'
  import { Button } from 'vant'
  import { useSession } from '../oa/session'
  import { useRemote } from '../oa/use-remote'
  import PageState from './page-state.vue'

  const props = defineProps({
    modelValue: { type: Object, required: true },
    disabled: Boolean
  })
  const emit = defineEmits(['update:modelValue', 'validated'])
  const session = useSession()
  const selectedLeaveType = computed({
    get: () => props.modelValue.leaveTypeId ?? '',
    set: (value) => update('leaveTypeId', value === '' ? null : Number(value))
  })
  const types = useRemote((signal) =>
    session.authorized('/leave-types', { signal })
  )
  const {
    result: leaveTypes,
    loading: typesLoading,
    error: typesError,
    load: loadTypes
  } = types
  const previewKey = shallowRef('')
  const validation = shallowRef('')
  const invalidField = shallowRef('')
  const fieldIds = {
    leaveTypeId: 'h5-leave-type',
    startDate: 'h5-field-startDate',
    endDate: 'h5-field-endDate',
    startHalf: 'h5-start-half',
    endHalf: 'h5-end-half'
  }
  const input = computed(() =>
    Object.fromEntries(
      ['leaveTypeId', 'startDate', 'endDate', 'startHalf', 'endHalf'].map(
        (key) => [key, props.modelValue[key]]
      )
    )
  )
  const inputKey = computed(() => JSON.stringify(input.value))
  const previewRead = useRemote((signal) =>
    session.authorized('/leave-preview', {
      method: 'POST',
      data: input.value,
      signal
    })
  )
  const { result, loading, error, load } = previewRead
  const preview = computed(() =>
    previewKey.value === inputKey.value ? result.value : null
  )
  watch(
    inputKey,
    () => {
      previewRead.cancel()
      previewKey.value = ''
      result.value = null
      error.value = ''
      validation.value = ''
      invalidField.value = ''
    },
    { flush: 'sync' }
  )
  watch(preview, (value) => emit('validated', value), {
    immediate: true,
    flush: 'sync'
  })
  void loadTypes()
  function update(name, value) {
    emit('update:modelValue', { ...props.modelValue, [name]: value })
  }
  async function calculate() {
    if (props.disabled || loading.value) return
    validation.value = ''
    const value = input.value
    invalidField.value = ''
    const invalid =
      !Number.isSafeInteger(value.leaveTypeId) || value.leaveTypeId < 1
        ? ['leaveTypeId', '请选择请假类型。']
        : !value.startDate
        ? ['startDate', '请选择开始日期。']
        : !value.endDate
        ? ['endDate', '请选择结束日期。']
        : value.startDate > value.endDate
        ? ['endDate', '结束日期不能早于开始日期。']
        : value.startDate === value.endDate &&
          value.startHalf === 'PM' &&
          value.endHalf === 'AM'
        ? ['endHalf', '同一天的结束时段不能早于开始时段。']
        : null
    if (invalid) {
      invalidField.value = invalid[0]
      validation.value = invalid[1]
      await nextTick()
      const field = document.getElementById(fieldIds[invalidField.value])
      field?.focus({ preventScroll: true })
      field?.closest('.h5-leave-field')?.scrollIntoView?.({
        block: 'center',
        inline: 'nearest'
      })
      return
    }
    result.value = null
    previewKey.value = inputKey.value
    await load()
  }
  const issueLabels = {
    LEAVE_OVERLAP: '与已有请假时段重叠',
    INSUFFICIENT_LEAVE: '年度可用余额不足'
  }
</script>

<template>
  <div class="h5-leave-fields">
    <PageState :loading="typesLoading" :error="typesError" @retry="loadTypes" />
    <p
      v-if="
        leaveTypes &&
        !typesLoading &&
        !leaveTypes.list.some((item) => item.enabled)
      "
      class="h5-muted"
    >
      暂无可用请假类型，请联系管理员。
    </p>
    <div class="h5-leave-field">
      <label for="h5-leave-type"
        >请假类型
        <select
          id="h5-leave-type"
          :aria-invalid="invalidField === 'leaveTypeId' || undefined"
          :aria-describedby="
            invalidField === 'leaveTypeId' ? 'h5-leave-error' : undefined
          "
          v-model="selectedLeaveType"
          :disabled="disabled || typesLoading"
          required
        >
          <option value="" disabled>请选择请假类型</option>
          <option
            v-for="item in leaveTypes?.list.filter((item) => item.enabled) ||
            []"
            :key="item.id"
            :value="item.id"
          >
            {{ item.name
            }}{{ item.usesBalance ? ' · 扣年度余额' : ' · 不扣余额' }}
          </option>
        </select>
      </label>
      <p
        v-if="validation && invalidField === 'leaveTypeId'"
        id="h5-leave-error"
        class="h5-message h5-message-error"
        role="alert"
      >
        {{ validation }}
      </p>
    </div>
    <p class="h5-muted">日期按年、月、日选择，记录格式为 YYYY-MM-DD。</p>
    <div class="h5-leave-dates">
      <div class="h5-leave-field">
        <label for="h5-field-startDate"
          >开始日期
          <input
            id="h5-field-startDate"
            :aria-invalid="invalidField === 'startDate' || undefined"
            :aria-describedby="
              invalidField === 'startDate' ? 'h5-leave-error' : undefined
            "
            :value="modelValue.startDate"
            type="date"
            required
            :disabled="disabled"
            @input="update('startDate', $event.target.value)"
          />
        </label>
        <p
          v-if="validation && invalidField === 'startDate'"
          id="h5-leave-error"
          class="h5-message h5-message-error"
          role="alert"
        >
          {{ validation }}
        </p>
      </div>
      <div class="h5-leave-field">
        <label for="h5-start-half"
          >开始时段
          <select
            id="h5-start-half"
            :aria-invalid="invalidField === 'startHalf' || undefined"
            :aria-describedby="
              invalidField === 'startHalf' ? 'h5-leave-error' : undefined
            "
            :value="modelValue.startHalf"
            :disabled="disabled"
            @change="update('startHalf', $event.target.value)"
          >
            <option value="AM">上午</option>
            <option value="PM">下午</option>
          </select>
        </label>
        <p
          v-if="validation && invalidField === 'startHalf'"
          id="h5-leave-error"
          class="h5-message h5-message-error"
          role="alert"
        >
          {{ validation }}
        </p>
      </div>
      <div class="h5-leave-field">
        <label for="h5-field-endDate"
          >结束日期
          <input
            id="h5-field-endDate"
            :aria-invalid="invalidField === 'endDate' || undefined"
            :aria-describedby="
              invalidField === 'endDate' ? 'h5-leave-error' : undefined
            "
            :value="modelValue.endDate"
            type="date"
            required
            :disabled="disabled"
            @input="update('endDate', $event.target.value)"
          />
        </label>
        <p
          v-if="validation && invalidField === 'endDate'"
          id="h5-leave-error"
          class="h5-message h5-message-error"
          role="alert"
        >
          {{ validation }}
        </p>
      </div>
      <div class="h5-leave-field">
        <label for="h5-end-half"
          >结束时段
          <select
            id="h5-end-half"
            :aria-invalid="invalidField === 'endHalf' || undefined"
            :aria-describedby="
              invalidField === 'endHalf' ? 'h5-leave-error' : undefined
            "
            :value="modelValue.endHalf"
            :disabled="disabled"
            @change="update('endHalf', $event.target.value)"
          >
            <option value="AM">上午</option>
            <option value="PM">下午</option>
          </select>
        </label>
        <p
          v-if="validation && invalidField === 'endHalf'"
          id="h5-leave-error"
          class="h5-message h5-message-error"
          role="alert"
        >
          {{ validation }}
        </p>
      </div>
    </div>
    <label for="h5-field-reason"
      >请假事由<textarea
        id="h5-field-reason"
        :value="modelValue.reason"
        required
        :disabled="disabled"
        @input="update('reason', $event.target.value)"
      />
    </label>
    <p class="h5-muted">
      按管理员配置的工作日历计算；默认周一至周五，无内置法定节假日。
    </p>
    <Button
      :disabled="disabled || loading"
      :loading="loading"
      :aria-describedby="error ? 'h5-leave-preview-error' : undefined"
      @click="calculate"
      >预览请假天数</Button
    >
    <p
      v-if="error"
      id="h5-leave-preview-error"
      class="h5-message h5-message-error"
      role="alert"
    >
      {{ error }}
    </p>
    <section
      v-if="preview"
      class="h5-card h5-block"
      aria-label="请假计算预览"
      aria-live="polite"
    >
      <h3>
        {{ preview.calculation.leaveTypeName }} ·
        {{ preview.calculation.totalHalfDays / 2 }} 天
      </h3>
      <p class="h5-muted">
        {{
          preview.calculation.usesBalance
            ? '共用年度年假额度；提交后预占，最终批准后计入审批已用。'
            : '此类型不扣年度余额，仍校验时段重叠。'
        }}
      </p>
      <ul class="h5-history">
        <li v-for="year in preview.calculation.years" :key="year.year">
          {{ year.year }} 年：{{ year.halfDays / 2 }} 天
        </li>
      </ul>
      <p
        v-for="balance in preview.balances"
        :key="balance.year"
        class="h5-muted"
      >
        {{ balance.year }} 年可用 {{ balance.remainingHalfDays / 2 }} 天
      </p>
      <p
        v-if="!preview.canSubmit"
        class="h5-message h5-message-error"
        role="alert"
      >
        {{
          preview.issues.map((issue) => issueLabels[issue] || issue).join('；')
        }}
      </p>
      <details>
        <summary>查看实际工作时段</summary>
        <ul class="h5-history">
          <li v-for="day in preview.calculation.days" :key="day.date">
            {{ day.date }} ·
            {{
              day.halves
                .map((half) => (half === 'AM' ? '上午' : '下午'))
                .join('、')
            }}
          </li>
        </ul>
      </details>
    </section>
  </div>
</template>

<style scoped>
  .h5-leave-fields {
    display: grid;
    gap: 0.75rem;
    min-width: 0;
  }
  .h5-leave-fields > label,
  .h5-leave-field > label {
    display: grid;
    gap: 0.375rem;
  }
  .h5-leave-field {
    display: grid;
    align-content: start;
    gap: 0.375rem;
    min-width: 0;
    scroll-margin-block: 4rem 7rem;
  }
  .h5-leave-field .h5-message {
    margin: 0;
    font-size: 0.875rem;
  }
  .h5-leave-dates {
    display: grid;
    grid-template-columns: minmax(0, 1fr) minmax(5rem, 0.55fr);
    gap: 0.75rem;
  }
  .h5-leave-dates label {
    display: grid;
    gap: 0.375rem;
    min-width: 0;
  }
  .h5-leave-fields summary {
    min-height: 2.75rem;
    display: list-item;
    cursor: pointer;
    padding-block: 0.5rem;
  }
</style>
