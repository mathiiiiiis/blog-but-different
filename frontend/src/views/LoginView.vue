<script setup>
import { ref } from 'vue'
import { useRouter } from 'vue-router'
import { useAuthStore } from '../stores/auth'
import Icon from '../components/Icon.vue'

const router = useRouter()
const authStore = useAuthStore()

const email = ref('')
const password = ref('')
const newPassword = ref('')
const confirmPassword = ref('')
const error = ref('')
const loading = ref(false)
const showPasswordChange = ref(false)

async function handleLogin() {
  error.value = ''
  loading.value = true
  
  try {
    await authStore.login(email.value, password.value)
    
    if (authStore.mustChangePassword) {
      showPasswordChange.value = true
    } else {
      router.push('/')
    }
  } catch (e) {
    error.value = e.message
  } finally {
    loading.value = false
  }
}

async function handlePasswordChange() {
  error.value = ''
  
  if (newPassword.value.length < 8) {
    error.value = 'Password must be at least 8 characters'
    return
  }
  
  if (newPassword.value !== confirmPassword.value) {
    error.value = 'Passwords do not match'
    return
  }
  
  loading.value = true
  
  try {
    await authStore.changePassword(newPassword.value)
    router.push('/')
  } catch (e) {
    error.value = e.message
  } finally {
    loading.value = false
  }
}

function goBack() {
  router.push('/')
}
</script>

<template>
  <div class="login-container">
    <div class="login-card">
      <template v-if="showPasswordChange">
        <div class="login-header">
          <div class="login-icon">
            <Icon name="settings" :size="32" />
          </div>
          <h1 class="login-title">Change Password</h1>
          <p class="login-subtitle">Please set a new secure password</p>
        </div>
        
        <form @submit.prevent="handlePasswordChange" class="login-form">
          <input
            v-model="newPassword"
            type="password"
            placeholder="New password"
            class="login-input"
            autocomplete="new-password"
            required
          />
          
          <input
            v-model="confirmPassword"
            type="password"
            placeholder="Confirm password"
            class="login-input"
            autocomplete="new-password"
            required
          />
          
          <div v-if="error" class="login-error">
            {{ error }}
          </div>
          
          <button type="submit" class="login-btn" :disabled="loading">
            {{ loading ? 'Updating...' : 'Update Password' }}
          </button>
        </form>
      </template>

      <template v-else>
        <div class="login-header">
          <div class="login-icon">
            <Icon name="user" :size="32" />
          </div>
          <h1 class="login-title">Admin Login</h1>
          <p class="login-subtitle">Sign in to manage your blog</p>
        </div>
        
        <form @submit.prevent="handleLogin" class="login-form">
          <input
            v-model="email"
            type="email"
            placeholder="Email"
            class="login-input"
            autocomplete="email"
            required
          />
          
          <input
            v-model="password"
            type="password"
            placeholder="Password"
            class="login-input"
            autocomplete="current-password"
            required
          />
          
          <div v-if="error" class="login-error">
            {{ error }}
          </div>
          
          <button type="submit" class="login-btn" :disabled="loading">
            {{ loading ? 'Signing in...' : 'Sign In' }}
          </button>
        </form>
        
        <div class="login-footer">
          <button @click="goBack" class="login-back">
            <Icon name="back" :size="16" />
            Back to blog
          </button>
        </div>
      </template>
    </div>
  </div>
</template>