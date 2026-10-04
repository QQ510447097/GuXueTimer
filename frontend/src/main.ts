import { createApp } from "vue";
import App from "./App.vue";

// global.css 放设计令牌与基础重置，glass.css 放玻璃质感的原子类。
// 顺序有意义：令牌必须先定义好，原子类才引得到它们。
import "./styles/global.css";
import "./styles/glass.css";

createApp(App).mount("#app");
