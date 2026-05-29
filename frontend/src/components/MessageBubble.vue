<script setup>
defineProps({
  message: { type: Object, required: true },
});
</script>

<template>
  <div class="bubble">
    <span class="bubble__author">{{ message.author_username }}</span>
    <span class="bubble__body">{{ message.content }}</span>
  </div>
</template>
