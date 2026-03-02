<script setup>
import { ref, computed } from 'vue'
import Icon from './Icon.vue'
import LinkPreview from './LinkPreview.vue'
import ReactionPicker from './ReactionPicker.vue'
import { useChatStore } from '../stores/chat'

const props = defineProps({
  message: { type: Object, required: true },
  currentUser: { type: Object, default: null },
  isAdmin: { type: Boolean, default: false },
  //grouping props
  isFirstInGroup: { type: Boolean, default: true },
  isLastInGroup: { type: Boolean, default: true },
  showTime: { type: Boolean, default: true },
  theme: { type: String, default: 'imessage' }
})

const emit = defineEmits(['react', 'delete', 'imageClick', 'openAttachments', 'showDetails', 'reply'])

const chatStore = useChatStore()
const showReactionPicker = ref(false)

//swipe logic state
const touchStartX = ref(0)
const touchStartY = ref(0)
const touchCurrentX = ref(0)
const isSwiping = ref(false)
const isScrolling = ref(false) 
const isDragging = ref(false)
const swipeThreshold = 50
const dragThreshold = 10 

//mouse long press state
let longPressTimer = null
const isLongPress = ref(false)

const formattedTime = computed(() => {
  const date = new Date(props.message.created_at)
  return date.toLocaleTimeString([], { hour: 'numeric', minute: '2-digit' })
})

const avatarUrl = computed(() => {
  return chatStore.getAvatarUrl(props.message.author_avatar)
})

const isMd = computed(() => props.theme === 'material')

const hasAttachments = computed(() => props.message.attachments && props.message.attachments.length > 0)
const hasGif = computed(() => !!props.message.gif_url)
const hasMedia = computed(() => hasAttachments.value || hasGif.value)
const hasContent = computed(() => !!props.message.content)

//detect sticker messages
const isSticker = computed(() => {
  const content = props.message.content
  if (!content) return false
  return content.trim().startsWith('<img') && content.includes('class="sticker"')
})

//safe extraction of sticker data
const stickerData = computed(() => {
  if (!isSticker.value) return null
  const content = props.message.content
  const srcMatch = content.match(/src="([^"]+)"/)
  const altMatch = content.match(/alt="([^"]+)"/)
  
  return {
    src: srcMatch ? srcMatch[1] : '',
    alt: altMatch ? altMatch[1] : 'Sticker'
  }
})

const isConnected = computed(() => isMd.value && hasMedia.value && hasContent.value)

const bubbleClass = computed(() => {
  const base = props.message.is_admin ? 'bubble-sent' : 'bubble-received'
  let radiusClass = ''
  
  if (props.message.is_admin) {
    if (props.isFirstInGroup && !props.isLastInGroup) radiusClass = 'group-sent-first'
    else if (!props.isFirstInGroup && !props.isLastInGroup) radiusClass = 'group-sent-middle'
    else if (!props.isFirstInGroup && props.isLastInGroup) radiusClass = 'group-sent-last'
  } else {
    if (props.isFirstInGroup && !props.isLastInGroup) radiusClass = 'group-received-first'
    else if (!props.isFirstInGroup && !props.isLastInGroup) radiusClass = 'group-received-middle'
    else if (!props.isFirstInGroup && props.isLastInGroup) radiusClass = 'group-received-last'
  }

  const connectionClass = isConnected.value ? 'connected-bottom' : ''
  const stickerClass = isSticker.value ? 'sticker-message' : ''
  
  return `${base} ${radiusClass} ${connectionClass} ${props.message.is_pinned ? 'pinned' : ''} ${stickerClass}`
})

const mediaContainerClass = computed(() => {
    return isConnected.value ? 'connected-top' : ''
})

const swipeStyle = computed(() => {
  if (!props.isAdmin) return {}
  if (!isSwiping.value || isScrolling.value || !isDragging.value) return {}
  const diff = touchCurrentX.value - touchStartX.value
  if (diff > 0) return {}
  const translate = Math.max(diff * 0.4, -80)
  return { transform: `translateX(${translate}px)` }
})

const firstAttachment = computed(() => hasAttachments.value ? props.message.attachments[0] : null)
const remainingAttachmentsCount = computed(() => hasAttachments.value ? props.message.attachments.length - 1 : 0)

const linkUrl = computed(() => {
  if (!props.message.content) return null
  const urlRegex = /(https?:\/\/[^\s]+)/
  const match = props.message.content.match(urlRegex)
  return match ? match[0] : null
})

function userHasReacted(reaction) {
  return props.currentUser && reaction.users.includes(props.currentUser.username)
}

function handleReactionClick(emoji) {
  if (!isLongPress.value) {
    emit('react', props.message.id, emoji)
    showReactionPicker.value = false
  }
  isLongPress.value = false
}

function handlePickerSelect(emoji) {
  emit('react', props.message.id, emoji)
  showReactionPicker.value = false
}

function handlePin() {
    chatStore.pinMessage(props.message.id)
}

function handleImageClick(url) {
  if (!isLongPress.value) {
    if (remainingAttachmentsCount.value > 0) {
      emit('openAttachments', props.message.attachments)
    } else {
      emit('imageClick', url)
    }
  }
}

function toggleReactionPicker(e) {
  if (e) {
    e.preventDefault()
    e.stopPropagation()
  }
  showReactionPicker.value = !showReactionPicker.value
}

function isImage(file) {
  if (!file) return false
  if (file.type && file.type.startsWith('image')) return true
  const extensions = ['.gif', '.png', '.jpg', '.jpeg', '.webp', '.svg', '.bmp', '.ico', '.tiff']
  if (file.url) {
    const url = file.url.toLowerCase().split('?')[0]
    if (file.url.toLowerCase().includes('tenor.com')) return true
    if (extensions.some(ext => url.endsWith(ext))) return true
  }
  if (file.name) {
    const name = file.name.toLowerCase()
    if (extensions.some(ext => name.endsWith(ext))) return true
  }
  return false
}

//gestures
function handleTouchStart(e) {
  touchStartX.value = e.touches[0].clientX
  touchStartY.value = e.touches[0].clientY
  touchCurrentX.value = e.touches[0].clientX
  
  if (props.isAdmin) {
    isSwiping.value = true
    isScrolling.value = false
    isDragging.value = false
  }
  
  startLongPress()
}

function handleTouchMove(e) {
  if (!isSwiping.value) return 

  const currentX = e.touches[0].clientX
  const currentY = e.touches[0].clientY
  
  const diffX = Math.abs(currentX - touchStartX.value)
  const diffY = Math.abs(currentY - touchStartY.value)

  if (diffY > diffX && diffY > 5) {
    isScrolling.value = true
    isSwiping.value = false
    isDragging.value = false
    cancelLongPress()
    return
  }

  if (diffX > dragThreshold) {
    isDragging.value = true
    cancelLongPress()
  }

  touchCurrentX.value = currentX
}

function handleTouchEnd() {
  cancelLongPress()
  if (!isSwiping.value || isScrolling.value) {
    resetSwipeState()
    return
  }
  const diff = touchCurrentX.value - touchStartX.value
  if (props.isAdmin && isDragging.value && diff < -swipeThreshold) {
    if (navigator.vibrate) navigator.vibrate(50)
    emit('reply', props.message)
  }
  resetSwipeState()
}

function resetSwipeState() {
  isSwiping.value = false
  isScrolling.value = false
  isDragging.value = false
  touchStartX.value = 0
  touchCurrentX.value = 0
}

function handleMouseDown(e) {
  touchStartX.value = e.clientX
  touchStartY.value = e.clientY
  touchCurrentX.value = e.clientX
  
  if (props.isAdmin) {
    isSwiping.value = true
    isDragging.value = false
  }
  startLongPress()
}

function handleMouseMove(e) {
  if (!isSwiping.value) return
  const diffX = Math.abs(e.clientX - touchStartX.value)
  const diffY = Math.abs(e.clientY - touchStartY.value)
  if (diffX > dragThreshold && diffX > diffY) {
    isDragging.value = true
    cancelLongPress()
  }
  if (isDragging.value) {
    touchCurrentX.value = e.clientX
  }
}

function handleMouseUp() {
  cancelLongPress()
  const diff = touchCurrentX.value - touchStartX.value
  if (props.isAdmin && isDragging.value && diff < -swipeThreshold) {
    if (navigator.vibrate) navigator.vibrate(50)
    emit('reply', props.message)
  }
  resetSwipeState()
}

function handleMouseLeave() {
  if (isSwiping.value || longPressTimer) {
    cancelLongPress()
    resetSwipeState()
  }
}

function startLongPress() {
  isLongPress.value = false
  if (longPressTimer) clearTimeout(longPressTimer)
  longPressTimer = setTimeout(() => {
    isLongPress.value = true
    if (navigator.vibrate) navigator.vibrate(50)
    showReactionPicker.value = true
    isSwiping.value = false
    isDragging.value = false
  }, 500)
}

function cancelLongPress() {
  if (longPressTimer) {
    clearTimeout(longPressTimer)
    longPressTimer = null
  }
}
</script>

<template>
  <div class="message-row">

    <div v-if="showTime" class="message-time">
      {{ formattedTime }}
    </div>

    <div 
      class="message-container"
      :class="{ 'justify-end': message.is_admin }"
      :style="swipeStyle"
      @touchstart="handleTouchStart"
      @touchmove="handleTouchMove"
      @touchend="handleTouchEnd"
      @mousedown="handleMouseDown"
      @mousemove="handleMouseMove"
      @mouseup="handleMouseUp"
      @mouseleave="handleMouseLeave"
    >
      <div v-if="!message.is_admin" class="avatar-space">
        <div v-if="isLastInGroup" class="avatar">
          <img v-if="avatarUrl" :src="avatarUrl" :alt="message.author_username" @error="$event.target.style.display='none'" />
          <div v-else class="avatar-fallback">
            <Icon name="user" :size="16" :theme="theme" />
          </div>
        </div>
        <div v-else class="avatar-spacer"></div>
      </div>
      
      <div class="message-content-col" :class="{ 'items-end': message.is_admin, 'items-start': !message.is_admin }">
        <div v-if="!message.is_admin && isFirstInGroup" class="message-author">
          {{ message.author_username }}
        </div>
        
        <div v-if="message.is_pinned" class="pinned-label">
            <Icon name="pin" :size="12" :theme="theme" /> Pinned
        </div>

        <div class="bubble-wrapper">

            <div class="desktop-actions">
                <button v-if="isAdmin" @click.stop="emit('reply', message)" class="action-btn" title="Reply">
                  <Icon name="reply" :size="16" :theme="theme" />
                </button>
                <button @click.stop="toggleReactionPicker" class="action-btn" title="React">
                  <Icon name="smile" :size="16" :theme="theme" />
                </button>
                <button v-if="isAdmin" @click.stop="handlePin" class="action-btn" :class="{ 'active': message.is_pinned }" :title="message.is_pinned ? 'Unpin' : 'Pin'">
                  <Icon name="pin" :size="16" :theme="theme" />
                </button>
            </div>

            <div class="message-stack">
              <div v-if="hasMedia && isMd" class="media-standalone-container" :class="mediaContainerClass">
                  <div v-if="message.gif_url" class="media-standalone" @click.stop="handleImageClick(message.gif_url)">
                     <img :src="message.gif_url" class="attachment-gif" />
                  </div>

                  <div v-else class="media-standalone" @click.stop="handleImageClick(firstAttachment.url)">
                    <img v-if="isImage(firstAttachment)" :src="firstAttachment.url" class="attachment-img" />
                    <video v-else-if="firstAttachment.type === 'video'" :src="firstAttachment.url" controls playsinline class="attachment-video" @click.stop />
                    <audio v-else-if="firstAttachment.type === 'audio'" :src="firstAttachment.url" controls class="attachment-audio" @click.stop />
                    
                    <a v-else :href="firstAttachment.url" target="_blank" class="attachment-file-standalone" @click.stop>
                        <div class="file-icon-wrapper">
                            <Icon name="file" :size="30" :theme="theme" />
                        </div>
                        <div class="file-info-col">
                            <span class="file-name">{{ firstAttachment.name }}</span>
                            <span class="file-meta">Attachment</span>
                        </div>
                        <div class="file-download-action">
                             <Icon name="arrow-down" :size="20" :theme="theme" />
                        </div>
                    </a>
                    
                    <div v-if="remainingAttachmentsCount > 0" class="attachment-more-overlay" @click.stop="emit('openAttachments', message.attachments)">
                        +{{ remainingAttachmentsCount }}
                    </div>
                  </div>
              </div>

              <div 
                v-if="message.content || (!isMd && hasMedia)"
                class="bubble"
                :class="bubbleClass"
              >
                <div v-if="message.reply_to" class="reply-context">
                   <div class="reply-author">{{ message.reply_to.author_username }}</div>
                   <div class="reply-text">
                     {{ message.reply_to.content?.includes('class="sticker"') ? 'Sticker' : (message.reply_to.content || 'Attachment') }}
                   </div>
                </div>

                <div v-if="hasMedia && !isMd" class="bubble-attachments">
                   <div v-if="message.gif_url" @click.stop="handleImageClick(message.gif_url)">
                     <img :src="message.gif_url" class="attachment-gif" />
                   </div>

                  <div v-else>
                      <div @click.stop="handleImageClick(firstAttachment.url)">
                          <img v-if="isImage(firstAttachment)" :src="firstAttachment.url" class="attachment-img" />
                          <video v-else-if="firstAttachment.type === 'video'" :src="firstAttachment.url" controls playsinline class="attachment-video" @click.stop />
                          <audio v-else-if="firstAttachment.type === 'audio'" :src="firstAttachment.url" controls class="attachment-audio" @click.stop />
                          
                          <a v-else :href="firstAttachment.url" target="_blank" class="attachment-file-standalone" @click.stop>
                            <div class="file-icon-wrapper">
                                <Icon name="file" :size="30" :theme="theme" />
                            </div>
                            <div class="file-info-col">
                                <span class="file-name">{{ firstAttachment.name }}</span>
                                <span class="file-meta">Attachment</span>
                            </div>
                          </a>
                      </div>

                      <div v-if="remainingAttachmentsCount > 0" class="attachment-more-overlay" @click.stop="emit('openAttachments', message.attachments)">
                          +{{ remainingAttachmentsCount }}
                      </div>
                  </div>
                </div>

                <div v-if="isSticker && stickerData" class="sticker-content">
                    <img :src="stickerData.src" :alt="stickerData.alt" class="sticker" />
                </div>

                <p v-if="message.content && !isSticker" class="bubble-text">
                  {{ message.content }}
                </p>
                
                <LinkPreview v-if="linkUrl" :url="linkUrl" :theme="theme" />
              </div>
            </div>

        </div>

        <div v-if="showReactionPicker" class="reaction-picker-wrapper">
           <ReactionPicker 
             :is-admin="isAdmin"
             @select="handlePickerSelect" 
             @close="showReactionPicker = false"
             @delete="emit('delete', message.id)"
           />
        </div>
        
        <div v-if="message.reactions?.length" class="reactions-container" :class="{ 'justify-end': message.is_admin }">
          <button
            v-for="reaction in message.reactions"
            :key="reaction.emoji"
            @click.stop="handleReactionClick(reaction.emoji)"
            @contextmenu.prevent="emit('showDetails', message)"
            class="reaction-btn"
            :class="{ 'active': userHasReacted(reaction) }"
          >
            <template v-if="reaction.custom_emoji_url">
                <img :src="reaction.custom_emoji_url" :alt="reaction.emoji" class="reaction-custom-emoji" />
            </template>
            <span v-else class="reaction-emoji">{{ reaction.emoji }}</span>
            <span v-if="reaction.count > 1" class="reaction-count">{{ reaction.count }}</span>
          </button>
        </div>
      </div>
    </div>
  </div>
</template>