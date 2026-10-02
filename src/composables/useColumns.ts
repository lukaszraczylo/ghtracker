import { onBeforeUnmount, onMounted, ref, type Ref } from 'vue'

/** 1 column below the breakpoint, 2 at or above it; follows the viewport live. */
export function useColumns(minWidth = '(min-width: 96rem)'): Ref<number> {
  const columns = ref(1)
  let query: MediaQueryList | null = null
  const update = () => {
    columns.value = query?.matches ? 2 : 1
  }
  onMounted(() => {
    if (typeof window.matchMedia !== 'function') return
    query = window.matchMedia(minWidth)
    update()
    query.addEventListener('change', update)
  })
  onBeforeUnmount(() => query?.removeEventListener('change', update))
  return columns
}
