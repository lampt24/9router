// Adds a "Copy" button to every <div class="code"><pre>…</pre></div> block.
document.addEventListener("DOMContentLoaded", () => {
  document.querySelectorAll(".code").forEach((box) => {
    const pre = box.querySelector("pre");
    if (!pre || box.querySelector(".copy")) return;
    const b = document.createElement("button");
    b.type = "button"; b.className = "copy"; b.textContent = "Copy";
    b.addEventListener("click", async () => {
      const text = pre.innerText.replace(/\n$/, "");
      try { await navigator.clipboard.writeText(text); }
      catch {
        const r = document.createRange(); r.selectNodeContents(pre);
        const s = getSelection(); s.removeAllRanges(); s.addRange(r); document.execCommand("copy"); s.removeAllRanges();
      }
      b.textContent = "Copied"; setTimeout(() => { b.textContent = "Copy"; }, 1400);
    });
    box.appendChild(b);
  });
});
