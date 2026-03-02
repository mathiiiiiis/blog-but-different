<script setup>
import { ref, computed, watch, onMounted, nextTick } from 'vue'
import { useChatStore } from '../stores/chat'
import Icon from './Icon.vue'
import { emojiCategories } from '../emojis'

const props = defineProps({
  customEmojis: {
    type: Array,
    default: () => []
  },
  initialTab: {
    type: String,
    default: 'emoji' //'emoji', 'sticker', 'gif'
  }
})

const emit = defineEmits(['select', 'close'])

const chatStore = useChatStore()
const mode = ref(props.initialTab)
const activeCategory = ref(0)

const gifSearchQuery = ref('')
const gifs = ref([])
const gifLoading = ref(false)
const gifNextPos = ref(null)
const gifError = ref('')
let searchTimeout = null

const filteredEmojis = computed(() => {
  return emojiCategories[activeCategory.value]?.emojis || []
})

const filteredStickers = computed(() => props.customEmojis)

// --- Methods ---

function selectEmoji(emoji) {
  emit('select', { type: 'emoji', data: emoji })
}

function selectSticker(sticker) {
  emit('select', { type: 'sticker', data: sticker })
}

function selectGif(gif) {
  emit('select', { 
    type: 'gif', 
    data: {
      id: gif.id,
      url: gif.url,
      preview_url: gif.preview_url
    }
  })
}

function handleBackdropClick(e) {
  if (e.target === e.currentTarget) {
    emit('close')
  }
}

async function loadTrendingGifs() {
  if (gifs.value.length > 0) return
  
  gifLoading.value = true
  gifError.value = ''
  try {
    const result = await chatStore.getTrendingGifs()
    gifs.value = result.results
    gifNextPos.value = result.next
  } catch (e) {
    gifError.value = e.message || 'Failed to load GIFs'
  } finally {
    gifLoading.value = false
  }
}

async function searchGifs() {
  if (!gifSearchQuery.value.trim()) {
    gifs.value = []
    gifNextPos.value = null
    await loadTrendingGifs()
    return
  }
  
  gifLoading.value = true
  gifError.value = ''
  try {
    const result = await chatStore.searchGifs(gifSearchQuery.value)
    gifs.value = result.results
    gifNextPos.value = result.next
  } catch (e) {
    gifError.value = e.message || 'Failed to search GIFs'
  } finally {
    gifLoading.value = false
  }
}

async function loadMoreGifs() {
  if (!gifNextPos.value || gifLoading.value) return
  
  gifLoading.value = true
  try {
    const result = gifSearchQuery.value.trim()
      ? await chatStore.searchGifs(gifSearchQuery.value, gifNextPos.value)
      : await chatStore.getTrendingGifs(gifNextPos.value)
    
    gifs.value = [...gifs.value, ...result.results]
    gifNextPos.value = result.next
  } catch (e) {
    console.error('Failed to load more GIFs:', e)
  } finally {
    gifLoading.value = false
  }
}

function handleGifSearchInput() {
  clearTimeout(searchTimeout)
  searchTimeout = setTimeout(searchGifs, 400)
}

function handleGifScroll(e) {
  const { scrollTop, scrollHeight, clientHeight } = e.target
  if (scrollHeight - scrollTop - clientHeight < 200) {
    loadMoreGifs()
  }
}

//watch mode to load gifs when tab switched
watch(mode, (newMode) => {
  if (newMode === 'gif' && gifs.value.length === 0) {
    loadTrendingGifs()
  }
})

onMounted(() => {
  if (mode.value === 'gif') {
    loadTrendingGifs()
  }
})
</script>

<template>
  <div class="emoji-picker-backdrop" @click="handleBackdropClick">
    <div class="emoji-picker-sheet">
      <div class="emoji-picker-handle">
        <div class="emoji-picker-handle-bar"></div>
      </div>
      
      <div class="emoji-picker-header">
        <h3 class="emoji-picker-title">
          <span v-if="mode === 'sticker'">Stickers</span>
          <span v-else-if="mode === 'gif'">GIFs</span>
          <span v-else>Emoji</span>
        </h3>
        <button @click="emit('close')" class="emoji-picker-close">
          <Icon name="close" :size="24" />
        </button>
      </div>

      <div class="emoji-picker-toggle">
        <button 
          :class="['toggle-btn', { active: mode === 'emoji' }]"
          @click="mode = 'emoji'"
        >
          Emoji
        </button>
        <button 
          :class="['toggle-btn', { active: mode === 'sticker' }]"
          @click="mode = 'sticker'"
        >
          Stickers
        </button>
        <button 
          :class="['toggle-btn', { active: mode === 'gif' }]"
          @click="mode = 'gif'"
        >
          GIFs
        </button>
        
        <div 
            class="toggle-indicator" 
            :class="{ 
                'pos-emoji': mode === 'emoji',
                'pos-sticker': mode === 'sticker',
                'pos-gif': mode === 'gif'
            }"
        ></div>
      </div>

      <div v-if="mode === 'sticker'" class="emoji-picker-content">
        <div class="mode-hint">
            <Icon name="info" :size="14" />
            <span>Stickers send as big custom emoji</span>
        </div>
        <div class="category-label">Custom Stickers</div>
        
        <div v-if="filteredStickers.length > 0" class="sticker-grid">
          <button
            v-for="sticker in filteredStickers"
            :key="sticker.id || sticker.shortcode"
            class="sticker-option"
            @click="selectSticker(sticker)"
            :title="sticker.name || sticker.shortcode"
          >
            <img :src="sticker.url" :alt="sticker.name || sticker.shortcode" class="sticker-img" />
          </button>
        </div>

        <div v-else class="emoji-empty">
          <template v-if="customEmojis.length === 0">
            No custom stickers available
          </template>
          <template v-else>
            No stickers found
          </template>
        </div>
      </div>

      <div v-else-if="mode === 'gif'" class="emoji-picker-content no-padding">
         <div class="gif-search-bar">
            <div class="gif-input-wrapper">
                <Icon name="search" :size="16" class="gif-search-icon"/>
                <input 
                    v-model="gifSearchQuery"
                    @input="handleGifSearchInput"
                    type="text" 
                    placeholder="Search GIFs..." 
                    class="gif-input"
                    autofocus
                />
            </div>
         </div>

         <div class="gif-scroll-area" @scroll="handleGifScroll">
            <div v-if="gifError" class="gif-status">{{ gifError }}</div>
            <div v-else-if="gifLoading && gifs.length === 0" class="gif-status">Loading GIFs...</div>
            <div v-else-if="gifs.length === 0" class="gif-status">No GIFs found</div>
            
            <div v-else class="gif-grid">
                <div
                    v-for="gif in gifs"
                    :key="gif.id"
                    class="gif-item"
                    @click="selectGif(gif)"
                >
                    <img :src="gif.preview_url" :alt="gif.title" loading="lazy" />
                </div>
            </div>
            
            <div v-if="gifLoading && gifs.length > 0" class="gif-loading-more">
                Loading more...
            </div>
            <div class="gif-attribution">Powered by Klipy</div>
         </div>
      </div>

      <template v-else>
        <div class="emoji-category-tabs">
          <button
            v-for="(category, index) in emojiCategories"
            :key="category.name"
            :class="['category-tab', { active: activeCategory === index }]"
            @click="activeCategory = index"
            :title="category.name"
          >
            {{ category.emojis[0] }}
          </button>
        </div>

        <div class="emoji-picker-content">
          <div class="mode-hint">
            <Icon name="info" :size="14" />
            <span>Emoji inserts into your message</span>
          </div>
          <div class="category-label">
            {{ emojiCategories[activeCategory]?.name }}
          </div>
          
          <div class="emoji-grid-picker">
            <button
              v-for="emoji in filteredEmojis"
              :key="emoji"
              class="emoji-option"
              @click="selectEmoji(emoji)"
            >
              <span class="emoji-char">{{ emoji }}</span>
            </button>
          </div>

          <div v-if="filteredEmojis.length === 0" class="emoji-empty">
            No emoji found
          </div>
        </div>
      </template>
    </div>
  </div>
</template>