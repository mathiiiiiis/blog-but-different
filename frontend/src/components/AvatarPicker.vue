<script setup>
import { useChatStore } from '../stores/chat'
import Icon from './Icon.vue'

const props = defineProps({
  currentAvatar: { type: String, default: 'default' }
})

const emit = defineEmits(['select', 'close'])

const chatStore = useChatStore()

function selectAvatar(avatarId) {
  emit('select', avatarId)
}

function getAvatarUrl(avatarId) {
    return chatStore.getAvatarUrl(avatarId)
}
</script>

<template>
  <div class="avatar-picker-overlay" @click="emit('close')">
    <div class="avatar-picker" @click.stop>
      <div class="avatar-picker-header">
        <h3 class="avatar-picker-title">Choose Avatar</h3>
        <button @click="emit('close')" class="icon-btn">
          <Icon name="close" :size="20" />
        </button>
      </div>
      
      <div v-if="chatStore.avatars.length === 0" class="avatar-loading">
        Loading avatars...
      </div>
      
      <div v-else class="avatar-grid">
        <button
          v-for="avatar in chatStore.avatars"
          :key="avatar.id"
          @click="selectAvatar(avatar.id)"
          class="avatar-option"
          :class="{ 'selected': currentAvatar === avatar.id }"
          :title="avatar.name"
        >
          <img :src="getAvatarUrl(avatar.id)" :alt="avatar.name" @error="$event.target.style.display='none'" />
        </button>
      </div>
    </div>
  </div>
</template>