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

  // ---- relay devices (argv passed directly to docker exec, never through a shell)
  const relayCmd = (...args) => run(["docker", "exec", "relay-server", "/9r-relay-server", ...args], { superuser: "require" });
  // cockpit.spawn with superuser:"require" fails with problem "access-denied" while the
  // session is in "Limited access" mode; tell the user how to fix it.
  const ADMIN_HINT = "Cần quyền quản trị: bấm \"Limited access\" ở góc trên bên phải → \"Turn on administrative access\" → nhập mật khẩu vpsadmin, rồi thử lại.";
  const errText = (e) => (e && e.problem === "access-denied" ? ADMIN_HINT : "Lỗi: " + ((e && e.message) || e));
  const fmt = (iso) => (iso ? new Date(iso).toLocaleString() : "");

  function devices() {
    relayCmd("devices", "--json")
      .then((o) => {
        const data = JSON.parse(o);
        const tb = $("devices"); tb.textContent = "";
        if (!data.devices.length) {
          const tr = document.createElement("tr"); const td = document.createElement("td");
          td.colSpan = 5; td.className = "muted"; td.textContent = "Chưa có thiết bị nào."; tr.appendChild(td); tb.appendChild(tr);
        }
        for (const d of data.devices) {
          const tr = document.createElement("tr");
          const cells = [d.name, d.id, fmt(d.created)].map((v) => { const td = document.createElement("td"); td.textContent = v; return td; });
          const st = document.createElement("td"); const p = document.createElement("span");
          pill(p, d.revoked ? "Đã thu hồi " + fmt(d.revoked) : "Hoạt động", d.revoked ? "bad" : "ok"); st.appendChild(p);
          const act = document.createElement("td");
          if (!d.revoked) {
            const b = document.createElement("button"); b.type = "button"; b.className = "btn"; b.textContent = "Thu hồi";
            b.addEventListener("click", () => {
              if (!window.confirm(`Thu hồi thiết bị "${d.name}" (${d.id})? Máy này sẽ mất kết nối và phải ghép đôi lại bằng mã mới.`)) return;
              b.disabled = true;
              relayCmd("revoke", d.id).then(devices).catch((e) => { window.alert(errText(e)); b.disabled = false; });
            });
            act.appendChild(b);
          }
          tr.append(...cells, st, act); tb.appendChild(tr);
        }
        $("pending").textContent = data.pending_invites ? `${data.pending_invites} mã ghép đôi chưa dùng (tự hết hạn sau 10 phút).` : "";
      })
      .catch(() => {
        $("devices").innerHTML = '<tr><td colspan="5" class="muted"></td></tr>'; $("devices").querySelector("td").textContent = ADMIN_HINT;
      });
  }

  $("pair-form").addEventListener("submit", (ev) => {
    ev.preventDefault();
    const name = $("pair-name").value.trim();
    if (!/^[A-Za-z0-9 ._-]{1,40}$/.test(name)) { window.alert("Tên chỉ gồm chữ, số, khoảng trắng, . _ -"); return; }
    $("pair-btn").disabled = true;
    relayCmd("pair-new", "--name", name, "--json")
      .then((o) => {
        const r = JSON.parse(o);
        $("pair-for").textContent = r.name;
        $("pair-exp").textContent = new Date(r.expires).toLocaleTimeString();
        $("pair-cmd").textContent = ".\\9r-relay-windows-amd64.exe install " + r.code;
        $("pair-result").hidden = false;
        devices();
      })
      .catch((e) => window.alert(errText(e)))
      .finally(() => { $("pair-btn").disabled = false; });
  });

  function refresh() { health(); timer(); containers(); devices(); }

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
