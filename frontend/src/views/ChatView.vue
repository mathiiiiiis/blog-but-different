<script setup>
import { ref, provide, onMounted, onUnmounted } from "vue";
import { useRouter } from "vue-router";
import { useAuthStore } from "../stores/auth";
import { useChatStore } from "../stores/chat";
import AppHeader from "../components/AppHeader.vue";
import MessageList from "../components/MessageList.vue";
import Composer from "../components/Composer.vue";
import Icon from "../components/Icon.vue";

const router = useRouter();
const auth = useAuthStore();
const chat = useChatStore();

// ==== reply / edit target ====
const replyTo = ref(null);
const editing = ref(null);
provide("composer", {
  reply: (m) => {
    replyTo.value = m;
    editing.value = null;
  },
  edit: (m) => {
    editing.value = m;
    replyTo.value = null;
  },
});

//header gains shadow once list is scrolled > emitted by MessageList
const isScrolled = ref(false);

function goLogin() {
  router.push("/login");
}
function openAdmin() {} //admin panel slice
function toggleAvatar() {} //avatar picker slice
function openGif() {} //gif picker slice
function openEmoji() {} //emoji picker slice

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
  <div class="chat" :class="{ 'is-scrolled': isScrolled }">
    <AppHeader @login="goLogin" @open-admin="openAdmin" @toggle-avatar="toggleAvatar" />

    <MessageList @scrolled="isScrolled = $event" />

    <Composer
      v-if="auth.canPost"
      :reply-to="replyTo"
      :editing="editing"
      @cancel-reply="replyTo = null"
      @cancel-edit="editing = null"
      @open-gif="openGif"
      @open-emoji="openEmoji"
    />
    <div v-else class="composer-guest">
      <Icon name="info" :size="16" />
      viewing as {{ auth.user?.username || "guest" }}
    </div>
  </div>
</template>
