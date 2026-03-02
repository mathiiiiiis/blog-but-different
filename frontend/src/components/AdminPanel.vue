<script setup>
import { ref } from 'vue'
import { useChatStore } from '../stores/chat'
import Icon from './Icon.vue'

const emit = defineEmits(['close'])
const chatStore = useChatStore()

const emojiName = ref('')
const emojiFile = ref(null)
const emojiPreview = ref(null)
const uploadLoading = ref(false)
const isDragging = ref(false)

function handleFileSelect(e) {
  const file = e.target.files?.[0]
  if (file) processFile(file)
}

function handleDrop(e) {
  e.preventDefault()
  isDragging.value = false
  const file = e.dataTransfer.files?.[0]
  if (file && file.type.startsWith('image/')) {
    processFile(file)
  }
}

function processFile(file) {
  emojiFile.value = file
  emojiPreview.value = URL.createObjectURL(file)
  if (!emojiName.value) {
    emojiName.value = file.name.split('.')[0].toLowerCase().replace(/[^a-z0-9]/g, '_')
  }
}

function clearFile() {
  emojiFile.value = null
  emojiPreview.value = null
  emojiName.value = ''
}

async function handleUpload() {
  if (!emojiFile.value || !emojiName.value) return
  
  uploadLoading.value = true
  try {
    await chatStore.uploadCustomEmoji(emojiFile.value, emojiName.value)
    clearFile()
  } catch (e) {
    alert('Failed to upload emoji')
  } finally {
    uploadLoading.value = false
  }
}

async function handleDelete(id) {
  if (!confirm('Delete this emoji?')) return
  try {
    await chatStore.deleteCustomEmoji(id)
  } catch (e) {
    alert('Failed to delete emoji')
  }
}
</script>

<template>
  <div class="overlay" @click="emit('close')">
    <div class="panel" @click.stop>
      <div class="header">
        <h2>Custom Emojis</h2>
        <button @click="emit('close')" class="close-btn">
          <Icon name="close" :size="20" />
        </button>
      </div>

      <div class="content">
        <div 
          class="upload-zone"
          :class="{ dragging: isDragging, 'has-file': emojiFile }"
          @dragover.prevent="isDragging = true"
          @dragleave="isDragging = false"
          @drop="handleDrop"
        >
          <template v-if="!emojiFile">
            <Icon name="plus" :size="32" class="upload-icon" />
            <p>Drop image here or click to upload</p>
            <input 
              type="file" 
              accept="image/*"
              @change="handleFileSelect"
              class="file-input"
            />
          </template>
          <template v-else>
            <img :src="emojiPreview" class="emoji-preview" />
            <input 
              v-model="emojiName" 
              placeholder="emoji_name"
              class="name-input"
              @keydown.enter="handleUpload"
            />
            <p class="hint">:{{ emojiName }}:</p>
            <div class="upload-actions">
              <button @click="clearFile" class="btn-secondary">Cancel</button>
              <button 
                @click="handleUpload" 
                class="btn-primary"
                :disabled="!emojiName || uploadLoading"
              >
                {{ uploadLoading ? 'Uploading...' : 'Upload' }}
              </button>
            </div>
          </template>
        </div>

        <div v-if="chatStore.customEmojis.length" class="emoji-list">
          <div 
            v-for="emoji in chatStore.customEmojis" 
            :key="emoji.id" 
            class="emoji-item"
          >
            <img :src="emoji.url" :alt="emoji.name" />
            <span class="emoji-name">:{{ emoji.name }}:</span>
            <button @click="handleDelete(emoji.id)" class="delete-btn">
              <Icon name="trash" :size="14" />
            </button>
          </div>
        </div>
        <div v-else class="empty-state">
          <p>No custom emojis yet</p>
        </div>
      </div>
    </div>
  </div>
</template>