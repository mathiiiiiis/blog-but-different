<script setup>
import { ref, computed, onMounted, onUnmounted } from 'vue'
import { useChatStore } from '../stores/chat'
import { emojiCategories } from '../emojis'
import Icon from './Icon.vue'

const props = defineProps({
  isAdmin: { type: Boolean, default: false }
})

const emit = defineEmits(['select', 'close', 'delete'])

const chatStore = useChatStore()
const isExpanded = ref(false)
const currentTab = ref('custom')
const recentEmojis = ref([])
const pickerRef = ref(null)

const DEFAULT_RECENT = ['❤️', '😂', '😮', '😢', '😡', '👍']


const hasCustomEmojis = computed(() => chatStore.customEmojis.length > 0)
const activeTab = computed(() => {
  if (currentTab.value === 'custom' && !hasCustomEmojis.value) {
    return emojiCategories[0].name
  }
  return currentTab.value
})

function loadRecent() {
  try {
    const stored = localStorage.getItem('recent_emojis')
    if (stored) {
      recentEmojis.value = JSON.parse(stored)
    } else {
      recentEmojis.value = DEFAULT_RECENT
    }
  } catch {
    recentEmojis.value = DEFAULT_RECENT
  }
}

function addToRecent(emoji) {
  const val = typeof emoji === 'string' ? emoji : emoji.name
  let newRecent = recentEmojis.value.filter(e => e !== val)
  newRecent.unshift(val)
  newRecent = newRecent.slice(0, 5)
  recentEmojis.value = newRecent
  localStorage.setItem('recent_emojis', JSON.stringify(newRecent))
}

function selectEmoji(emoji) {
  addToRecent(emoji)
  emit('select', emoji)
}

function selectCustomEmoji(emoji) {
  addToRecent(emoji.name)
  emit('select', emoji.name)
}

function getCompactEmoji(val) {
  const custom = chatStore.customEmojis.find(e => e.name === val)
  if (custom) return { type: 'custom', data: custom }
  return { type: 'standard', data: val }
}

function handleClose() {
  if (isExpanded.value) {
    isExpanded.value = false
  } else {
    emit('close')
  }
}

function handleDelete() {
  if (confirm('Delete this message?')) {
    emit('delete')
  }
}

function handleClickOutside(e) {
  if (pickerRef.value && !pickerRef.value.contains(e.target)) {
    emit('close')
  }
}

onMounted(() => {
  loadRecent()
  setTimeout(() => {
    document.addEventListener('click', handleClickOutside)
  }, 100)
})

onUnmounted(() => {
  document.removeEventListener('click', handleClickOutside)
})
</script>

<template>
  <div ref="pickerRef" class="reaction-wrapper" :class="{ expanded: isExpanded }" @click.stop>

    <div v-if="!isExpanded" class="compact-picker">
      <button class="compact-option close-btn" @click.stop="handleClose" title="Close">
        <Icon name="close" :size="14" />
      </button>

      <button 
        v-if="isAdmin" 
        class="compact-option delete-btn-compact" 
        @click.stop="handleDelete" 
        title="Delete message"
      >
        <Icon name="trash" :size="14" />
      </button>
      
      <div v-if="isAdmin" style="width: 1px; height: 16px; background: rgba(0,0,0,0.1); margin: 0 2px;"></div>
      
      <button 
        v-for="val in recentEmojis" 
        :key="val"
        class="compact-option"
        @click="val.length > 4 && chatStore.getCustomEmoji(val) ? selectCustomEmoji(chatStore.getCustomEmoji(val)) : selectEmoji(val)"
        :title="val"
      >
        <template v-if="getCompactEmoji(val).type === 'custom'">
           <img :src="getCompactEmoji(val).data.url" :alt="val" class="compact-custom-img" />
        </template>
        <template v-else>
           {{ val }}
        </template>
      </button>

      <button class="compact-option add-btn" @click.stop="isExpanded = true" title="More reactions">
        <Icon name="plus" :size="16" />
      </button>
    </div>

    <div v-else class="expanded-picker">
      <div class="picker-tabs">
        <button 
          class="picker-tab close-tab"
          @click="handleClose"
          title="Close"
        >
          ✕
        </button>
        <button 
          v-if="hasCustomEmojis"
          class="picker-tab"
          :class="{ active: activeTab === 'custom' }"
          @click="currentTab = 'custom'"
          title="Custom"
        >
          ★
        </button>
        <button 
          v-for="cat in emojiCategories" 
          :key="cat.name"
          class="picker-tab"
          :class="{ active: activeTab === cat.name }"
          @click="currentTab = cat.name"
          :title="cat.name"
        >
          {{ cat.emojis[0] }}
        </button>
      </div>

      <div class="picker-content">
        <div v-if="activeTab === 'custom'" class="emoji-grid custom-grid">
          <button 
            v-for="emoji in chatStore.customEmojis" 
            :key="emoji.id" 
            @click="selectCustomEmoji(emoji)" 
            class="reaction-option custom-option"
            :title="emoji.name"
          >
            <img :src="emoji.url" :alt="emoji.name" loading="lazy" />
          </button>
        </div>

        <template v-else>
          <div 
            v-for="cat in emojiCategories" 
            :key="cat.name"
            v-show="activeTab === cat.name"
            class="emoji-grid"
          >
            <button 
              v-for="emoji in cat.emojis" 
              :key="emoji" 
              @click="selectEmoji(emoji)" 
              class="reaction-option"
            >
              {{ emoji }}
            </button>
          </div>
        </template>
      </div>
    </div>
  </div>
</template>