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

//composer floats over messages; measure it so list can scroll clear
const dockEl = ref(null);
const dockH = ref(64);
let ro = null;

function goLogin() {
  router.push("/login");
}
function openAdmin() {} //admin panel slice
function toggleAvatar() {} //avatar picker slice
function openGif() {} //gif picker slice
function openEmoji() {} //emoji picker slice

onMounted(async () => {
  if (dockEl.value && "ResizeObserver" in window) {
    ro = new ResizeObserver(() => {
      dockH.value = dockEl.value?.offsetHeight ?? 64;
    });
    ro.observe(dockEl.value);
  }
  await chat.fetchAvatars();
  await chat.fetchCustomEmojis();
  await chat.fetchMessages();
  chat.connectWebSocket();
});

onUnmounted(() => {
  ro?.disconnect();
  chat.disconnect();
});
</script>

<template>
  <div class="chat" :class="{ 'is-scrolled': isScrolled }">
    <AppHeader @login="goLogin" @open-admin="openAdmin" @toggle-avatar="toggleAvatar" />

    <div class="chat__body" :style="{ '--dock-h': dockH + 'px' }">
      <MessageList @scrolled="isScrolled = $event" />

      <div ref="dockEl" class="chat__dock">
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
    </div>
  </div>
</template>
