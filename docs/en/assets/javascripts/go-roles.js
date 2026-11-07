// Pygments marks almost every Go identifier as one token, Name.Other (.nx). Give each one its
// role from the tokens around it, so calls and types get colors of their own.
function goRoles(root) {
  const sibling = (el, dir) => {
    let n = el[dir];
    while (n && (n.nodeType === 3 ? !n.textContent.trim() : n.classList.contains("w"))) n = n[dir];
    return n && n.nodeType === 1 ? n : null;
  };
  for (const id of root.querySelectorAll(".highlight .nx:not([data-role])")) {
    const next = sibling(id, "nextSibling");
    const prev = sibling(id, "previousSibling");
    const after = next && next.classList.contains("p") ? next.textContent : "";
    const before = prev && prev.classList.contains("p") ? prev.textContent : "";
    const exported = /^[A-Z]/.test(id.textContent);
    let role = "";
    if (after.startsWith("(")) role = "fn";
    else if (exported && (/^[\[{]/.test(after) || before.endsWith("[") || before.endsWith("."))) role = "type";
    id.dataset.role = role || "name";
  }
}
document$.subscribe(() => goRoles(document));
