import { defineStore } from 'pinia'
import { ref, computed } from 'vue'

//cookie helpers
function setCookie(name, value, days = 30) {
  const expires = new Date(Date.now() + days * 864e5).toUTCString()
  document.cookie = `${name}=${encodeURIComponent(value)}; expires=${expires}; path=/; SameSite=Lax`
}

function getCookie(name) {
  const value = `; ${document.cookie}`
  const parts = value.split(`; ${name}=`)
  if (parts.length === 2) return decodeURIComponent(parts.pop().split(';').shift())
  return null
}

function deleteCookie(name) {
  document.cookie = `${name}=; expires=Thu, 01 Jan 1970 00:00:00 GMT; path=/`
}

export const useAuthStore = defineStore('auth', () => {
  const token = ref(getCookie('blog_token') || null)
  const user = ref(null)
  const mustChangePassword = ref(false)
  
  const isLoggedIn = computed(() => !!token.value)
  const isAdmin = computed(() => user.value?.is_admin)
  const canPost = computed(() => user.value?.can_post ?? user.value?.is_admin)
  
  function setToken(newToken) {
    token.value = newToken
    if (newToken) {
      setCookie('blog_token', newToken, 30)
    } else {
      deleteCookie('blog_token')
    }
  }
  
  async function login(email, password) {
    const res = await fetch('/api/auth/login', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ email, password })
    })
    
    if (!res.ok) {
      const error = await res.json().catch(() => ({ detail: 'Login failed' }))
      throw new Error(error.detail || 'Login failed')
    }
    
    const data = await res.json()
    setToken(data.access_token)
    mustChangePassword.value = data.must_change_password
    
    await fetchUser()
    return data
  }
  
  async function changePassword(newPassword) {
    const res = await fetch('/api/auth/change-password', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        'Authorization': `Bearer ${token.value}`
      },
      body: JSON.stringify({ new_password: newPassword })
    })
    
    if (!res.ok) {
      const error = await res.json().catch(() => ({ detail: 'Password change failed' }))
      throw new Error(error.detail || 'Password change failed')
    }
    
    mustChangePassword.value = false
  }
  
  async function createGuestSession(avatar = 'default') {
    const res = await fetch('/api/auth/guest', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ avatar })
    })
    
    if (!res.ok) {
      throw new Error('Failed to create guest session')
    }
    
    const data = await res.json()
    setToken(data.access_token)
    
    await fetchUser()
  }
  
  async function fetchUser() {
    if (!token.value) return
    
    try {
      const res = await fetch('/api/auth/me', {
        headers: { 'Authorization': `Bearer ${token.value}` }
      })
      
      if (res.ok) {
        user.value = await res.json()
      } else {
        logout()
      }
    } catch {
      logout()
    }
  }
  
  async function updateAvatar(avatar) {
    const res = await fetch(`/api/auth/avatar?avatar=${encodeURIComponent(avatar)}`, {
      method: 'PATCH',
      headers: { 'Authorization': `Bearer ${token.value}` }
    })
    
    if (res.ok && user.value) {
      user.value.avatar = avatar
    }
  }

  // --- Admin Actions ---

  function logout() {
    setToken(null)
    user.value = null
  }
  
  //handle token from WebSocket (for new guests)
  function handleWsToken(newToken) {
    if (newToken && !token.value) {
      setToken(newToken)
    }
  }
  
  //initialize
  if (token.value) {
    fetchUser()
  }
  
  return {
    token,
    user,
    mustChangePassword,
    isLoggedIn,
    isAdmin,
    canPost,
    login,
    changePassword,
    createGuestSession,
    fetchUser,
    updateAvatar,
    logout,
    handleWsToken,
  }
})