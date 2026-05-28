<script setup>
import { ref } from "vue";
import { useRouter } from "vue-router";
import { useAuthStore } from "../stores/auth";
import Icon from "../components/Icon.vue";

const router = useRouter();
const auth = useAuthStore();

const mode = ref("login"); // 'login' | 'change'
const email = ref("");
const password = ref("");
const newPassword = ref("");
const confirmPassword = ref("");
const error = ref("");
const loading = ref(false);

async function submitLogin() {
  error.value = "";
  loading.value = true;
  try {
    await auth.login(email.value, password.value);
    if (auth.mustChangePassword) {
      mode.value = "change";
    } else {
      router.push("/");
    }
  } catch (e) {
    error.value = e.message;
  } finally {
    loading.value = false;
  }
}

async function submitChange() {
  error.value = "";
  if (newPassword.value.length < 8) {
    error.value = "Password must be at least 8 characters";
    return;
  }
  if (newPassword.value !== confirmPassword.value) {
    error.value = "Passwords do not match";
    return;
  }
  loading.value = true;
  try {
    await auth.changePassword(newPassword.value);
    router.push("/");
  } catch (e) {
    error.value = e.message;
  } finally {
    loading.value = false;
  }
}

function goBack() {
  router.push("/");
}
</script>

<template>
  <div class="login-screen">
    <div class="login-card">
      <template v-if="mode === 'change'">
        <div class="login-head">
          <div class="login-badge">
            <Icon name="lock" :size="34" />
          </div>
          <h1 class="login-title">Change password</h1>
          <p class="login-sub">Set a new password to continue</p>
        </div>

        <form class="login-form" @submit.prevent="submitChange">
          <div class="login-fields">
            <div class="field">
              <label class="field-label">New password</label>
              <input
                v-model="newPassword"
                type="password"
                class="field-input"
                placeholder="At least 8 characters"
                autocomplete="new-password"
                required
              />
            </div>
            <div class="field">
              <label class="field-label">Confirm password</label>
              <input
                v-model="confirmPassword"
                type="password"
                class="field-input"
                placeholder="Repeat password"
                autocomplete="new-password"
                required
              />
            </div>
          </div>

          <div v-if="error" class="login-error">{{ error }}</div>

          <button type="submit" class="login-submit" :disabled="loading">
            {{ loading ? "Updating..." : "Update password" }}
          </button>
        </form>
      </template>

      <template v-else>
        <div class="login-head">
          <div class="login-badge shape-soft-burst">
            <Icon name="user" :size="34" />
          </div>
          <h1 class="login-title">Admin login</h1>
          <p class="login-sub">Sign in to manage your blog</p>
        </div>

        <form class="login-form" @submit.prevent="submitLogin">
          <div class="login-fields">
            <div class="field">
              <label class="field-label">Email</label>
              <input
                v-model="email"
                type="email"
                class="field-input"
                placeholder="you@domain.com"
                autocomplete="email"
                required
              />
            </div>
            <div class="field">
              <label class="field-label">Password</label>
              <input
                v-model="password"
                type="password"
                class="field-input"
                placeholder="Your password"
                autocomplete="current-password"
                required
              />
            </div>
          </div>

          <div v-if="error" class="login-error">{{ error }}</div>

          <button type="submit" class="login-submit" :disabled="loading">
            {{ loading ? "Signing in..." : "Sign in" }}
          </button>
        </form>

        <button class="login-back" @click="goBack">
          <Icon name="back" :size="18" />
          Back to blog
        </button>
      </template>
    </div>
  </div>
</template>
