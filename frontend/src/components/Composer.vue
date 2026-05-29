<script setup>
import { ref, computed, watch, nextTick, onMounted, onBeforeUnmount } from "vue";
import { useChatStore } from "../stores/chat";
import Icon from "./Icon.vue";

const props = defineProps({
  replyTo: { type: Object, default: null },
  editing: { type: Object, default: null },
});
const emit = defineEmits(["cancel-reply", "cancel-edit", "open-gif", "open-emoji"]);

const chat = useChatStore();

const text = ref("");
const pendingFiles = ref([]); //{ id, file, isImage, url }
const menuOpen = ref(false);
const fieldEl = ref(null);
const fileInput = ref(null);
const fieldScrolled = ref(false);

const isEditing = computed(() => !!props.editing);
const hasContent = computed(() => text.value.trim().length > 0 || pendingFiles.value.length > 0);

// ==== responsive ====
const vw = ref(window.innerWidth);
const isTouch = ref(false);
function onResize() {
  vw.value = window.innerWidth;
}

//emoji button stands alone only on a wide pointer device;
//on a narrow desktop it folds into "+ menu"; on touch its dropped (native keyboard)
const showEmojiBtn = computed(() => !isTouch.value && vw.value >= 640);
const menuItems = computed(() => {
  const items = [
    { key: "image", icon: "image", label: "Image" },
    { key: "file", icon: "file", label: "File" },
    { key: "gif", icon: "gif", label: "GIF" },
  ];
  if (!isTouch.value && vw.value < 640) items.push({ key: "emoji", icon: "smile", label: "Emoji" });
  return items;
});

// ==== edit / reply prefill + focus ====
watch(
  () => props.editing,
  (m) => {
    if (m) {
      text.value = m.content || "";
      focusField();
      nextTick(() => {
        grow();
        checkFieldScroll();
      });
    }
  },
  { immediate: true },
);
watch(
  () => props.replyTo,
  (m) => {
    if (m) focusField();
  },
);

function focusField() {
  nextTick(() => fieldEl.value?.focus());
}
function cancelContext() {
  isEditing.value ? emit("cancel-edit") : emit("cancel-reply");
}

// ==== auto-grow textarea ====
function grow() {
  const el = fieldEl.value;
  if (!el) return;
  el.style.height = "auto";
  el.style.height = Math.min(el.scrollHeight, 140) + "px";
}
//shadow at top of input once it hits max height and scrolls
function checkFieldScroll() {
  fieldScrolled.value = (fieldEl.value?.scrollTop ?? 0) > 0;
}
let lastTyping = 0;
function onInput() {
  grow();
  checkFieldScroll();
  const now = Date.now();
  if (now - lastTyping > 2000) {
    lastTyping = now;
    chat.sendTyping?.();
  }
}

// ==== + menu ====
function toggleMenu() {
  menuOpen.value = !menuOpen.value;
}
function onDocClick(e) {
  if (menuOpen.value && !e.target.closest(".composer__actions")) menuOpen.value = false;
}
function pick(item) {
  menuOpen.value = false;
  if (item.key === "gif") return emit("open-gif");
  if (item.key === "emoji") return emit("open-emoji");
  const accept = item.key === "image" ? "image/*" : "*/*";
  nextTick(() => {
    if (!fileInput.value) return;
    fileInput.value.accept = accept;
    fileInput.value.click();
  });
}
function onFiles(e) {
  for (const f of Array.from(e.target.files || [])) {
    const isImage = f.type.startsWith("image/");
    pendingFiles.value.push({
      id: `${Date.now()}-${Math.random()}`,
      file: f,
      isImage,
      url: isImage ? URL.createObjectURL(f) : null,
    });
  }
  e.target.value = "";
}
function removeFile(id) {
  const i = pendingFiles.value.findIndex((f) => f.id === id);
  if (i < 0) return;
  const f = pendingFiles.value[i];
  if (f.url) URL.revokeObjectURL(f.url);
  pendingFiles.value.splice(i, 1);
}

// ==== send ====
async function send() {
  const content = text.value.trim();
  if (!hasContent.value) return;

  if (isEditing.value) {
    await chat.editMessage(props.editing.id, content);
    emit("cancel-edit");
  } else {
    const files = pendingFiles.value.map((f) => f.file);
    await chat.sendMessage(content, files, props.replyTo?.id ?? null, null, null);
    if (props.replyTo) emit("cancel-reply");
  }

  text.value = "";
  pendingFiles.value.forEach((f) => f.url && URL.revokeObjectURL(f.url));
  pendingFiles.value = [];
  nextTick(() => {
    grow();
    checkFieldScroll();
  });
}

//enter sends on desktop; shift+enter newline; touch always newlines (use button)
function onKeydown(e) {
  if (e.key === "Enter" && !e.shiftKey && !isTouch.value) {
    e.preventDefault();
    send();
  }
}

onMounted(() => {
  isTouch.value = window.matchMedia("(pointer: coarse)").matches;
  window.addEventListener("resize", onResize);
  document.addEventListener("click", onDocClick);
});
onBeforeUnmount(() => {
  window.removeEventListener("resize", onResize);
  document.removeEventListener("click", onDocClick);
  pendingFiles.value.forEach((f) => f.url && URL.revokeObjectURL(f.url));
});
</script>

<template>
  <div
    class="composer"
    :class="{ 'composer--context': replyTo || editing, 'composer--has-send': hasContent }"
  >
    <!-- ==== reply / edit strip ==== -->
    <div v-if="replyTo || editing" class="composer__context">
      <span class="composer__context-bar" />
      <div class="composer__context-body">
        <span class="composer__context-title">
          <Icon :name="editing ? 'pencil' : 'reply'" :size="14" />
          {{ editing ? "Editing message" : replyTo.author_username }}
        </span>
        <span class="composer__context-text">{{
          editing ? editing.content : replyTo.content
        }}</span>
      </div>
      <button class="icon-btn composer__context-x" aria-label="Cancel" @click="cancelContext">
        <Icon name="close" :size="18" />
      </button>
    </div>

    <!-- ==== pending attachments ==== -->
    <div v-if="pendingFiles.length" class="composer__attachments">
      <div v-for="f in pendingFiles" :key="f.id" class="composer__attachment">
        <img v-if="f.isImage" :src="f.url" alt="" />
        <Icon v-else name="file" :size="22" />
        <button class="composer__attachment-x" aria-label="Remove" @click="removeFile(f.id)">
          <Icon name="close" :size="14" />
        </button>
      </div>
    </div>

    <!-- ==== bar ==== -->
    <div class="composer__row">
      <div class="composer__actions">
        <button
          class="icon-btn"
          :class="{ 'is-active': menuOpen }"
          aria-label="Add"
          @click.stop="toggleMenu"
        >
          <Icon name="plus" :size="24" />
        </button>
        <button v-if="showEmojiBtn" class="icon-btn" aria-label="Emoji" @click="emit('open-emoji')">
          <Icon name="smile" :size="24" />
        </button>

        <Transition name="scale">
          <div v-if="menuOpen" class="composer__menu">
            <button
              v-for="it in menuItems"
              :key="it.key"
              class="composer__menu-item"
              @click="pick(it)"
            >
              <Icon :name="it.icon" :size="20" />
              <span>{{ it.label }}</span>
            </button>
          </div>
        </Transition>
      </div>

      <div class="composer__field" :class="{ 'is-scrolled': fieldScrolled }">
        <textarea
          ref="fieldEl"
          v-model="text"
          class="composer__textarea"
          rows="1"
          maxlength="10000"
          :placeholder="editing ? 'Edit message…' : 'Write a new message…'"
          @input="onInput"
          @keydown="onKeydown"
          @scroll="checkFieldScroll"
        />
      </div>

      <Transition name="send">
        <button
          v-if="hasContent"
          class="composer__send"
          :aria-label="isEditing ? 'Save' : 'Send'"
          @pointerdown.prevent
          @click="send"
        >
          <Icon name="arrow-up" :size="24" />
        </button>
      </Transition>

      <input ref="fileInput" type="file" multiple hidden @change="onFiles" />
    </div>
  </div>
</template>
