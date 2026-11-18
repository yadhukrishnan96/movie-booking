import "./style.css";
import { App } from "./app";

const root = document.querySelector("#app");
if (!(root instanceof HTMLElement)) {
  throw new Error("missing #app");
}

const app = new App(root);
void app.start();
