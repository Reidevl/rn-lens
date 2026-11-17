import { StrictMode } from "react";
import { createRoot } from "react-dom/client";

import { App } from "./app/App";
import { detectBackend } from "./backend";
import "./styles/global.css";

const backend = await detectBackend();

createRoot(document.getElementById("root") as HTMLElement).render(
  <StrictMode>
    <App backend={backend} />
  </StrictMode>,
);
