import { createRoot } from "react-dom/client";

// The suites drive components by rendering into the same container again with
// new props, which under createRoot means reusing that container's root rather
// than making a second one. Keyed off the container so the callers keep reading
// as a plain render call.
const roots = new WeakMap();

const renderInto = (element, container) => {
  let root = roots.get(container);
  if (!root) {
    root = createRoot(container);
    roots.set(container, root);
  }
  root.render(element);
};

const unmountFrom = (container) => {
  const root = roots.get(container);
  if (root) {
    root.unmount();
    roots.delete(container);
  }
};

export { renderInto, unmountFrom };
