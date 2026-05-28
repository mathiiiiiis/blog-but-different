<script setup>
import { onMounted, onUnmounted } from "vue";
import { useRouter } from "vue-router";
import { useAuthStore } from "../stores/auth";
import { useChatStore } from "../stores/chat";
import AppHeader from "../components/AppHeader.vue";

const router = useRouter();
const auth = useAuthStore();
const chat = useChatStore();

function goLogin() {
  router.push("/login");
}

function logout() {
  auth.logout();
  chat.disconnect();
  window.location.reload();
}

// overlays land in later slices
function openAdmin() {}
function toggleAvatar() {}

onMounted(async () => {
  await chat.fetchAvatars();
  await chat.fetchCustomEmojis();
  await chat.fetchMessages();
  chat.connectWebSocket();
});

onUnmounted(() => {
  chat.disconnect();
});
</script>

<template>
  <div class="chat">
    <AppHeader
      @login="goLogin"
      @logout="logout"
      @open-admin="openAdmin"
      @toggle-avatar="toggleAvatar"
    />

    <div class="chat__messages">
      <!-- message list slice -->
    </div>

    <!-- composer slice -->
  </div>
</template>
