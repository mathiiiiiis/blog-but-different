<script setup>
import { computed, inject } from "vue";
import { useAuthStore } from "../stores/auth";
import { useChatStore } from "../stores/chat";
import Icon from "./Icon.vue";

const props = defineProps({
  message: { type: Object, required: true },
  continuation: { type: Boolean, default: false },
});

const auth = useAuthStore();
const chat = useChatStore();

//provided by ChatView
//no reaction opener for now
const composer = inject("composer", null);
const openReactions = inject("reactions", null);

const m = computed(() => props.message);
const canPost = computed(() => auth.canPost);
const avatarUrl = computed(() => chat.getAvatarUrl(m.value.author_avatar));
const time = computed(() => fmtTime(m.value.created_at));
const edited = computed(() => !!m.value.edited_at);
const attachments = computed(() => m.value.attachments || []);
const reactions = computed(() => m.value.reactions || []);
const reply = computed(() => m.value.reply_to || null);

function fmtTime(iso) {
  if (!iso) return "";
  return new Date(iso).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" });
}
function reactedByMe(r) {
  return !!auth.user && (r.users || []).includes(auth.user.username);
}

function startReply() {
  composer?.reply(m.value);
}
function startEdit() {
  composer?.edit(m.value);
}
function react() {
  openReactions?.(m.value);
}
async function pin() {
  await chat.pinMessage(m.value.id);
}
async function remove() {
  if (confirm("Delete this message?")) await chat.deleteMessage(m.value.id);
}
function toggleReaction(r) {
  chat.toggleReaction(m.value.id, r.emoji, null);
}
</script>

<template>
  <div class="bubble" :class="{ 'bubble--cont': continuation, 'bubble--pinned': m.is_pinned }">
    <div class="bubble__gutter">
      <img
        v-if="!continuation"
        class="bubble__avatar"
        :src="avatarUrl"
        :alt="m.author_username"
        @error="$event.target.style.visibility = 'hidden'"
      />
    </div>

    <div class="bubble__main">
      <div v-if="!continuation" class="bubble__head">
        <span class="bubble__author">{{ m.author_username }}</span>
        <Icon v-if="m.is_admin" name="shield" :size="13" class="bubble__admin" />
        <span class="bubble__time">{{ time }}</span>
        <Icon v-if="m.is_pinned" name="pin" :size="13" class="bubble__pin" />
      </div>

      <div v-if="reply" class="bubble__reply">
        <Icon name="reply" :size="14" />
        <span class="bubble__reply-author">{{ reply.author_username }}</span>
        <span class="bubble__reply-text">{{ reply.content || "attachment" }}</span>
      </div>

      <p v-if="m.content" class="bubble__content">
        {{ m.content }}<span v-if="edited" class="bubble__edited"> (edited)</span>
      </p>

      <div v-if="attachments.length" class="bubble__attachments">
        <template v-for="(a, i) in attachments" :key="i">
          <img
            v-if="a.type === 'image' || a.type === 'gif'"
            class="bubble__media"
            :src="a.url"
            :alt="a.name"
            loading="lazy"
          />
          <img
            v-else-if="a.type === 'sticker'"
            class="bubble__sticker"
            :src="a.url"
            :alt="a.name"
            loading="lazy"
          />
          <audio v-else-if="a.type === 'audio'" class="bubble__audio" :src="a.url" controls />
          <video v-else-if="a.type === 'video'" class="bubble__video" :src="a.url" controls />
          <a v-else class="bubble__file" :href="a.url" target="_blank" rel="noopener">
            <Icon name="file" :size="20" />
            <span class="bubble__file-name">{{ a.name }}</span>
          </a>
        </template>
      </div>

      <div v-if="reactions.length" class="bubble__reactions">
        <button
          v-for="r in reactions"
          :key="r.emoji"
          class="reaction"
          :class="{ 'reaction--me': reactedByMe(r) }"
          :title="(r.users || []).join(', ')"
          @click="toggleReaction(r)"
        >
          <img
            v-if="r.custom_emoji_url"
            class="reaction__img"
            :src="r.custom_emoji_url"
            :alt="r.emoji"
          />
          <span v-else class="reaction__emoji">{{ r.emoji }}</span>
          <span class="reaction__count">{{ r.count }}</span>
        </button>
      </div>
    </div>

    <div class="bubble__actions">
      <button class="icon-btn icon-btn--sm" aria-label="React" @click="react">
        <Icon name="smile" :size="18" />
      </button>
      <template v-if="canPost">
        <button class="icon-btn icon-btn--sm" aria-label="Reply" @click="startReply">
          <Icon name="reply" :size="18" />
        </button>
        <button class="icon-btn icon-btn--sm" aria-label="Edit" @click="startEdit">
          <Icon name="pencil" :size="18" />
        </button>
        <button class="icon-btn icon-btn--sm" aria-label="Pin" @click="pin">
          <Icon name="pin" :size="18" />
        </button>
        <button class="icon-btn icon-btn--sm" aria-label="Delete" @click="remove">
          <Icon name="trash" :size="18" />
        </button>
      </template>
    </div>
  </div>
</template>
