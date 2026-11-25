<template>
  <main class="workspace-onboarding">
    <OceanBackdrop />
    <div class="workspace-brand"><BrandMark variant="ocean" /></div>
    <section class="workspace-card">
      <div class="workspace-mark" aria-hidden="true"><t-icon name="system-sum" size="24px" /></div>
      <h1 v-if="authStore.canCreateTenant">{{ $t('auth.workspaceOnboarding.title') }}</h1>
      <h1 v-else>{{ $t('auth.workspaceOnboarding.inviteOnlyTitle') }}</h1>
      <p v-if="authStore.canCreateTenant" class="workspace-description">
        {{ $t('auth.workspaceOnboarding.description') }}
      </p>
      <p v-else class="workspace-description">
        {{ $t('auth.workspaceOnboarding.inviteOnlyDescription') }}
      </p>

      <div v-if="policyLoading" class="policy-loading">
        <t-loading size="small" />
        <span>{{ $t('auth.workspaceOnboarding.loadingPolicy') }}</span>
      </div>
      <div v-else-if="policyLoadFailed" class="policy-error" role="alert">
        <t-icon name="error-circle" size="20px" aria-hidden="true" />
        <span>{{ $t('auth.workspaceOnboarding.policyLoadFailed') }}</span>
        <t-button size="small" variant="text" @click="loadPolicy">
          {{ $t('auth.workspaceOnboarding.retry') }}
        </t-button>
      </div>

      <template v-else>
        <div v-if="!authStore.canCreateTenant" class="invite-only-notice">
          <t-icon name="lock-on" size="20px" aria-hidden="true" />
          <span>{{ $t('auth.workspaceOnboarding.inviteOnlyNotice') }}</span>
        </div>

        <div class="workspace-actions" :class="{ 'workspace-actions--single': !authStore.canCreateTenant }">
          <t-button v-if="authStore.canCreateTenant" theme="primary" size="large" @click="createVisible = true">
            <template #icon><t-icon name="add" /></template>
            {{ $t('auth.workspaceOnboarding.create') }}
          </t-button>
          <t-button
            :theme="authStore.canCreateTenant ? 'default' : 'primary'"
            :variant="authStore.canCreateTenant ? 'outline' : 'base'"
            size="large"
            @click="invitationsVisible = true"
          >
            <template #icon><t-icon name="mail" /></template>
            {{ $t('auth.workspaceOnboarding.invitations') }}
            <template v-if="authStore.pendingInvitationCount > 0">
              ({{ authStore.pendingInvitationCount }})
            </template>
          </t-button>
        </div>
      </template>

      <p v-if="!policyLoading && !policyLoadFailed && authStore.canCreateTenant" class="workspace-help">
        {{ $t('auth.workspaceOnboarding.help') }}
      </p>
      <p v-else-if="!policyLoading && !policyLoadFailed" class="workspace-help">
        {{ $t('auth.workspaceOnboarding.inviteOnlyHelp') }}
      </p>
      <button class="logout-link" type="button" @click="handleLogout">
        {{ $t('auth.logout') }}
      </button>
    </section>

    <CreateTenantDialog v-model:visible="createVisible" @created="onTenantCreated" />
    <MyInvitationsDialog v-model:visible="invitationsVisible" />
  </main>
</template>

<script setup lang="ts">
import { onMounted, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import CreateTenantDialog from '@/components/CreateTenantDialog.vue'
import MyInvitationsDialog from '@/components/MyInvitationsDialog.vue'
import BrandMark from '@/components/BrandMark.vue'
import OceanBackdrop from '@/components/OceanBackdrop.vue'
import { logout as logoutApi } from '@/api/auth'
import type { TenantInfo } from '@/api/tenant'
import { useAuthStore } from '@/stores/auth'

const router = useRouter()
const authStore = useAuthStore()
const createVisible = ref(false)
const invitationsVisible = ref(false)
const policyLoading = ref(true)
const policyLoadFailed = ref(false)

async function loadPolicy() {
  policyLoading.value = true
  policyLoadFailed.value = false
  try {
    const refreshed = await authStore.refreshFromAuthMe()
    if (!refreshed) {
      policyLoadFailed.value = true
      return
    }
    await authStore.fetchPendingInvitationCount()
  } finally {
    policyLoading.value = false
  }
}

onMounted(async () => {
  await loadPolicy()
})

watch(
  () => authStore.hasValidTenant,
  (ready) => {
    if (ready) router.replace('/platform/knowledge-bases')
  },
)

async function onTenantCreated(tenant: TenantInfo) {
  await authStore.refreshFromAuthMe()
  authStore.setSelectedTenant(tenant.id, tenant.name)
  await router.replace('/platform/knowledge-bases')
}

async function handleLogout() {
  await logoutApi()
  authStore.logout()
  await router.replace('/login')
}
</script>

<style scoped lang="less">
.workspace-onboarding {
  --td-text-color-primary: #f0faf7;
  --td-text-color-secondary: #c6dedc;
  --td-brand-color: #b5efd9;
  --td-brand-color-hover: #d3f8e9;
  --td-brand-color-active: #98dec7;
  --td-brand-color-light: rgba(181, 239, 217, .12);
  --td-text-color-anti: #102e31;
  --td-bg-color-container: rgba(3, 24, 33, .48);
  --td-bg-color-container-hover: rgba(181, 239, 217, .12);
  --td-bg-color-secondarycontainer: rgba(181, 239, 217, .1);
  --td-component-stroke: rgba(211, 239, 235, .28);
  min-height: 100dvh;
  display: grid;
  grid-template-columns: minmax(0, 1fr) minmax(0, 520px);
  align-items: center;
  padding: 96px max(5vw, 24px) 48px;
  box-sizing: border-box;
  isolation: isolate;
  background: #0b3e47;
}

.workspace-brand {
  position: absolute;
  top: 28px;
  left: max(5vw, 24px);
  z-index: 1;
}

.workspace-card {
  grid-column: 2;
  position: relative;
  z-index: 1;
  width: 100%;
  box-sizing: border-box;
  padding: 42px;
  border: 1px solid var(--td-component-stroke);
  border-radius: 8px;
  background: var(--td-bg-color-container);
  text-align: center;
}

.workspace-mark {
  width: 48px;
  height: 48px;
  margin: 0 auto 20px;
  display: grid;
  place-items: center;
  border: 1px solid var(--td-component-stroke);
  border-radius: 8px;
  color: var(--td-brand-color);
  background: var(--td-brand-color-light);
}

h1 {
  margin: 0;
  color: var(--td-text-color-primary);
  font-size: 26px;
  line-height: 1.3;
}

.workspace-description,
.workspace-help {
  color: var(--td-text-color-secondary);
  line-height: 1.7;
}

.workspace-description {
  margin: 14px 0 28px;
}

.workspace-actions {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 12px;
}

.workspace-actions--single {
  grid-template-columns: minmax(220px, 1fr);
}

.policy-loading,
.invite-only-notice,
.policy-error {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 10px;
  min-height: 52px;
  margin-bottom: 18px;
  color: var(--td-text-color-secondary);
  font-size: var(--app-text-base);
}

.policy-error {
  flex-wrap: wrap;
  padding: 12px 16px;
  border-radius: var(--app-radius-lg);
  color: var(--td-error-color);
  background: var(--td-error-color-light);
}

.invite-only-notice {
  padding: 12px 16px;
  border: 1px solid var(--td-component-stroke);
  border-radius: var(--app-radius-lg);
  color: var(--td-text-color-primary);
  background: var(--td-bg-color-secondarycontainer);
  line-height: 1.5;
}

.invite-only-notice :deep(.t-icon) {
  flex: 0 0 auto;
  color: var(--td-text-color-secondary);
}

.workspace-help {
  margin: 24px 0 8px;
  font-size: var(--app-text-md);
}

.logout-link {
  border: 0;
  padding: 6px 10px;
  color: var(--td-text-color-secondary);
  background: transparent;
  cursor: pointer;
}

.logout-link:hover {
  color: var(--td-brand-color);
}

@media (max-width: 900px) {
  .workspace-onboarding {
    grid-template-columns: minmax(0, 1fr);
    justify-items: center;
  }

  .workspace-card {
    grid-column: 1;
    max-width: 520px;
  }
}

@media (max-width: 560px) {
  .workspace-card {
    padding: 28px 20px;
  }

  .workspace-actions {
    grid-template-columns: 1fr;
  }
}
</style>
