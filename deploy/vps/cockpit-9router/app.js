/* global cockpit */
document.addEventListener("DOMContentLoaded", () => {
  const $ = (id) => document.getElementById(id);
  const run = (args, opts = {}) => cockpit.spawn(args, { err: "message", ...opts });
  const pill = (el, text, state) => { el.textContent = text; el.className = "pill" + (state ? " " + state : ""); };

  function health() {
    run(["curl", "-s", "-m", "5", "http://127.0.0.1:20128/api/health"])
      .then((o) => (o.includes('"ok":true') ? pill($("health"), "Hoạt động", "ok") : pill($("health"), "Lỗi: " + o.trim(), "bad")))
      .catch(() => pill($("health"), "Không phản hồi", "bad"));
  }

  function timer() {
    run(["systemctl", "show", "9router-update.timer", "-p", "ActiveState", "-p", "NextElapseUSecRealtime", "-p", "LastTriggerUSec"])
      .then((o) => {
        const v = Object.fromEntries(o.trim().split("\n").map((l) => [l.split("=")[0], l.slice(l.indexOf("=") + 1)]));
        if (v.ActiveState === "active") pill($("timer"), "Đang bật", "ok"); else pill($("timer"), v.ActiveState || "không rõ", "bad");
        $("timer-detail").textContent = "Lần trước: " + (v.LastTriggerUSec || "—") + " · Lần tới: " + (v.NextElapseUSecRealtime || "—");
      })
      .catch(() => pill($("timer"), "Không đọc được", "bad"));
  }

  function containers() {
    run(["docker", "ps", "-a", "--format", "{{.Names}}\t{{.Status}}\t{{.Image}}"], { superuser: "require" })
      .then((o) => {
        $("admin-hint").hidden = true;
        const rows = o.trim().split("\n").filter(Boolean).map((l) => l.split("\t"));
        const tb = $("containers"); tb.textContent = "";
        for (const [name, status, image] of rows) {
          const tr = document.createElement("tr");
          const up = /^Up/.test(status);
          const unhealthy = /unhealthy/.test(status);
          const td1 = document.createElement("td"); td1.textContent = name;
          const td2 = document.createElement("td"); const p = document.createElement("span");
          pill(p, status, up && !unhealthy ? "ok" : "bad"); td2.appendChild(p);
          const td3 = document.createElement("td"); const c = document.createElement("code"); c.textContent = image; td3.appendChild(c);
          tr.append(td1, td2, td3); tb.appendChild(tr);
          if (name === "9router") $("version").textContent = "Image: " + image;
          if (name === "cloudflared") (up ? pill($("tunnel"), "Đã kết nối", "ok") : pill($("tunnel"), status, "bad"));
        }
        return run(["sh", "-c", "docker logs cloudflared --since 24h 2>&1 | grep -c 'Registered tunnel connection'"], { superuser: "require" })
          .then((n) => { $("tunnel-detail").textContent = "Lượt đăng ký kết nối trong 24h: " + n.trim(); })
          .catch(() => {});
      })
      .catch(() => {
        $("admin-hint").hidden = false;
        $("containers").innerHTML = '<tr><td colspan="3" class="muted">Cần quyền quản trị</td></tr>';
        pill($("tunnel"), "Cần quyền quản trị");
      });
  }

  // cockpit.spawn with superuser:"require" fails with problem "access-denied" while the
  // session is in "Limited access" mode; tell the user how to fix it.
  const ADMIN_HINT = "Cần quyền quản trị: bấm \"Limited access\" ở góc trên bên phải → \"Turn on administrative access\" → nhập mật khẩu vpsadmin, rồi thử lại.";
  const errText = (e) => (e && e.problem === "access-denied" ? ADMIN_HINT : "Lỗi: " + ((e && e.message) || e));

  function refresh() { health(); timer(); containers(); }

  $("refresh").addEventListener("click", refresh);
  $("update").addEventListener("click", () => {
    const b = $("update"); b.disabled = true;
    $("out").hidden = false; $("out").textContent = "Đang chạy 9router-update…";
    run(["systemctl", "start", "9router-update.service"], { superuser: "require" })
      .then(() => run(["journalctl", "-u", "9router-update", "-n", "8", "--no-pager", "-o", "cat"], { superuser: "require" }))
      .then((o) => { $("out").textContent = o; refresh(); })
      .catch((e) => { $("out").textContent = errText(e); })
      .finally(() => { b.disabled = false; });
  });

  refresh();
  setInterval(health, 30000);
});
