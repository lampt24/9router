/* global cockpit */
document.addEventListener("DOMContentLoaded", () => {
  const $ = (id) => document.getElementById(id);
  const run = (args, opts = {}) => cockpit.spawn(args, { err: "out", ...opts });

  function refresh() {
    run(["curl", "-s", "-m", "5", "http://127.0.0.1:20128/api/health"])
      .then((o) => { $("health").textContent = o.trim(); $("health").className = o.includes('"ok":true') ? "ok" : "bad"; })
      .catch((e) => { $("health").textContent = "lỗi: " + e; $("health").className = "bad"; });
    run(["docker", "ps", "--format", "{{.Names}}\t{{.Status}}"], { superuser: "require" })
      .then((o) => { $("ps").textContent = o.trim(); })
      .catch((e) => { $("ps").textContent = "Cần bật Administrative access (góc trên bên phải): " + e.message; });
    run(["systemctl", "list-timers", "9router-update.timer", "--no-pager"])
      .then((o) => { $("timer").textContent = o.split("\n").slice(0, 2).join("\n"); })
      .catch((e) => { $("timer").textContent = String(e); });
  }

  $("refresh").addEventListener("click", refresh);
  $("update").addEventListener("click", () => {
    $("out").hidden = false;
    $("out").textContent = "Đang chạy 9router-update…";
    run(["systemctl", "start", "9router-update.service"], { superuser: "require" })
      .then(() => run(["journalctl", "-u", "9router-update", "-n", "8", "--no-pager", "-o", "cat"], { superuser: "require" }))
      .then((o) => { $("out").textContent = o; refresh(); })
      .catch((e) => { $("out").textContent = "Lỗi: " + e.message; });
  });
  refresh();
});
