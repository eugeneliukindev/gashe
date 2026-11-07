// A click on a diagram, or Enter on it, shows it scaled to the window; a click or Escape closes it.
// The theme draws diagrams in a closed shadow root, so the whole block is scaled, not the SVG.
function closeZoom() {
  const open = document.querySelector(".mermaid.zoomed");
  if (open) {
    open.classList.remove("zoomed");
    open.style.removeProperty("--zoom");
    open.style.removeProperty("width");
    open.focus();
  }
  return open;
}
function zoom(d) {
  const r = d.getBoundingClientRect();
  d.style.width = `${r.width}px`;
  d.style.setProperty("--zoom", Math.min((innerWidth * 0.95) / r.width, (innerHeight * 0.95) / r.height, 3));
  d.classList.add("zoomed");
}
document.addEventListener("click", (e) => {
  if (closeZoom()) return;
  const d = e.target.closest(".mermaid");
  if (d) zoom(d);
});
document.addEventListener("keydown", (e) => {
  if (e.key === "Escape") closeZoom();
  else if ((e.key === "Enter" || e.key === " ") && e.target.matches?.(".mermaid:not(.zoomed)")) {
    e.preventDefault();
    zoom(e.target);
  }
});
// The theme puts each drawn diagram in a new element, also on pages it loads without a reload:
// each one takes focus once it appears.
new MutationObserver(() => {
  for (const d of document.querySelectorAll(".mermaid:not([tabindex])")) {
    d.tabIndex = 0;
    d.setAttribute("role", "button");
    d.setAttribute("aria-label", "Zoom the diagram");
  }
}).observe(document.body, { childList: true, subtree: true });
