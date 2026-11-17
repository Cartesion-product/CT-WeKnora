<template>
  <div class="graph-portal">
    <header class="portal-header">
      <div class="portal-brand">
        <t-icon name="relation" size="22px" />
        <span>知识图谱</span>
      </div>
      <div class="portal-account">
        <span class="account-name">{{ authStore.user?.email }}</span>
        <t-button variant="text" size="small" @click="refresh" :loading="loading" title="刷新图谱列表">
          <template #icon><t-icon name="refresh" /></template>
        </t-button>
        <t-button variant="text" size="small" @click="signOut" title="退出登录">
          <template #icon><t-icon name="logout" /></template>
        </t-button>
      </div>
    </header>

    <div class="portal-body">
      <aside class="portal-sidebar">
        <div class="sidebar-heading">
          <h1>可访问的图谱</h1>
          <span>{{ graphs.length }}</span>
        </div>
        <label class="search-field">
          <t-icon name="search" />
          <input v-model="search" type="search" placeholder="搜索知识库" aria-label="搜索知识库" />
        </label>
        <div v-if="loading" class="sidebar-state">正在加载...</div>
        <div v-else-if="error" class="sidebar-state error-state">
          {{ error }}
          <button type="button" @click="refresh">重试</button>
        </div>
        <div v-else-if="!filteredGraphs.length" class="sidebar-state">暂无可访问的图谱</div>
        <nav v-else aria-label="图谱列表" class="graph-list">
          <button v-for="graph in filteredGraphs" :key="graph.id" type="button"
            class="graph-list-item" :class="{ active: graph.id === selectedId }"
            @click="openGraph(graph.id)">
            <t-icon name="relation" />
            <span class="graph-list-copy">
              <strong>{{ graph.name }}</strong>
              <small>{{ graph.shared ? '共享知识库' : '我的空间' }}</small>
            </span>
            <t-icon name="chevron-right" class="row-chevron" />
          </button>
        </nav>
      </aside>

      <main class="portal-main">
        <template v-if="selectedGraph">
          <div class="graph-titlebar">
            <h2>{{ selectedGraph.name }}</h2>
            <span>{{ selectedGraph.shared ? '共享知识库' : '我的空间' }}</span>
            <t-button class="mobile-controls-button" variant="text" size="small"
              :aria-label="showMobileControls ? '收起图谱控件' : '展开图谱控件'"
              :title="showMobileControls ? '收起图谱控件' : '展开图谱控件'"
              @click="showMobileControls = !showMobileControls">
              <template #icon><t-icon name="setting" /></template>
            </t-button>
          </div>
          <div class="graph-surface" :class="{ 'controls-open': showMobileControls }">
            <WikiBrowser :key="selectedGraph.id" :knowledge-base-id="selectedGraph.id" view="graph" :can-edit="false" />
          </div>
        </template>
        <div v-else class="portal-empty">
          <t-icon name="relation" size="36px" />
          <p>{{ requestedId && !loading ? '此图谱不可访问或未启用 Wiki' : '选择一个图谱' }}</p>
        </div>
      </main>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useAuthStore } from '@/stores/auth'
import { getKnowledgeBaseById, listKnowledgeBases } from '@/api/knowledge-base'
import { listSharedKnowledgeBases } from '@/api/organization'
import WikiBrowser from './wiki/WikiBrowser.vue'

interface GraphItem { id: string; name: string; shared: boolean }

const route = useRoute()
const router = useRouter()
const authStore = useAuthStore()
const graphs = ref<GraphItem[]>([])
const loading = ref(false)
const error = ref('')
const search = ref('')
const showMobileControls = ref(false)
const requestedId = computed(() => typeof route.params.kbId === 'string' ? route.params.kbId : '')
const selectedId = computed(() => requestedId.value)
const selectedGraph = computed(() => graphs.value.find(graph => graph.id === selectedId.value))
const filteredGraphs = computed(() => graphs.value.filter(graph => graph.name.toLocaleLowerCase().includes(search.value.toLocaleLowerCase())))

async function refresh() {
  loading.value = true
  error.value = ''
  try {
    const [owned, shared] = await Promise.all([listKnowledgeBases(), listSharedKnowledgeBases()])
    const ownRows = Array.isArray((owned as any)?.data) ? (owned as any).data : []
    if ((owned as any)?.success === false) throw new Error((owned as any).message || '无法读取知识库')
    const ownGraphs: GraphItem[] = ownRows
      .filter((kb: any) => kb.indexing_strategy?.wiki_enabled)
      .map((kb: any) => ({ id: kb.id, name: kb.name, shared: false }))
    const sharedRows = shared.success && Array.isArray(shared.data) ? shared.data : []
    const sharedGraphs = await Promise.all(sharedRows.map(async item => {
      try {
        const response: any = await getKnowledgeBaseById(item.knowledge_base.id)
        const kb = response?.data
        return kb?.indexing_strategy?.wiki_enabled
          ? { id: kb.id, name: kb.name, shared: true } satisfies GraphItem : null
      } catch { return null }
    }))
    const accessible = new Map<string, GraphItem>()
    for (const graph of [...ownGraphs, ...sharedGraphs]) {
      if (graph) accessible.set(graph.id, graph)
    }
    graphs.value = [...accessible.values()]
    if (!requestedId.value && graphs.value.length === 1) void openGraph(graphs.value[0]!.id)
  } catch (cause: any) {
    error.value = cause?.message || '加载失败'
    graphs.value = []
  } finally {
    loading.value = false
  }
}

function openGraph(id: string) {
  showMobileControls.value = false
  return router.push({ name: 'graphPortal', params: { kbId: id } })
}

function signOut() {
  authStore.logout()
  void router.replace({ path: '/login', query: { redirect: '/graphs', email: 'shin@qcznai.com' } })
}

watch(() => authStore.effectiveTenantId, () => { void refresh() })
onMounted(() => { void refresh() })
</script>

<style scoped>
.graph-portal { display: flex; flex-direction: column; width: 100%; height: 100dvh; min-height: 0; color: #232b2b; background: #fff; }
.portal-header { height: 56px; flex: none; display: flex; align-items: center; justify-content: space-between; padding: 0 20px; border-bottom: 1px solid #dce2e0; }
.portal-brand { display: flex; align-items: center; gap: 10px; color: #147c68; font-size: 17px; font-weight: 650; }
.portal-account { display: flex; align-items: center; gap: 4px; }
.account-name { padding-right: 10px; color: #52615e; font-size: 12px; }
.portal-body { display: flex; flex: 1; min-height: 0; }
.portal-sidebar { display: flex; flex-direction: column; width: 272px; flex: none; border-right: 1px solid #dce2e0; background: #f8faf9; }
.sidebar-heading { display: flex; justify-content: space-between; align-items: center; padding: 22px 16px 13px; }
.sidebar-heading h1 { margin: 0; font-size: 15px; font-weight: 650; }
.sidebar-heading span { color: #72807c; font-size: 12px; }
.search-field { height: 34px; margin: 0 12px 12px; padding: 0 9px; display: flex; align-items: center; gap: 7px; border: 1px solid #dce2e0; background: #fff; border-radius: 4px; color: #72807c; }
.search-field input { width: 100%; border: 0; outline: none; background: transparent; font: inherit; font-size: 13px; }
.graph-list { flex: 1; min-height: 0; overflow: auto; padding: 0 8px 12px; }
.graph-list-item { display: flex; align-items: center; gap: 10px; width: 100%; min-height: 58px; padding: 8px 10px; border: 0; border-radius: 4px; background: transparent; color: #3d4b48; text-align: left; cursor: pointer; }
.graph-list-item:hover { background: #e9f1ef; }
.graph-list-item.active { background: #dcefeb; color: #086c5a; }
.graph-list-copy { display: flex; flex: 1; min-width: 0; flex-direction: column; gap: 3px; }
.graph-list-copy strong { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; font-size: 13px; font-weight: 600; }
.graph-list-copy small { color: #74817e; font-size: 11px; }
.row-chevron { color: #99a6a2; }
.sidebar-state { padding: 20px 16px; color: #72807c; font-size: 13px; }
.error-state { color: #bf4e43; }
.error-state button { display: block; margin-top: 8px; padding: 0; border: 0; background: none; color: #147c68; cursor: pointer; }
.portal-main { display: flex; flex: 1; min-width: 0; min-height: 0; flex-direction: column; }
.graph-titlebar { height: 48px; flex: none; display: flex; align-items: center; gap: 12px; padding: 0 18px; border-bottom: 1px solid #edf0ef; }
.graph-titlebar h2 { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; margin: 0; font-size: 14px; font-weight: 600; }
.graph-titlebar span { color: #87938f; font-size: 11px; }
.mobile-controls-button { display: none; }
.graph-surface { flex: 1; min-height: 0; }
.graph-surface :deep(.wiki-graph-canvas) { min-height: 0; }
.portal-empty { flex: 1; display: flex; align-items: center; justify-content: center; flex-direction: column; gap: 8px; color: #8b9995; font-size: 14px; }
.portal-empty p { margin: 0; }
@media (max-width: 720px) {
  .portal-header { padding: 0 12px; }
  .account-name { display: none; }
  .portal-body { flex-direction: column; }
  .portal-sidebar { width: 100%; max-height: 42%; border-right: 0; border-bottom: 1px solid #dce2e0; }
  .sidebar-heading { padding: 12px; }
  .graph-list { display: flex; overflow-x: auto; padding: 0 8px 8px; }
  .graph-list-item { min-width: 180px; max-width: 220px; }
  .graph-titlebar h2 { min-width: 0; }
  .graph-titlebar span { white-space: nowrap; }
  .mobile-controls-button { display: inline-flex; flex: none; margin-left: auto; }
  .graph-surface :deep(.wiki-graph-search-container) { left: 12px; top: 12px; width: calc(100% - 24px); }
  .graph-surface :deep(.wiki-graph-legend) { display: none; }
  .graph-surface.controls-open :deep(.wiki-graph-legend) {
    display: flex;
    top: 56px;
    right: 12px;
    left: 12px;
    width: auto;
    max-width: none;
    max-height: calc(100% - 68px);
    overflow-y: auto;
  }
}
</style>
