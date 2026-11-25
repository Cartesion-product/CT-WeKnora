<template>
    <div class="dialogue-wrap">
        <div class="dialogue-answers">
            <div class="chat-ocean-emblem" aria-hidden="true"><BrandMark icon-only /></div>
            <div class="chat-ocean-label" aria-hidden="true">SenseDeep / Knowledge in depth</div>
            <div class="dialogue-title" style="--wails-draggable: drag">
                <span style="--wails-draggable: drag">{{ $t('createChat.title') }}</span>
            </div>
            <p class="chat-ocean-description">{{ $t('createChat.description') }}</p>
            <!-- 推荐问题 -->
            <div ref="sqContainerRef" class="suggested-questions-container">
                <!-- 骨架屏占位 -->
                <div v-if="sqInitialLoad && suggestedQuestions.length === 0" class="suggested-questions-inner">
                    <div class="suggested-questions-title"><t-skeleton animation="gradient"
                            :row-col="[{ width: '120px', height: '14px' }]" /></div>
                    <div class="suggested-questions-grid">
                        <div v-for="n in 6" :key="'sq-skel-' + n" class="suggested-question-card sq-card-skeleton">
                            <t-skeleton animation="gradient"
                                :row-col="[{ width: '100%', height: '14px', type: 'rect' }]" />
                        </div>
                    </div>
                </div>
                <transition v-else appear name="sq-slide-fade" mode="out-in" @before-leave="onBeforeLeave"
                    @after-leave="onAfterLeave" @enter="onEnter" @after-enter="onQuestionsEntered">
                    <div v-if="suggestedQuestions.length > 0" :key="sqRenderKey" class="suggested-questions-inner">
                        <div class="suggested-questions-title-row">
                            <p class="suggested-questions-caption">
                                <span class="suggested-questions-title">{{ $t('chat.suggestedQuestions') }}</span>
                                <button type="button" class="suggested-questions-refresh" :disabled="sqLoading"
                                    :title="$t('chat.refreshSuggestedQuestions')"
                                    :aria-label="$t('chat.refreshSuggestedQuestions')" @click="fetchSuggestedQuestions(true)">
                                    <t-icon :name="sqLoading ? 'loading' : 'refresh'"
                                        :class="{ 'sq-refresh-spin': sqLoading }" />
                                </button>
                            </p>
                        </div>
                        <div class="suggested-questions-grid">
                            <div v-for="(item, index) in suggestedQuestions" :key="item.question"
                                class="suggested-question-card" :class="{ 'sq-card-visible': sqCardsRevealed }"
                                :style="{ transitionDelay: sqCardsRevealed ? `${index * 50}ms` : '0ms' }"
                                @click="handleSuggestedQuestionClick(item)">
                                <span class="suggested-question-text">{{ item.question }}</span>
                                <span v-if="item.source === 'faq'" class="suggested-question-badge faq">FAQ</span>
                            </div>
                        </div>
                    </div>
                </transition>
            </div>
            <div class="create-chat-composer">
                <div v-if="hostSandboxEnabled" class="project-dir-bar">
                    <button type="button" class="project-dir-bar__btn"
                        :class="{ 'is-bound': !!selectedProjectDir, 'is-picking': pickingProjectDir }"
                        :disabled="pickingProjectDir" :title="selectedProjectDir || $t('createChat.openProject')"
                        @click="openProjectDir">
                        <t-icon :name="pickingProjectDir ? 'loading' : 'folder'" />
                        <span class="project-dir-bar__name">{{
                            selectedProjectDir ? projectDirBasename(selectedProjectDir) : $t('createChat.openProject')
                        }}</span>
                    </button>
                    <button v-if="selectedProjectDir" type="button" class="project-dir-bar__clear"
                        :aria-label="$t('createChat.clearProject')" @click="clearProjectDir">×</button>
                </div>
                <InputField ref="inputFieldRef" @send-msg="sendMsg"></InputField>
            </div>
            <nav class="chat-workspace-links" :aria-label="$t('knowledgeBase.title')">
                <RouterLink to="/platform/knowledge-bases"><ResourceIcon type="knowledge" :size="18" />{{ $t('knowledgeBase.title') }}<span aria-hidden="true">↗</span></RouterLink>
                <RouterLink to="/platform/agents"><ResourceIcon type="agent" :size="18" />{{ $t('agent.title') }}<span aria-hidden="true">↗</span></RouterLink>
                <RouterLink to="/platform/artifacts"><ResourceIcon type="artifact" :size="18" />{{ $t('artifactLibrary.title') }}<span aria-hidden="true">↗</span></RouterLink>
            </nav>
        </div>
    </div>


    <!-- 知识库编辑器（创建/编辑统一组件） -->
    <KnowledgeBaseEditorModal :visible="uiStore.showKBEditorModal" :mode="uiStore.kbEditorMode"
        :kb-id="uiStore.currentKBId || undefined" :initial-type="uiStore.kbEditorType"
        @update:visible="(val) => val ? null : uiStore.closeKBEditor()" @success="handleKBEditorSuccess" />
</template>
<script setup lang="ts">
import { ref, watch, onMounted, nextTick, computed } from 'vue';
import BrandMark from '@/components/BrandMark.vue';
import ResourceIcon from '@/components/icons/ResourceIcon.vue';
import InputField from '@/components/Input-field.vue';
import { createSessions } from "@/api/chat/index";
import { pickHostProjectDir } from '@/utils/desktopProjectDir';
import { projectDirBasename, shouldRenderHostProjectSettings, withOptionalProjectDir } from '@/utils/hostWorkspace';
import { getSuggestedQuestions } from "@/api/agent/index";
import type { SuggestedQuestion } from "@/api/agent/index";
import { questionOriginFromSuggestion, type SendMessageOptions } from '@/utils/questionOrigin';
import { useMenuStore } from '@/stores/menu';
import { useSettingsStore } from '@/stores/settings';
import { useUIStore } from '@/stores/ui';
import { useDeploymentCapabilitiesStore } from '@/stores/deploymentCapabilities';
import { useRoute, useRouter, onBeforeRouteLeave } from 'vue-router';
import { MessagePlugin } from 'tdesign-vue-next';
import { useI18n } from 'vue-i18n';
import KnowledgeBaseEditorModal from '@/views/knowledge/KnowledgeBaseEditorModal.vue';
import { useKnowledgeBaseCreationNavigation } from '@/hooks/useKnowledgeBaseCreationNavigation';

const router = useRouter();
const route = useRoute();
const usemenuStore = useMenuStore();
const settingsStore = useSettingsStore();
onBeforeRouteLeave((to) => {
    // The first send carries the draft into its new session; abandoning the
    // composer must not make this a default for the next conversation.
    if (!to.path.startsWith('/platform/chat/') || !usemenuStore.isFirstSession) {
        settingsStore.reasoningEffortOverride = '';
    }
});
const uiStore = useUIStore();
const deploymentCapabilities = useDeploymentCapabilitiesStore();
const { t } = useI18n();
const { navigateToKnowledgeBaseList } = useKnowledgeBaseCreationNavigation();

const hostSandboxEnabled = computed(() =>
    shouldRenderHostProjectSettings(deploymentCapabilities.isSupported('settings.sandbox.host')),
);
const selectedProjectDir = ref('');
const pickingProjectDir = ref(false);

// ===== 推荐问题 =====
const suggestedQuestions = ref<SuggestedQuestion[]>([]);
const sqLoading = ref(true);
// The skeleton reserves space only until the first answer arrives. Later
// refetches keep the current content in place, so the composer below does not
// jump each time the agent or @mention selection changes.
const sqInitialLoad = ref(true);
const sqCardsRevealed = ref(false);
const sqRenderKey = ref(0);
const sqContainerRef = ref<HTMLElement | null>(null);
let suggestedQuestionsFetchId = 0;
let lastSuggestedQuestionsKey = '';
let debounceTimer: ReturnType<typeof setTimeout> | null = null;

const sameSuggestedQuestions = (a: SuggestedQuestion[], b: SuggestedQuestion[]) =>
    a.length === b.length && a.every((item, i) => item.question === b[i].question && item.source === b[i].source);

// --- 高度平滑过渡钩子 ---
const onBeforeLeave = () => {
    const c = sqContainerRef.value;
    if (!c) return;
    c.style.height = c.offsetHeight + 'px';
    c.style.overflow = 'hidden';
};

const onAfterLeave = () => {
    const c = sqContainerRef.value;
    if (!c) return;
    if (suggestedQuestions.value.length === 0) {
        requestAnimationFrame(() => { c.style.height = '0px'; });
        c.addEventListener('transitionend', () => {
            c.style.height = '';
            c.style.overflow = '';
        }, { once: true });
    }
};

const onEnter = (el: Element) => {
    const c = sqContainerRef.value;
    if (!c) return;
    const startHeight = c.offsetHeight;
    c.style.height = 'auto';
    c.style.overflow = 'hidden';
    const targetHeight = c.offsetHeight;
    c.style.height = startHeight + 'px';
    requestAnimationFrame(() => {
        c.style.height = targetHeight + 'px';
    });
};

const onQuestionsEntered = () => {
    const c = sqContainerRef.value;
    if (c) {
        c.style.height = '';
        c.style.overflow = '';
    }
    nextTick(() => { sqCardsRevealed.value = true; });
};

const fetchSuggestedQuestions = async (force = false) => {
    const agentId = settingsStore.selectedAgentId;
    const params = settingsStore.getSuggestedQuestionsParams();
    const requestKey = JSON.stringify([agentId, params]);
    if (!force && requestKey === lastSuggestedQuestionsKey) return;
    lastSuggestedQuestionsKey = requestKey;

    const fetchId = ++suggestedQuestionsFetchId;
    sqLoading.value = true;
    try {
        if (!agentId) {
            suggestedQuestions.value = [];
            return;
        }
        const res = await getSuggestedQuestions(agentId, params);
        if (fetchId === suggestedQuestionsFetchId) {
            const next = res?.data?.questions || [];
            if (!sameSuggestedQuestions(suggestedQuestions.value, next)) {
                sqCardsRevealed.value = false;
                sqRenderKey.value++;
                suggestedQuestions.value = next;
            }
        }
    } catch (err) {
        console.warn('[SuggestedQuestions] Failed to fetch:', err);
        if (fetchId === suggestedQuestionsFetchId) {
            lastSuggestedQuestionsKey = '';
            suggestedQuestions.value = [];
        }
    } finally {
        if (fetchId === suggestedQuestionsFetchId) {
            sqLoading.value = false;
            sqInitialLoad.value = false;
        }
    }
};

// 防抖包装，切换知识库/文件时300ms内不重复请求
const debouncedFetch = () => {
    if (debounceTimer) clearTimeout(debounceTimer);
    debounceTimer = setTimeout(() => { fetchSuggestedQuestions(); }, 300);
};

// 监听 Agent / 知识库 / 文件 / 标签 @mention；MCP 与 Skill 不影响推荐范围
watch(
    () => ({
        agentId: settingsStore.selectedAgentId,
        kbs: settingsStore.settings.selectedKnowledgeBases,
        files: settingsStore.settings.selectedFiles,
        tags: settingsStore.settings.selectedTags,
    }),
    debouncedFetch,
    { deep: true },
);

onMounted(() => {
    fetchSuggestedQuestions();
});

const inputFieldRef = ref();

// The suggestion's source rides with this send to the new session's first
// request, so the agent searches it before answering.
const handleSuggestedQuestionClick = (item: SuggestedQuestion) => {
    inputFieldRef.value?.triggerSend(item.question, { questionOrigin: questionOriginFromSuggestion(item) });
};

const sendMsg = (value: string, modelId: string, mentionedItems: any[], imageFiles: any[] = [], attachmentFiles: any[] = [], options: SendMessageOptions = {}) => {
    createNewSession(value, modelId, mentionedItems, imageFiles, attachmentFiles, options);
}

async function createNewSession(value: string, modelId: string, mentionedItems: any[] = [], imageFiles: any[] = [], attachmentFiles: any[] = [], options: SendMessageOptions = {}) {
    const selectedKbs = settingsStore.settings.selectedKnowledgeBases || [];
    const selectedFiles = settingsStore.settings.selectedFiles || [];

    // 构建 session 数据，包含 Agent 配置
    const sessionData: any = {};

    // 添加 Agent 配置（知识库信息在 agent_config 中）
    sessionData.agent_config = {
        enabled: true,
        max_iterations: settingsStore.agentConfig.maxIterations,
        temperature: settingsStore.agentConfig.temperature,
        knowledge_bases: selectedKbs,  // 所有选中的知识库
        knowledge_ids: selectedFiles,  // 所有选中的普通知识/文件
        allowed_tools: settingsStore.agentConfig.allowedTools
    };

    try {
        const res = await createSessions(withOptionalProjectDir(sessionData, selectedProjectDir.value));
        if (res.data && res.data.id) {
            await navigateToSession(res.data.id, value, modelId, mentionedItems, imageFiles, attachmentFiles, options);
        } else {
            console.error('[createChat] Failed to create session');
            MessagePlugin.error(t('createChat.messages.createFailed'));
        }
    } catch (error) {
        console.error('[createChat] Create session error:', error);
        MessagePlugin.error(t('createChat.messages.createError'));
    }
}

const navigateToSession = async (sessionId: string, value: string, modelId: string, mentionedItems: any[], imageFiles: any[] = [], attachmentFiles: any[] = [], options: SendMessageOptions = {}) => {
    const now = new Date().toISOString();
    let obj = {
        title: t('createChat.newSessionTitle'),
        path: `chat/${sessionId}`,
        id: sessionId,
        isMore: false,
        isNoTitle: true,
        created_at: now,
        updated_at: now
    };
    usemenuStore.updataMenuChildren(obj);
    usemenuStore.changeIsFirstSession(true);
    usemenuStore.changeFirstQuery(value, mentionedItems, modelId, imageFiles, attachmentFiles, options.questionOrigin ?? null);
    router.push(`/platform/chat/${sessionId}`);
}

const handleKBEditorSuccess = (kbId: string) => {
    navigateToKnowledgeBaseList(kbId)
}

function clearProjectDir() {
    selectedProjectDir.value = '';
}

async function openProjectDir() {
    if (pickingProjectDir.value) return;
    pickingProjectDir.value = true;
    try {
        const picked = await pickHostProjectDir();
        if (picked) selectedProjectDir.value = picked;
    } catch (e: any) {
        MessagePlugin.error(e?.message || t('createChat.pickFailed'));
    } finally {
        pickingProjectDir.value = false;
    }
}

</script>
<style lang="less" scoped>
.dialogue-wrap {
    flex: 1;
    display: flex;
    justify-content: center;
    align-items: center;
    position: relative;
    isolation: isolate;
    padding: 40px 32px;
    overflow-y: auto;
    &::before {
        content: '';
        position: absolute;
        z-index: -1;
        inset: 0;
        background: var(--sd-ocean-poster) center / cover;
        opacity: .11;
        mask-image: linear-gradient(#000, transparent 85%);
        pointer-events: none;
    }
}

.dialogue-answers {
    display: flex;
    flex-flow: column;
    align-items: center;
    width: 100%;
    max-width: 800px;
    gap: 20px;
    margin-block: auto;

    :deep(.answers-input) {
        position: static;
        transform: translateX(0);
    }
}

.chat-ocean-emblem {
    display: grid;
    place-items: center;
    width: 88px;
    height: 88px;
    border: 1px solid var(--td-component-border);
    border-radius: 50%;
    background: var(--sd-card-surface);
    box-shadow: 0 0 0 12px color-mix(in srgb, var(--td-brand-color) 4%, transparent);
    margin-bottom: 6px;
    :deep(.brand-mark__icon) { width: 56px; height: 56px; }
    :deep(.brand-mark__depth) { color: var(--td-brand-color); opacity: .5; }
}
.chat-ocean-label { color: var(--td-text-color-secondary); font-size: 10px; letter-spacing: .22em; text-transform: uppercase; }
.chat-ocean-description { margin: -8px 0 8px; max-width: 560px; color: var(--td-text-color-secondary); font-size: 13px; line-height: 1.8; text-align: center; }
.chat-workspace-links {
    display: flex;
    flex-wrap: wrap;
    justify-content: center;
    gap: 10px;
    a { display: flex; align-items: center; gap: 10px; padding: 12px 16px; border: 1px solid var(--td-component-stroke); border-radius: 8px; text-decoration: none; color: var(--td-text-color-secondary); font-size: 12px; background: var(--sd-control-surface); transition: border-color .2s, transform .2s; }
    a:hover { color: var(--td-brand-color); border-color: var(--td-component-border); transform: translateY(-2px); }
    span { margin-left: 12px; color: var(--td-brand-color); }
}
.dialogue-answers :deep(.rich-input-container) { background: var(--sd-control-surface); border-radius: 18px; border-color: var(--td-component-border); box-shadow: var(--sd-shadow); }
@media (max-width: 720px) {
    .dialogue-wrap { padding: 36px 16px; }
    .chat-ocean-emblem { width: 64px; height: 64px; }
    .chat-ocean-emblem :deep(.brand-mark__icon) { width: 42px; height: 42px; }
    .chat-workspace-links { gap: 6px; }
    .chat-workspace-links a { padding: 10px; gap: 6px; }
    .chat-workspace-links span { display: none; }
}
@media (prefers-reduced-motion: reduce) { .chat-workspace-links a { transition: none; transform: none; } }

.create-chat-composer {
    display: flex;
    flex-direction: column;
    align-items: stretch;
    gap: 8px;
    width: 100%;
}

.project-dir-bar {
    display: flex;
    align-items: center;
    gap: 4px;
    width: 100%;
    max-width: 960px;
    padding: 0;
    box-sizing: border-box;
}

.project-dir-bar__btn {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    max-width: 100%;
    height: 28px;
    padding: 0 10px;
    border: 0.5px solid var(--td-component-border);
    border-radius: var(--app-radius-md);
    background: var(--td-bg-color-container);
    color: var(--td-text-color-secondary);
    font-size: var(--app-text-sm);
    cursor: pointer;
}

.project-dir-bar__btn.is-bound {
    color: var(--td-text-color-primary);
}

.project-dir-bar__btn.is-picking,
.project-dir-bar__btn:disabled {
    cursor: default;
    opacity: 0.75;
}

.project-dir-bar__name {
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
}

.project-dir-bar__clear {
    flex: 0 0 auto;
    border: none;
    background: transparent;
    padding: 0 4px;
    color: var(--td-text-color-placeholder);
    font-size: var(--app-text-base);
    line-height: 1;
    cursor: pointer;
}

.dialogue-title {
    display: flex;
    color: var(--td-text-color-primary);
    font-family: var(--app-font-family);
    font-size: clamp(24px, 2.8vw, 36px);
    font-weight: 500;
    letter-spacing: .03em;
    text-align: center;
    align-items: center;
    margin-bottom: 0;

    .icon {
        display: flex;
        width: 32px;
        height: 32px;
        justify-content: center;
        align-items: center;
        border-radius: var(--app-radius-sm);
        background: var(--td-bg-color-container);
        box-shadow: var(--td-shadow-1);
        margin-right: 12px;

        .logo_img {
            height: 24px;
            width: 24px;
        }
    }
}

@import '../../components/css/suggested-questions.less';

@keyframes skeletonFadeIn {
    from {
        opacity: 0;
    }

    to {
        opacity: 1;
    }
}

.suggested-questions-container {
    max-width: 960px;
    margin: 0;
    padding: 0 16px;
    transition: height 0.35s @suggested-ease;
}

.suggested-questions-inner {
    animation: skeletonFadeIn 0.3s ease-out;
}

.sq-slide-fade-enter-active {
    transition: opacity 0.35s @suggested-ease, transform 0.35s @suggested-ease;
}

.sq-slide-fade-leave-active {
    transition: opacity var(--app-motion-fast) cubic-bezier(0.4, 0, 1, 1),
        transform 0.15s cubic-bezier(0.4, 0, 1, 1);
}

.sq-slide-fade-enter-from {
    opacity: 0;
    transform: translateY(10px);
}

.sq-slide-fade-leave-to {
    opacity: 0;
    transform: translateY(-4px);
}

.suggested-question-card {
    opacity: 0;
    transform: translateY(8px) scale(0.97);
    transition:
        opacity 0.35s @suggested-ease,
        transform 0.35s @suggested-ease,
        background 0.2s @suggested-ease,
        border-color 0.25s @suggested-ease,
        box-shadow 0.25s @suggested-ease;

    &.sq-card-skeleton {
        opacity: 1;
        transform: none;
    }

    &.sq-card-visible {
        opacity: 1;
        transform: translateY(0) scale(1);
    }

    &:not(.sq-card-skeleton):active {
        transform: scale(0.98);
    }

    &.sq-card-visible:active {
        transform: scale(0.98);
    }
}
</style>
<style lang="less">
.del-menu-popup {
    z-index: 99 !important;

    .t-popup__content {
        width: 100px;
        height: 40px;
        line-height: 30px;
        padding-left: 14px;
        cursor: pointer;
        margin-top: 4px !important;

    }
}
</style>
