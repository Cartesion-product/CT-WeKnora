<template>
  <header class="workspace-heading" style="--wails-draggable: drag">
    <div class="workspace-heading__scene" aria-hidden="true" />
    <div class="workspace-heading__copy">
      <div class="workspace-heading__eyebrow"><span>{{ section }}</span><i />SenseDeep</div>
      <h1>{{ title }}</h1>
      <p>{{ subtitle }}</p>
    </div>
    <div v-if="$slots.default" class="workspace-heading__actions" style="--wails-draggable: no-drag">
      <slot />
    </div>
  </header>
</template>

<script setup lang="ts">
defineProps<{ title: string; subtitle: string; section: string }>()
</script>

<style scoped lang="less">
.workspace-heading {
  position: relative;
  isolation: isolate;
  display: flex;
  align-items: flex-end;
  justify-content: space-between;
  flex-wrap: wrap;
  gap: 24px;
  flex-shrink: 0;
  min-height: 184px;
  padding: 36px 32px 30px;
  margin: -28px -32px 26px;
  border-bottom: 1px solid var(--td-component-stroke);
  background: var(--sd-heading-surface);
  box-sizing: border-box;
}
.workspace-heading__scene {
  position: absolute;
  inset: 0;
  z-index: -1;
  pointer-events: none;
  background: var(--sd-ocean-poster) right 54% / cover;
  opacity: var(--sd-heading-image-opacity);
  mask-image: linear-gradient(90deg, transparent 22%, #000 100%);
}
.workspace-heading__copy { flex: 1; min-width: min(100%, 250px); }
.workspace-heading__eyebrow {
  display: flex;
  align-items: center;
  gap: 12px;
  color: var(--td-text-color-secondary);
  font-size: 10px;
  font-weight: 500;
  letter-spacing: .18em;
  line-height: 16px;
  margin-bottom: 18px;
  text-transform: uppercase;
  span { color: var(--td-brand-color); }
  i { width: 24px; height: 1px; background: var(--td-component-border); }
}
h1 { color: var(--td-text-color-primary); font-size: clamp(27px, 2.5vw, 36px); font-weight: 500; line-height: 1.3; letter-spacing: .04em; margin: 0; }
p { color: var(--td-text-color-secondary); font-size: 13px; line-height: 1.8; max-width: 620px; margin: 12px 0 0; text-wrap: pretty; }
.workspace-heading__actions { display: flex; align-items: center; flex-wrap: wrap; gap: 8px; max-width: 100%; }
.workspace-heading__actions :deep(.header-actions) { display: flex; align-items: center; flex-wrap: wrap; gap: 8px; max-width: 100%; }
.workspace-heading__actions :deep(.t-button) { min-height: 36px; border-radius: 8px; padding: 0 14px; font-size: 13px; }
.workspace-heading__actions :deep(.header-action-btn) {
  width: auto;
  flex-shrink: 0;
  min-width: 36px;
  color: var(--td-text-color-primary);
  background: var(--sd-control-surface);
  border: 1px solid var(--td-component-border);
  &:hover:not(:disabled) { background: var(--td-bg-color-container-hover); border-color: var(--td-brand-color); }
}
@media (max-width: 900px) {
  .workspace-heading { gap: 18px; }
  .workspace-heading__actions { width: 100%; }
}
@media (max-width: 720px) {
  .workspace-heading { margin: -16px -16px 20px; padding: 28px 20px 24px; min-height: 165px; }
  .workspace-heading__eyebrow { margin-bottom: 12px; }
  .workspace-heading__actions :deep(.header-actions-lite) { width: 100%; }
  .workspace-heading__actions :deep(.resource-toolbar) { width: 100%; }
}
@media (max-height: 650px) and (min-width: 721px) {
  .workspace-heading { min-height: 140px; padding-top: 22px; padding-bottom: 20px; }
  .workspace-heading__eyebrow { margin-bottom: 8px; }
}
</style>
