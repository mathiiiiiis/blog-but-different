<script setup>
import { computed } from "vue";
import { useAuthStore } from "../stores/auth";
import { useChatStore } from "../stores/chat";
import Icon from "./Icon.vue";

const auth = useAuthStore();
const chat = useChatStore();

defineEmits(["login", "open-admin", "toggle-avatar"]);

const canPost = computed(() => auth.canPost);
const online = computed(() => chat.onlineCount);
const username = computed(() => auth.user?.username || "");
const avatarUrl = computed(() => chat.getAvatarUrl(auth.user?.avatar));

// who else is online, shown after the "+"
const others = computed(() => (chat.onlineUsers || []).filter((u) => u.user_id !== auth.user?.id));
const stack = computed(() => others.value.slice(0, 3));
const overflow = computed(() => Math.max(0, others.value.length - 3));
</script>

<template>
  <header class="app-header">
    <div class="app-header__lead">
      <span class="blog-title">Re:Blog (WIP)</span>
      <span class="presence">
        <span class="presence__dot"></span>
        {{ online }} online
      </span>
    </div>

    <div class="app-header__actions">
      <div class="presence-group">
        <template v-if="others.length">
          <div class="head-stack">
            <span v-for="u in stack" :key="u.user_id" class="head-stack__item" :title="u.username">
              <img
                :src="chat.getAvatarUrl(u.avatar)"
                :alt="u.username"
                @error="$event.target.style.display = 'none'"
              />
            </span>
            <span v-if="overflow" class="head-stack__more">+{{ overflow }}</span>
          </div>
          <span class="presence-plus">+</span>
        </template>

        <button class="head-avatar" :title="username" @click="$emit('toggle-avatar')">
          <img
            v-if="avatarUrl"
            :src="avatarUrl"
            :alt="username"
            @error="$event.target.style.display = 'none'"
          />
          <span v-else class="head-avatar__fallback">
            <Icon name="user" :size="16" />
          </span>
        </button>
      </div>

      <button v-if="canPost" class="icon-btn" title="Admin panel" @click="$emit('open-admin')">
        <Icon name="shield" :size="22" />
      </button>
      <button v-else class="text-btn" @click="$emit('login')">Login</button>
    </div>
  </header>
</template>
