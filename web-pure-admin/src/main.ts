import router from "./router";
import { createOaApp } from "./bootstrap";

import "./style/reset.scss";
import "./style/index.scss";
import "./style/tailwind.css";
import "element-plus/dist/index.css";
import "tippy.js/dist/tippy.css";
import "tippy.js/themes/light.css";
import "./oa/style.css";

const app = createOaApp();
void router.isReady().then(() => app.mount("#app"));
