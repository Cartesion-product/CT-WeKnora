<template>
  <span class="brand-mark" :class="{ 'brand-mark--ocean': variant === 'ocean' }">
    <svg class="brand-mark__icon" viewBox="0 0 48 48" fill="none" aria-hidden="true">
      <!-- An open horizon and descending currents: knowledge with depth. -->
      <path d="M8 15.5C11.4 8.3 19.1 4.6 27 6.1C33 7.2 37.9 11.4 40 17" stroke="currentColor" stroke-opacity=".55" stroke-width="1.2" stroke-linecap="round" />
      <path d="M7 21C13 15.8 18.2 16.1 24 20.2C29.8 24.3 35 24.6 41 19.4" stroke="currentColor" stroke-width="2" stroke-linecap="round" />
      <path d="M9 28C14.2 24.1 19.1 24.2 24 27.4C28.9 30.6 33.8 30.7 39 26.8" stroke="currentColor" stroke-opacity=".8" stroke-width="1.7" stroke-linecap="round" />
      <path d="M13 35C16.9 32.8 20.4 33.1 24 35C27.6 36.9 31.1 37.2 35 35" stroke="currentColor" stroke-opacity=".55" stroke-width="1.4" stroke-linecap="round" />
      <circle class="brand-mark__depth" cx="24" cy="42" r="1.3" fill="currentColor" fill-opacity=".65" />
    </svg>
    <span v-if="!iconOnly" class="brand-mark__name">{{ PRODUCT_SHORT_NAME }}<span class="brand-mark__descriptor">{{ PRODUCT_ENGLISH_NAME }}</span></span>
  </span>
</template>

<script setup lang="ts">
import { PRODUCT_SHORT_NAME, PRODUCT_ENGLISH_NAME } from '@/config/brand'

withDefaults(defineProps<{ variant?: 'default' | 'ocean'; iconOnly?: boolean }>(), { variant: 'default', iconOnly: false })
</script>

<style scoped>
.brand-mark { display: inline-flex; align-items: center; gap: 8px; min-width: 0; white-space: nowrap; color: var(--td-brand-color); }
.brand-mark__icon { width: 34px; height: 34px; flex: none; }
.brand-mark__depth { color: #b57947; fill-opacity: 1; }
.brand-mark__name { display: flex; flex-direction: column; gap: 3px; color: var(--td-text-color-primary); font-size: 15px; font-weight: 650; line-height: 1; }
.brand-mark__descriptor { color: var(--td-text-color-secondary); font-size: 9px; font-weight: 600; line-height: 1.2; }
.brand-mark--ocean { gap: 14px; color: #d8eee9; }
.brand-mark--ocean .brand-mark__depth { color: inherit; fill-opacity: .65; }
.brand-mark--ocean .brand-mark__icon { width: 46px; height: 46px; }
.brand-mark--ocean .brand-mark__name {
  display: flex;
  flex-direction: column;
  gap: 7px;
  color: #edf7f3;
  font-size: 23px;
  font-weight: 500;
  letter-spacing: .22em;
  text-shadow: 0 2px 14px rgba(0, 22, 29, .25);
}
.brand-mark--ocean .brand-mark__descriptor {
  margin: 0;
  color: #c4deda;
  font-size: 9px;
  font-weight: 500;
  line-height: 1.3;
  letter-spacing: .24em;
  text-transform: uppercase;
}
@media (max-width: 768px) {
  .brand-mark--ocean { gap: 10px; }
  .brand-mark--ocean .brand-mark__icon { width: 36px; height: 36px; }
  .brand-mark--ocean .brand-mark__name { font-size: 19px; gap: 5px; }
  .brand-mark--ocean .brand-mark__descriptor { font-size: 8px; letter-spacing: .2em; }
}
</style>
