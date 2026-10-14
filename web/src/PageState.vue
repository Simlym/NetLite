<script setup lang="ts">
import {ref,watch,onUnmounted} from 'vue'
const props=defineProps<{loading:boolean;loaded:boolean;error?:string}>()
defineEmits<{retry:[]}>()
const showFeedback=ref(false)
let timer:ReturnType<typeof setTimeout>|undefined
watch(()=>props.loading,loading=>{clearTimeout(timer);showFeedback.value=false;if(loading)timer=setTimeout(()=>{showFeedback.value=true},180)},{immediate:true})
onUnmounted(()=>clearTimeout(timer))
</script>
<template>
 <div :class="['page-state',{'page-state-pending':loading&&!loaded}]" :aria-busy="loading">
  <div v-if="loading&&loaded&&showFeedback" class="refresh-progress" role="status" aria-label="正在更新数据"><span></span></div>
  <section v-if="loading&&!loaded&&showFeedback" class="card page-loading" aria-busy="true" aria-label="正在加载页面">
    <span class="sr-only" role="status">正在加载页面数据</span>
    <div class="page-skeleton" aria-hidden="true">
      <div class="skeleton-toolbar"><div><span class="skeleton-block skeleton-title"></span><span class="skeleton-block skeleton-description"></span></div><span class="skeleton-block skeleton-button"></span></div>
      <div class="skeleton-table"><div class="skeleton-table-head"><span class="skeleton-block"></span><span class="skeleton-block"></span><span class="skeleton-block"></span></div><div v-for="row in 4" :key="row" class="skeleton-table-row"><div><span class="skeleton-block"></span><span class="skeleton-block skeleton-secondary"></span></div><span class="skeleton-block"></span><span class="skeleton-block skeleton-status"></span></div></div>
    </div>
  </section>
  <section v-else-if="error&&!loaded" class="card page-load-error" role="alert">
    <h3>加载失败</h3><p>{{error}}</p><button class="secondary" @click="$emit('retry')">重新加载</button>
  </section>
  <div v-if="error&&loaded" class="refresh-error" role="alert"><span>刷新失败，当前显示上次数据。{{error}}</span><button class="text-button" @click="$emit('retry')">重试</button></div>
  <div v-show="loaded"><slot /></div>
 </div>
</template>
