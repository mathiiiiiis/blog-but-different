<script setup>
import { ref, computed, onMounted } from 'vue'
import Icon from './Icon.vue'

const props = defineProps({
  url: { type: String, required: true },
  theme: { type: String, default: 'imessage' }
})

const metaData = ref({
  title: null,
  image: null,
  favicon: null,
  description: null
})

const loading = ref(true)
const error = ref(false)
const imageLoadError = ref(false)

const domain = computed(() => {
  try {
    const u = new URL(props.url)
    return u.hostname.replace('www.', '')
  } catch {
    return props.url
  }
})

const displayImage = computed(() => {
  if (metaData.value.image && !imageLoadError.value) {
    return { type: 'banner', url: metaData.value.image }
  }
  if (metaData.value.favicon) {
    return { type: 'favicon', url: metaData.value.favicon }
  }
  return null
})

function onImageError() {
  imageLoadError.value = true
}

async function fetchMetadata() {
  loading.value = true
  error.value = false
  imageLoadError.value = false

  try {
    const response = await fetch(`https://api.microlink.io/?url=${encodeURIComponent(props.url)}`)
    const result = await response.json()

    if (result.status === 'success') {
      metaData.value = {
        title: result.data.title || domain.value,
        image: result.data.image?.url || null,
        favicon: result.data.logo?.url || null,
        description: result.data.description || result.data.url
      }
    } else {
      error.value = true
    }
  } catch (e) {
    console.error('Failed to fetch link preview', e)
    error.value = true
  } finally {
    loading.value = false
  }
}

onMounted(() => {
  fetchMetadata()
})
</script>

<template>
  <div class="link-preview" :class="theme">
    <div v-if="loading" class="link-preview-card link-preview-skeleton">
      <div class="link-preview-image-placeholder skeleton-pulse"></div>
      <div class="link-preview-info">
        <div class="skeleton-line skeleton-title"></div>
        <div class="skeleton-line skeleton-desc"></div>
      </div>
    </div>

    <a v-else :href="url" target="_blank" class="link-preview-card">
      <div v-if="displayImage?.type === 'banner'" class="link-preview-image-container">
        <img
          :src="displayImage.url"
          alt="Link preview"
          class="link-preview-img"
          loading="lazy"
          @error="onImageError"
        />
      </div>

      <div v-else-if="displayImage?.type === 'favicon'" class="link-preview-image-placeholder link-preview-favicon">
        <img
          :src="displayImage.url"
          alt="Site icon"
          class="link-preview-favicon-img"
          @error="onImageError"
        />
      </div>

      <div v-else class="link-preview-image-placeholder">
        <Icon name="link" :size="24" :theme="theme" />
      </div>

      <div class="link-preview-info">
        <div class="link-preview-title">{{ metaData.title || domain }}</div>
        <div class="link-preview-domain">{{ metaData.description || url }}</div>
      </div>
    </a>
  </div>
</template>