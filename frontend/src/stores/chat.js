import { defineStore } from 'pinia'
import { ref } from 'vue'
import { useAuthStore } from './auth'

export const useChatStore = defineStore('chat', () => {
  const messages = ref([])
  const pinnedMessages = ref([])
  const onlineUsers = ref([])
  const onlineCount = ref(0)
  const typingUsers = ref([])
  const ws = ref(null)
  const wsConnected = ref(false)
  const loading = ref(false)
  const hasMore = ref(true)
  const avatars = ref([])
  const customEmojis = ref([])
  
  const authStore = useAuthStore()

  async function fetchAvatars() {
    try {
      const res = await fetch('/api/avatars')
      if (res.ok) {
        const data = await res.json()
        avatars.value = data.avatars || []
      }
    } catch (e) {
      console.error('Failed to fetch avatars:', e)
    }
  }

  async function fetchCustomEmojis() {
    try {
      const res = await fetch('/api/emojis')
      if (res.ok) {
        const data = await res.json()
        customEmojis.value = data.emojis || []
      }
    } catch (e) {
      console.error('Failed to fetch custom emojis:', e)
    }
  }

  async function uploadCustomEmoji(file, name) {
    const formData = new FormData()
    formData.append('file', file)
    formData.append('name', name)

    const res = await fetch('/api/emojis', {
      method: 'POST',
      headers: { 'Authorization': `Bearer ${authStore.token}` },
      body: formData
    })

    if (!res.ok) {
      throw new Error('Failed to upload emoji')
    }
    return await res.json()
  }

  async function deleteCustomEmoji(emojiId) {
    const res = await fetch(`/api/emojis/${emojiId}`, {
      method: 'DELETE',
      headers: { 'Authorization': `Bearer ${authStore.token}` }
    })

    if (!res.ok) {
      throw new Error('Failed to delete emoji')
    }
  }

  //helper to attach custom emoji URLs to reactions if missing
  function hydrateMessageReactions(msg) {
    if (!msg.reactions) return msg
    
    msg.reactions = msg.reactions.map(r => {
      if (!r.custom_emoji_url) {
        const custom = customEmojis.value.find(e => e.name === r.emoji)
        if (custom) {
          r.custom_emoji_url = custom.url
        }
      }
      return r
    })
    return msg
  }
  
  async function fetchMessages(before = null) {
    loading.value = true
    try {
      let url = '/api/messages?limit=50'
      if (before) url += `&before=${before}`
      
      const res = await fetch(url)
      if (res.ok) {
        const data = await res.json()
        
        //hydrate messages with custom emoji URLs
        const hydratedMessages = (data.messages || []).map(hydrateMessageReactions)
        
        if (before) {
          messages.value = [...hydratedMessages, ...messages.value]
        } else {
          //initial load
          messages.value = hydratedMessages
          pinnedMessages.value = (data.pinned_messages || []).map(hydrateMessageReactions)
        }
        
        hasMore.value = data.has_more
      }
    } finally {
      loading.value = false
    }
  }
  
  async function sendMessage(content, files = [], replyToId = null, gif = null) {
    const formData = new FormData()
    if (content) formData.append('content', content)
    if (replyToId) formData.append('reply_to_id', replyToId)
    
    //handle GIF
    if (gif) {
      formData.append('gif_url', gif.url)
      formData.append('gif_id', gif.id)
      if (gif.preview_url) formData.append('gif_preview_url', gif.preview_url)
    }
    
    files.forEach(file => formData.append('files', file))
    
    const res = await fetch('/api/messages', {
      method: 'POST',
      headers: { 'Authorization': `Bearer ${authStore.token}` },
      body: formData
    })
    
    if (!res.ok) {
      const error = await res.json().catch(() => ({ detail: 'Failed to send message' }))
      throw new Error(error.detail || 'Failed to send message')
    }
    
    const data = await res.json()
    
    //handle command responses (like /clear)
    if (data.success !== undefined && data.command) {
      //this is a command response => not a message
      return data
    }
    
    return data
  }
  
  async function deleteMessage(messageId) {
    const res = await fetch(`/api/messages/${messageId}`, {
      method: 'DELETE',
      headers: { 'Authorization': `Bearer ${authStore.token}` }
    })
    
    if (!res.ok) {
      throw new Error('Failed to delete message')
    }
  }

  async function pinMessage(messageId) {
    const res = await fetch(`/api/messages/${messageId}/pin`, {
      method: 'POST',
      headers: { 'Authorization': `Bearer ${authStore.token}` }
    })
    
    if (!res.ok) {
      throw new Error('Failed to pin message')
    }
    
    return await res.json()
  }
  
  async function toggleReaction(messageId, emoji, customEmojiId = null) {
    //if no customEmojiId provided => try to find it in store
    if (!customEmojiId) {
      const custom = customEmojis.value.find(e => e.name === emoji)
      if (custom) {
        customEmojiId = custom.id
      }
    }

    const res = await fetch(`/api/messages/${messageId}/reactions`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        'Authorization': `Bearer ${authStore.token}`
      },
      body: JSON.stringify({ emoji, custom_emoji_id: customEmojiId })
    })
    
    if (!res.ok) {
      throw new Error('Failed to toggle reaction')
    }
  }

  async function searchGifs(query, pos = null) {
    const params = new URLSearchParams({ q: query, limit: '20' })
    if (pos) params.append('pos', pos)
    
    const res = await fetch(`/api/gifs/search?${params}`, {
      headers: { 'Authorization': `Bearer ${authStore.token}` }
    })
    
    if (!res.ok) {
      throw new Error('Failed to search GIFs')
    }
    
    return await res.json()
  }

  async function getTrendingGifs(pos = null) {
    const params = new URLSearchParams({ limit: '20' })
    if (pos) params.append('pos', pos)
    
    const res = await fetch(`/api/gifs/trending?${params}`, {
      headers: { 'Authorization': `Bearer ${authStore.token}` }
    })
    
    if (!res.ok) {
      throw new Error('Failed to get trending GIFs')
    }
    
    return await res.json()
  }
  
  function connectWebSocket() {
    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
    const wsUrl = `${protocol}//${window.location.host}/ws${authStore.token ? `?token=${authStore.token}` : ''}`
    
    ws.value = new WebSocket(wsUrl)
    
    ws.value.onopen = () => {
      wsConnected.value = true
      console.log('WebSocket connected')
    }
    
    ws.value.onclose = () => {
      wsConnected.value = false
      console.log('WebSocket disconnected, reconnecting...')
      setTimeout(connectWebSocket, 3000)
    }
    
    ws.value.onerror = (error) => {
      console.error('WebSocket error:', error)
    }
    
    ws.value.onmessage = (event) => {
      const data = JSON.parse(event.data)
      handleWSMessage(data)
    }
  }
  
  function handleWSMessage(data) {
    switch (data.type) {
      case 'connected':
        onlineUsers.value = data.data.online_users
        onlineCount.value = data.data.online_count
        
        if (data.data.token) {
          authStore.handleWsToken(data.data.token)
        }
        
        if (!authStore.user) {
          authStore.user = {
            id: data.data.user_id,
            username: data.data.username,
            avatar: data.data.avatar,
            is_admin: data.data.is_admin,
            can_post: data.data.can_post
          }
        } else {
          //update can_post status
          authStore.user.can_post = data.data.can_post
        }
        break
        
      case 'new_message':
        //hydrate before adding
        const hydratedMsg = hydrateMessageReactions(data.data)
        messages.value.push(hydratedMsg)
        if (data.data.is_pinned) {
          pinnedMessages.value.unshift(hydratedMsg)
        }
        break
        
      case 'message_deleted':
        messages.value = messages.value.filter(m => m.id !== data.data.message_id)
        pinnedMessages.value = pinnedMessages.value.filter(m => m.id !== data.data.message_id)
        break
      
      case 'chat_cleared':
        messages.value = []
        pinnedMessages.value = []
        break

      case 'message_pinned_update': {
        const { message_id, is_pinned } = data.data
        //update main list
        const msg = messages.value.find(m => m.id === message_id)
        if (msg) msg.is_pinned = is_pinned
        
        //update pinned list
        if (is_pinned) {
          if (!pinnedMessages.value.find(m => m.id === message_id) && msg) {
            pinnedMessages.value.unshift(msg)
          }
        } else {
          pinnedMessages.value = pinnedMessages.value.filter(m => m.id !== message_id)
        }
        break
      }

      case 'reaction_added':
        updateMessageReaction(data.data.message_id, data.data.emoji, data.data.username, data.data.avatar, 'add', data.data.custom_emoji_url)
        break
        
      case 'reaction_removed':
        updateMessageReaction(data.data.message_id, data.data.emoji, data.data.username, data.data.avatar, 'remove', data.data.custom_emoji_url)
        break
        
      case 'user_join':
        onlineCount.value = data.data.online_count
        //check if user already in list (shouldnt be => but just in case)
        if (!onlineUsers.value.find(u => u.user_id === data.data.user_id)) {
          onlineUsers.value.push({
            user_id: data.data.user_id,
            username: data.data.username,
            avatar: data.data.avatar,
            is_admin: data.data.is_admin,
          })
        }
        break
        
      case 'user_leave':
        onlineCount.value = data.data.online_count
        onlineUsers.value = onlineUsers.value.filter(u => u.user_id !== data.data.user_id)
        break
        
      case 'typing':
        if (!typingUsers.value.includes(data.data.username)) {
          typingUsers.value.push(data.data.username)
          setTimeout(() => {
            typingUsers.value = typingUsers.value.filter(u => u !== data.data.username)
          }, 3000)
        }
        break
        
      case 'user_avatar_changed': {
        const user = onlineUsers.value.find(u => u.user_id === data.data.user_id)
        if (user) user.avatar = data.data.avatar
        break
      }

      case 'custom_emoji_added':
        customEmojis.value.push(data.data)
        break

      case 'custom_emoji_removed':
        customEmojis.value = customEmojis.value.filter(e => e.id !== data.data.id)
        break
    }
  }
  
  function updateMessageReaction(messageId, emoji, username, avatar, action, customEmojiUrl = null) {
    if (!customEmojiUrl) {
      const custom = customEmojis.value.find(e => e.name === emoji)
      if (custom) customEmojiUrl = custom.url
    }

    //update both lists
    const targets = [
      messages.value.find(m => m.id === messageId),
      pinnedMessages.value.find(m => m.id === messageId)
    ]
    
    targets.forEach(message => {
      if (!message) return
      
      let reaction = message.reactions.find(r => r.emoji === emoji)
      
      if (action === 'add') {
        if (reaction) {
          reaction.count++
          if (!reaction.users.includes(username)) {
            reaction.users.push(username)
            if (!reaction.user_avatars) reaction.user_avatars = []
            reaction.user_avatars.push(avatar)
          }
          //ensure URL is set if found late
          if (!reaction.custom_emoji_url && customEmojiUrl) {
            reaction.custom_emoji_url = customEmojiUrl
          }
        } else {
          message.reactions.push({ 
            emoji, 
            count: 1, 
            users: [username],
            user_avatars: [avatar],
            custom_emoji_url: customEmojiUrl
          })
        }
      } else if (action === 'remove' && reaction) {
        reaction.count--
        const idx = reaction.users.indexOf(username)
        if (idx !== -1) {
          reaction.users.splice(idx, 1)
          if (reaction.user_avatars) reaction.user_avatars.splice(idx, 1)
        }
        if (reaction.count <= 0) {
          message.reactions = message.reactions.filter(r => r.emoji !== emoji)
        }
      }
    })
  }
  
  function sendTyping() {
    if (ws.value && wsConnected.value) {
      ws.value.send(JSON.stringify({ type: 'typing' }))
    }
  }
  
  function updateAvatar(avatar) {
    if (ws.value && wsConnected.value) {
      ws.value.send(JSON.stringify({ type: 'update_avatar', avatar }))
    }
  }
  
  function disconnect() {
    if (ws.value) {
      ws.value.close()
      ws.value = null
    }
  }
  
  function getAvatarUrl(avatarId) {
    if (!avatarId) return '/avatars/default.png'
    //if already full URL => return it as-is
    if (avatarId.startsWith('http://') || avatarId.startsWith('https://')) {
      return avatarId
    }
    //check if matching avatar in the avatars list
    const avatarObj = avatars.value.find(a => a.id === avatarId)
    if (avatarObj) {
      //avatar URLs from API should already be relative paths
      return avatarObj.url.startsWith('/') ? avatarObj.url : `/${avatarObj.url}`
    }
    //fallback to default pattern
    return `/avatars/${avatarId}.png`
  }

  function getCustomEmoji(name) {
    return customEmojis.value.find(e => e.name === name)
  }
  
  return {
    messages,
    pinnedMessages,
    onlineUsers,
    onlineCount,
    typingUsers,
    wsConnected,
    loading,
    hasMore,
    avatars,
    customEmojis,
    fetchAvatars,
    fetchCustomEmojis,
    uploadCustomEmoji,
    deleteCustomEmoji,
    fetchMessages,
    sendMessage,
    deleteMessage,
    pinMessage,
    toggleReaction,
    searchGifs,
    getTrendingGifs,
    connectWebSocket,
    sendTyping,
    updateAvatar,
    disconnect,
    getAvatarUrl,
    getCustomEmoji
  }
})