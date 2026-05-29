<script setup>
import { ref, computed, watch, nextTick, onMounted } from "vue";
import { useChatStore } from "../stores/chat";
import MessageBubble from "./MessageBubble.vue";
import Icon from "./Icon.vue";

const emit = defineEmits(["scrolled"]);
const chat = useChatStore();

const scrollEl = ref(null);
const atBottom = ref(true);
const NEAR = 80; //px from bottom still counts as "at the bottom"

const messages = computed(() => chat.messages || []);
const typing = computed(() => chat.typingUsers || []);

//group consecutive messages and insert day dividers
function dayKey(iso) {
  const d = new Date(iso);
  return `${d.getFullYear()}-${d.getMonth()}-${d.getDate()}`;
}
function dayLabel(iso) {
  const d = new Date(iso);
  const today = new Date();
  const yest = new Date();
  yest.setDate(today.getDate() - 1);
  if (dayKey(iso) === dayKey(today)) return "Today";
  if (dayKey(iso) === dayKey(yest)) return "Yesterday";
  return d.toLocaleDateString([], { weekday: "long", day: "numeric", month: "long" });
}
function withinMin(a, b, mins) {
  return Math.abs(new Date(b) - new Date(a)) < mins * 60000;
}

const rows = computed(() =>
  messages.value.map((m, i) => {
    const prev = messages.value[i - 1];
    const newDay = !prev || dayKey(prev.created_at) !== dayKey(m.created_at);
    const continuation =
      !!prev &&
      !newDay &&
      prev.author_id === m.author_id &&
      withinMin(prev.created_at, m.created_at, 5);
    return { message: m, divider: newDay ? dayLabel(m.created_at) : null, continuation };
  }),
);

const typingLabel = computed(() => {
  const names = typing.value.map((u) => (typeof u === "string" ? u : u?.username)).filter(Boolean);
  if (!names.length) return "";
  if (names.length === 1) return `${names[0]} is typing…`;
  if (names.length === 2) return `${names[0]} and ${names[1]} are typing…`;
  return `${names[0]} and ${names.length - 1} others are typing…`; //just in-case I might add multi-users
});

function isNearBottom() {
  const el = scrollEl.value;
  if (!el) return true;
  return el.scrollHeight - el.scrollTop - el.clientHeight < NEAR;
}
function scrollToBottom(smooth = false) {
  const el = scrollEl.value;
  if (!el) return;
  el.scrollTo({ top: el.scrollHeight, behavior: smooth ? "smooth" : "auto" });
}
function onScroll() {
  const el = scrollEl.value;
  if (!el) return;
  atBottom.value = isNearBottom();
  emit("scrolled", el.scrollTop > 0);
}

//stay pinned to latest message unless user scrolled up
watch(
  () => messages.value.length,
  async (n, o) => {
    const stick = atBottom.value || n < o;
    await nextTick();
    if (stick) scrollToBottom();
  },
);
watch(typingLabel, async () => {
  if (!atBottom.value) return;
  await nextTick();
  scrollToBottom();
});

onMounted(async () => {
  await nextTick();
  scrollToBottom();
  onScroll();
});

defineExpose({ scrollToBottom });
</script>

<template>
  <div class="msglist">
    <div ref="scrollEl" class="msglist__scroll" @scroll="onScroll">
      <div v-if="!messages.length" class="msglist__empty">
        <Icon name="chat" :size="40" />
        <p>No messages yet.</p>
      </div>

      <template v-for="row in rows" :key="row.message.id">
        <div v-if="row.divider" class="msglist__divider">
          <span>{{ row.divider }}</span>
        </div>
        <MessageBubble :message="row.message" :continuation="row.continuation" />
      </template>

      <div v-if="typingLabel" class="msglist__typing">
        <span class="msglist__typing-dots"><i /><i /><i /></span>
        {{ typingLabel }}
      </div>
    </div>

    <Transition name="scale">
      <button
        v-if="!atBottom"
        class="msglist__jump"
        aria-label="Jump to latest"
        @click="scrollToBottom(true)"
      >
        <Icon name="arrow-down" :size="22" />
      </button>
    </Transition>
  </div>
</template>
