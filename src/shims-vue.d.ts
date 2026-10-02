// Lets plain `tsc` resolve .vue imports; vue-tsc reads the real component types.
declare module '*.vue' {
  import type { DefineComponent } from 'vue'
  const component: DefineComponent<object, object, unknown>
  export default component
}
