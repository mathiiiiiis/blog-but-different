<script setup>
import { ref, computed, onMounted, onUnmounted, nextTick, watch } from 'vue'
import { useRouter } from 'vue-router'
import { useAuthStore } from '../stores/auth'
import { useChatStore } from '../stores/chat'
import MessageBubble from '../components/MessageBubble.vue'
import AvatarPicker from '../components/AvatarPicker.vue'
import ReactionSheet from '../components/ReactionSheet.vue'
import EmojiStickerPicker from '../components/EmojiStickerGifPicker.vue'
import AdminPanel from '../components/AdminPanel.vue'
import Icon from '../components/Icon.vue'

const router = useRouter()
const authStore = useAuthStore()
const chatStore = useChatStore()

const messagesContainer = ref(null)
const textareaRef = ref(null)
const messageInput = ref('')
const fileInput = ref(null)
const selectedFiles = ref([])
const showAvatarPicker = ref(false)
const showEmojiPicker = ref(false)
const showAdminPanel = ref(false)
const selectedMessageForDetails = ref(null)
const replyToMessage = ref(null)
const toast = ref('')

const initialPickerTab = ref('emoji')

const lightboxSources = ref([])
const lightboxIndex = ref(0)
const transitionName = ref('fade')

const isAtBottom = ref(false)
const unreadCount = ref(0)
const sendBtnState = ref('idle')

const currentTheme = ref(localStorage.getItem('blog-theme') || 'imessage')

const isAdmin = computed(() => authStore.isAdmin)
const canPost = computed(() => authStore.canPost)
const isMd = computed(() => currentTheme.value === 'material')

const avatarUrl = computed(() => {
  return chatStore.getAvatarUrl(authStore.user?.avatar)
})

const hasContent = computed(() => messageInput.value.trim() || selectedFiles.value.length > 0)

const activeLightboxSource = computed(() => {
  if (!lightboxSources.value.length) return null
  return lightboxSources.value[lightboxIndex.value]
})

const lightboxImage = computed(() => {
  const source = activeLightboxSource.value
  if (source) {
      if (typeof source === 'string') return source
      return source.url
  }
  return null
})

const lightboxType = computed(() => {
    const source = activeLightboxSource.value
    if (!source) return null
    
    if (source.type) {
        if (source.type.startsWith('video')) return 'video'
        if (source.type.startsWith('audio')) return 'audio'
        return 'image'
    }

    const url = typeof source === 'string' ? source : source.url
    if (url.match(/\.(mp4|webm|ogg|mov)$/i)) return 'video'
    if (url.match(/\.(mp3|wav|ogg)$/i)) return 'audio'
    
    return 'image'
})

function groupMessages(msgs) {
  if (!msgs.length) return []
  return msgs.map((msg, index) => {
    const prevMsg = msgs[index - 1]
    const nextMsg = msgs[index + 1]
    const isSameAuthorAsPrev = prevMsg && prevMsg.author_id === msg.author_id
    const isSameAuthorAsNext = nextMsg && nextMsg.author_id === msg.author_id
    const dateMsg = new Date(msg.created_at)
    const datePrev = prevMsg ? new Date(prevMsg.created_at) : null
    const dateNext = nextMsg ? new Date(nextMsg.created_at) : null
    let timeDiffPrev = Infinity
    if (datePrev && !isNaN(dateMsg) && !isNaN(datePrev)) {
        timeDiffPrev = (dateMsg - datePrev) / 1000 / 60
    }
    let timeDiffNext = Infinity
    if (dateNext && !isNaN(dateMsg) && !isNaN(dateNext)) {
        timeDiffNext = (dateNext - dateMsg) / 1000 / 60
    }
    const GROUP_THRESHOLD = 5 
    const TIME_THRESHOLD = 15
    const isFirstInGroup = !isSameAuthorAsPrev || Math.abs(timeDiffPrev) > GROUP_THRESHOLD
    const isLastInGroup = !isSameAuthorAsNext || Math.abs(timeDiffNext) > GROUP_THRESHOLD
    const showTime = !prevMsg || Math.abs(timeDiffPrev) > TIME_THRESHOLD
    return { ...msg, _isFirstInGroup: isFirstInGroup, _isLastInGroup: isLastInGroup, _showTime: showTime }
  })
}

const groupedMessages = computed(() => groupMessages(chatStore.messages))
const pinnedMessages = computed(() => groupMessages(chatStore.pinnedMessages))

let typingTimeout = null

function checkIsAtBottom() {
  if (!messagesContainer.value) return true
  const { scrollTop, scrollHeight, clientHeight } = messagesContainer.value
  return scrollHeight - scrollTop - clientHeight < 150
}

function scrollToBottom(smooth = true, force = false) {
  nextTick(() => {
    if (messagesContainer.value && (force || isAtBottom.value)) {
      messagesContainer.value.scrollTo({
        top: messagesContainer.value.scrollHeight,
        behavior: smooth ? 'smooth' : 'auto'
      })
      unreadCount.value = 0
    }
  })
}

function jumpToBottom() {
  if (messagesContainer.value) {
    messagesContainer.value.scrollTo({
      top: messagesContainer.value.scrollHeight,
      behavior: 'smooth'
    })
    unreadCount.value = 0
    isAtBottom.value = true
  }
}

function handleScroll() {
  if (messagesContainer.value) {
    const wasAtBottom = isAtBottom.value
    isAtBottom.value = checkIsAtBottom()
    if (!wasAtBottom && isAtBottom.value) { unreadCount.value = 0 }
    const { scrollTop } = messagesContainer.value
    if (scrollTop < 300 && chatStore.hasMore && !chatStore.loading) {
      const firstMessage = chatStore.messages[0]
      if (firstMessage) { chatStore.fetchMessages(firstMessage.id) }
    }
  }
}

function handleTyping() {
  clearTimeout(typingTimeout)
  typingTimeout = setTimeout(() => { chatStore.sendTyping() }, 500)
}

async function sendMessage() {
  const content = messageInput.value.trim()
  const files = selectedFiles.value
  
  if (!content && files.length === 0) return
  if (!canPost.value) return
  
  const sentContent = content
  const sentFiles = [...files]
  const sentReplyTo = replyToMessage.value?.id
  
  messageInput.value = ''
  selectedFiles.value = []
  replyToMessage.value = null
  sendBtnState.value = 'sending'
  
  nextTick(() => {
    textareaRef.value?.focus()
  })
  
  try {
    await chatStore.sendMessage(sentContent, sentFiles, sentReplyTo)
    sendBtnState.value = 'idle'
    scrollToBottom(true, true)
  } catch (e) {
    messageInput.value = sentContent
    selectedFiles.value = sentFiles
    sendBtnState.value = 'error'
    showToast(e.message || 'Failed to send message')
    setTimeout(() => {
      sendBtnState.value = 'idle'
    }, 600)
  }
}

async function handlePickerSelect({ type, data }) {
  if (!canPost.value) return
  
  if (type === 'sticker') {
    showEmojiPicker.value = false
    const sentReplyTo = replyToMessage.value?.id
    replyToMessage.value = null
    try {
      const altText = data.name || data.shortcode || 'sticker'
      const stickerHtml = '<img src="' + data.url + '" alt="' + altText + '" class="sticker">'
      await chatStore.sendMessage(stickerHtml, [], sentReplyTo)
      scrollToBottom(true, true)
    } catch (e) {
      showToast(e.message || 'Failed to send sticker')
    }
  } else if (type === 'gif') {
    showEmojiPicker.value = false
    const sentReplyTo = replyToMessage.value?.id
    replyToMessage.value = null
    try {
      await chatStore.sendMessage(null, [], sentReplyTo, data)
      scrollToBottom(true, true)
    } catch (e) {
      showToast(e.message || 'Failed to send GIF')
    }
  } else if (type === 'emoji') {
    const textarea = textareaRef.value
    if (textarea) {
      const start = textarea.selectionStart
      const end = textarea.selectionEnd
      const text = messageInput.value
      messageInput.value = text.substring(0, start) + data + text.substring(end)
      nextTick(() => {
        textarea.focus()
        const newPos = start + data.length
        textarea.setSelectionRange(newPos, newPos)
      })
    } else {
      messageInput.value += data
    }
  }
}

function openPicker(tab) {
  initialPickerTab.value = tab
  showEmojiPicker.value = true
}

function triggerFileInput() {
  fileInput.value?.click()
}

function handleFileSelect(event) {
  const files = Array.from(event.target.files)
  selectedFiles.value = [...selectedFiles.value, ...files]
  event.target.value = ''
}

function removeFile(index) {
  selectedFiles.value.splice(index, 1)
}

function getFilePreview(file) {
  if (file.type.startsWith('image/')) {
    return URL.createObjectURL(file)
  }
  return null
}

function getFileIcon(file) {
  if (file.type.startsWith('audio/')) return 'music'
  if (file.type.startsWith('video/')) return 'video'
  return 'file'
}

async function handleReaction(messageId, emoji) {
  try {
    await chatStore.toggleReaction(messageId, emoji)
  } catch (e) {
    showToast('Failed to add reaction')
  }
}

async function handleDelete(messageId) {
  if (!confirm('Delete this message?')) return
  try {
    await chatStore.deleteMessage(messageId)
  } catch (e) {
    showToast('Failed to delete message')
  }
}

function selectAvatar(avatar) {
  authStore.updateAvatar(avatar)
  chatStore.updateAvatar(avatar)
  showAvatarPicker.value = false
}

function preloadImage(url) {
  if (!url) return
  const img = new Image()
  img.src = url
}

watch(lightboxIndex, (newVal) => {
  const sources = lightboxSources.value
  if (!sources.length) return
  if (newVal < sources.length - 1) {
    const next = sources[newVal + 1]
    const url = typeof next === 'string' ? next : next.url
    if (url && !url.match(/\.(mp4|webm|ogg|mov|mp3|wav)$/i)) { preloadImage(url) }
  }
  if (newVal > 0) {
    const prev = sources[newVal - 1]
    const url = typeof prev === 'string' ? prev : prev.url
    if (url && !url.match(/\.(mp4|webm|ogg|mov|mp3|wav)$/i)) { preloadImage(url) }
  }
})

watch(lightboxSources, (newSources) => {
  if (newSources.length) {
    const idx = lightboxIndex.value
    if (idx < newSources.length - 1) {
       const next = newSources[idx + 1]
       const url = typeof next === 'string' ? next : next.url
       if (url && !url.match(/\.(mp4|webm|ogg|mov|mp3|wav)$/i)) { preloadImage(url) }
    }
    if (idx > 0) {
       const prev = newSources[idx - 1]
       const url = typeof prev === 'string' ? prev : prev.url
       if (url && !url.match(/\.(mp4|webm|ogg|mov|mp3|wav)$/i)) { preloadImage(url) }
    }
  }
})

function openLightbox(sources, index = 0) {
  transitionName.value = 'fade'
  if (Array.isArray(sources)) {
    lightboxSources.value = sources
    lightboxIndex.value = index
  } else {
    const source = typeof sources === 'string' ? { url: sources, type: 'image' } : sources
    lightboxSources.value = [source]
    lightboxIndex.value = 0
  }
}

function handleOpenAttachments(attachments) {
  if (attachments && attachments.length > 0) {
     const media = attachments.filter(a => {
        if (a.type?.startsWith('image') || a.type?.startsWith('video') || a.type?.startsWith('audio') || a.type === 'gif') return true;
        if (a.url) {
            return a.url.match(/\.(png|jpg|jpeg|gif|webp|mp4|webm|ogg|mov|mp3|wav)$/i);
        }
        return false;
     });
     if (media.length > 0) { openLightbox(media, 0); }
  }
}

function handleSingleImageClick(url) { openLightbox(url) }
function closeLightbox() {
  lightboxSources.value = []
  lightboxIndex.value = 0
  transitionName.value = 'fade'
}
function nextLightboxImage() {
    if (lightboxIndex.value < lightboxSources.value.length - 1) {
        transitionName.value = 'slide-next'
        lightboxIndex.value++
    }
}
function prevLightboxImage() {
    if (lightboxIndex.value > 0) {
        transitionName.value = 'slide-prev'
        lightboxIndex.value--
    }
}
function showToast(message) {
  toast.value = message
  setTimeout(() => toast.value = '', 3000)
}
function goToLogin() { router.push('/login') }
function logout() {
  authStore.logout()
  chatStore.disconnect()
  window.location.reload()
}
function toggleTheme() {
  currentTheme.value = currentTheme.value === 'imessage' ? 'material' : 'imessage'
  document.documentElement.setAttribute('data-theme', currentTheme.value)
  localStorage.setItem('blog-theme', currentTheme.value)
}
function openReactionSheet(message) { selectedMessageForDetails.value = message }
function handleReply(message) {
  if (!canPost.value) return
  replyToMessage.value = message
  nextTick(() => { textareaRef.value?.focus() })
}
function cancelReply() { replyToMessage.value = null }
function handleKeydown(e) {
    if (lightboxSources.value.length > 0) {
        if (e.key === 'Escape') closeLightbox()
        if (e.key === 'ArrowRight') nextLightboxImage()
        if (e.key === 'ArrowLeft') prevLightboxImage()
    }
}
watch(() => chatStore.messages.length, (newLen, oldLen) => {
  if (newLen > oldLen) {
    if (isAtBottom.value) { scrollToBottom() } else { unreadCount.value += (newLen - oldLen) }
  }
})

onMounted(async () => {
  document.documentElement.setAttribute('data-theme', currentTheme.value)
  window.addEventListener('keydown', handleKeydown)
  await chatStore.fetchAvatars()
  await chatStore.fetchCustomEmojis()
  await chatStore.fetchMessages()
  chatStore.connectWebSocket()
  scrollToBottom(false, true)
  nextTick(() => { isAtBottom.value = true })
})

onUnmounted(() => {
  chatStore.disconnect()
  window.removeEventListener('keydown', handleKeydown)
  clearTimeout(typingTimeout)
})
</script>

<template>
  <div class="chat-container">
    <header class="header">
      <div class="header-left">
        <template v-if="!canPost">
          <button @click="goToLogin" class="header-btn">
            <Icon name="back" :size="22" :theme="currentTheme" />
            <span class="header-btn-text hidden-mobile">Login</span>
          </button>
        </template>
        <template v-else>
          <button @click="logout" class="header-btn">
            Logout
          </button>
        </template>
      </div>

      <div class="header-center">
        <span class="header-title">Blog</span>
        <span class="header-subtitle">{{ chatStore.onlineCount }} online</span>
      </div>

      <div class="header-right">
        <button v-if="isAdmin" @click="showAdminPanel = true" class="icon-btn" title="Admin Panel">
          <Icon name="shield" :size="22" :theme="currentTheme" />
        </button>

        <button @click="toggleTheme" class="icon-btn" title="Switch Theme">
          <Icon name="palette" :size="22" :theme="currentTheme" />
        </button>
        
        <button
          @click="showAvatarPicker = !showAvatarPicker"
          class="avatar avatar-btn"
          :title="authStore.user?.username"
        >
          <img v-if="avatarUrl" :src="avatarUrl" :alt="authStore.user?.username" />
          <div v-else class="avatar-fallback">
            <Icon name="user" :size="16" :theme="currentTheme" />
          </div>
        </button>

        <AvatarPicker
          v-if="showAvatarPicker"
          :current-avatar="authStore.user?.avatar"
          @select="selectAvatar"
          @close="showAvatarPicker = false"
        />
      </div>
    </header>
    
    <div
      ref="messagesContainer"
      class="messages-area"
      @scroll="handleScroll"
    >
      <div v-if="chatStore.loading && chatStore.messages.length" class="skeleton-wrapper">
        <div class="skeleton-row skeleton-sent">
            <div class="skeleton-avatar"></div>
            <div class="skeleton-bubble short"></div>
        </div>
        <div class="skeleton-row skeleton-sent">
            <div class="skeleton-bubble medium"></div>
        </div>
        <div class="skeleton-row skeleton-sent">
            <div class="skeleton-avatar"></div>
            <div class="skeleton-bubble long"></div>
        </div>
      </div>

      <div v-if="chatStore.loading && !chatStore.messages.length" class="skeleton-wrapper">
         <div class="skeleton-row skeleton-sent">
            <div class="skeleton-avatar"></div>
            <div class="skeleton-bubble medium"></div>
        </div>
        <div class="skeleton-row skeleton-sent">
            <div class="skeleton-bubble short"></div>
        </div>
        <div class="skeleton-row skeleton-sent">
            <div class="skeleton-avatar"></div>
            <div class="skeleton-bubble long"></div>
        </div>
      </div>

      <div v-if="pinnedMessages.length > 0" class="pinned-section">
        <div class="pinned-header">
          <Icon name="pin" :size="12" :theme="currentTheme" /> Pinned Messages
        </div>
        <MessageBubble
          v-for="message in pinnedMessages"
          :key="'pinned-'+message.id"
          :message="message"
          :current-user="authStore.user"
          :is-admin="isAdmin"
          :can-post="canPost"
          :is-first-in-group="message._isFirstInGroup"
          :is-last-in-group="message._isLastInGroup"
          :show-time="false"
          :theme="currentTheme"
          @react="handleReaction"
          @delete="handleDelete"
          @image-click="handleSingleImageClick"
          @open-attachments="handleOpenAttachments"
          @show-details="openReactionSheet"
          @reply="handleReply"
        />
        <div class="pinned-divider"></div>
      </div>

      <div v-if="!chatStore.loading && chatStore.messages.length === 0 && pinnedMessages.length === 0" class="empty-state">
        <Icon name="chat" :size="64" class="empty-state-icon" :theme="currentTheme" />
        <h2 class="empty-state-text">No messages yet</h2>
      </div>
      
      <TransitionGroup name="message">
        <MessageBubble
          v-for="message in groupedMessages"
          :key="message.id"
          :message="message"
          :current-user="authStore.user"
          :is-admin="isAdmin"
          :can-post="canPost"
          :is-first-in-group="message._isFirstInGroup"
          :is-last-in-group="message._isLastInGroup"
          :show-time="message._showTime"
          :theme="currentTheme"
          @react="handleReaction"
          @delete="handleDelete"
          @image-click="handleSingleImageClick"
          @open-attachments="handleOpenAttachments"
          @show-details="openReactionSheet"
          @reply="handleReply"
        />
      </TransitionGroup>
    </div>

    <Transition name="scroll-btn">
      <button 
        v-if="unreadCount > 0" 
        class="new-messages-indicator"
        @click="jumpToBottom"
      >
        <Icon name="arrow-down" :size="16" :theme="currentTheme" />
        {{ unreadCount }} new message{{ unreadCount > 1 ? 's' : '' }}
      </button>
    </Transition>

    <div v-if="canPost" class="composer-wrapper">
      <div v-if="replyToMessage" class="reply-preview">
        <div class="reply-preview-content">
          <span class="reply-preview-label">Replying to {{ replyToMessage.author_username }}</span>
          <span class="reply-preview-text">{{ replyToMessage.content || 'Attachment' }}</span>
        </div>
        <button @click="cancelReply" class="reply-preview-close">
          <Icon name="close" :size="20" :theme="currentTheme" />
        </button>
      </div>

      <Transition name="expand">
        <div v-if="selectedFiles.length > 0 && !isMd" class="file-preview-container">
          <div
            v-for="(file, index) in selectedFiles"
            :key="index"
            class="file-preview-item"
          >
            <img v-if="getFilePreview(file)" :src="getFilePreview(file)" />
            <div v-else class="file-preview-placeholder">
              <Icon :name="getFileIcon(file)" :size="24" :theme="currentTheme" />
            </div>
            <button @click="removeFile(index)" class="file-preview-remove">x</button>
          </div>
        </div>
      </Transition>
      
      <div class="input-row">
        <button 
           v-if="!isMd" 
           @click="triggerFileInput" 
           class="icon-btn outside-plus-btn" 
           title="Attach files"
        >
          <Icon name="plus" :size="26" :theme="currentTheme" />
        </button>

        <div class="input-container">
          <Transition name="expand">
             <div v-if="selectedFiles.length > 0 && isMd" class="file-preview-inline">
               <div
                  v-for="(file, index) in selectedFiles"
                  :key="index"
                  class="file-preview-item"
                >
                  <img v-if="getFilePreview(file)" :src="getFilePreview(file)" />
                  <div v-else class="file-preview-placeholder">
                    <Icon :name="getFileIcon(file)" :size="24" :theme="currentTheme" />
                  </div>
                  <button @click="removeFile(index)" class="file-preview-remove">x</button>
                </div>
             </div>
          </Transition>

          <input
            ref="fileInput"
            type="file"
            multiple
            accept="image/*,audio/*,video/*"
            class="hidden"
            @change="handleFileSelect"
          />

          <button 
            v-if="isMd"
            @click="triggerFileInput" 
            class="icon-btn" 
            title="Attach files"
          >
            <Icon name="plus" :size="26" :theme="currentTheme" />
          </button>
          
          <button @click="openPicker('emoji')" class="icon-btn emoji-sticker-btn" title="Emoji & Stickers">
            <Icon name="smile" :size="24" :theme="currentTheme" />
          </button>
          
          <textarea
            ref="textareaRef"
            v-model="messageInput"
            @input="handleTyping"
            @keydown.enter.exact.prevent="sendMessage"
            class="message-input"
            placeholder="Write a new message..."
            rows="1"
          ></textarea>
          
          <button
            @click="sendMessage"
            :disabled="!hasContent"
            :class="['send-btn', 'send-btn-inside', 'send-btn--'+sendBtnState]"
            title="Send"
          >
            <span class="send-btn-icon">
              <Icon name="arrow-up" :size="18" :theme="currentTheme" />
            </span>
          </button>
        </div>
        
        <button
          @click="sendMessage"
          :disabled="!hasContent"
          :class="['send-btn', 'send-btn-outside', 'send-btn--'+sendBtnState]"
          title="Send"
        >
          <span class="send-btn-icon">
            <Icon name="arrow-up" :size="18" :theme="currentTheme" />
          </span>
        </button>
      </div>
    </div>

    <div v-else class="guest-footer">
      <span>Viewing as <strong>{{ authStore.user?.username }}</strong></span>
      <span class="guest-footer-hint">· Long press to react</span>
    </div>

    <Teleport to="body">
      <Transition name="fade">
        <EmojiStickerPicker
          v-if="showEmojiPicker"
          :custom-emojis="chatStore.customEmojis"
          :initial-tab="initialPickerTab"
          @select="handlePickerSelect"
          @close="showEmojiPicker = false"
        />
      </Transition>
    </Teleport>

    <Teleport to="body">
      <Transition name="lightbox">
        <div v-if="lightboxSources.length > 0" class="lightbox" @click="closeLightbox">
          <button v-if="lightboxIndex > 0" class="lightbox-nav prev" @click.stop="prevLightboxImage">
            <Icon name="back" :size="32" :theme="currentTheme" />
          </button>
          <div class="lightbox-counter" v-if="lightboxSources.length > 1">
              {{ lightboxIndex + 1 }} / {{ lightboxSources.length }}
          </div>
          <div class="lightbox-content" @click.stop>
              <Transition :name="transitionName">
                <video v-if="lightboxType === 'video'" :key="'video-'+lightboxImage" :src="lightboxImage" class="lightbox-image" controls autoplay />
                <img v-else :key="'image-'+lightboxImage" :src="lightboxImage" class="lightbox-image" />
              </Transition>
          </div>
          <button v-if="lightboxIndex < lightboxSources.length - 1" class="lightbox-nav next" @click.stop="nextLightboxImage">
             <Icon name="forward" :size="32" :theme="currentTheme" />
          </button>
          <button @click="closeLightbox" class="lightbox-close">
            <Icon name="close" :size="32" :theme="currentTheme" />
          </button>
        </div>
      </Transition>
    </Teleport>

    <Teleport to="body">
      <Transition name="fade">
        <ReactionSheet
          v-if="selectedMessageForDetails"
          :message="selectedMessageForDetails"
          :is-admin="isAdmin"
          @close="selectedMessageForDetails = null"
        />
      </Transition>
    </Teleport>

    <Teleport to="body">
      <Transition name="fade">
        <AdminPanel
          v-if="showAdminPanel"
          @close="showAdminPanel = false"
        />
      </Transition>
    </Teleport>
    
    <Teleport to="body">
      <Transition name="toast">
        <div v-if="toast" class="toast">
          {{ toast }}
        </div>
      </Transition>
    </Teleport>
  </div>
</template>