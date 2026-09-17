<script setup lang="ts">
import { computed } from "vue";
import { getFlagImageURL } from "@/utils/nodeFlags";

const props = defineProps<{ text: string }>();
const parts = computed(() =>
  props.text.split(/(\p{Regional_Indicator}{2})/u).map((text) => ({
    text,
    flag: /^\p{Regional_Indicator}{2}$/u.test(text),
  })),
);
</script>

<template>
  <span>
    <template v-for="(part, index) in parts" :key="index">
      <img
        v-if="part.flag"
        :src="getFlagImageURL(part.text)"
        :alt="part.text"
        class="inline-block h-[1em] w-[1em] align-[-0.125em] object-contain"
      />
      <template v-else>{{ part.text }}</template>
    </template>
  </span>
</template>
