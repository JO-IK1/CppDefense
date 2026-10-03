const csrf = () => decodeURIComponent((document.cookie.match(/(?:^|; )cppdefense_csrf=([^;]*)/) || [])[1] || "");
const uuid = () => crypto.randomUUID();
const themeKey = "cppdefense_theme";
const preferredTheme = () => localStorage.getItem(themeKey) || (matchMedia("(prefers-color-scheme: light)").matches ? "light" : "dark");

function applyTheme(theme) {
  document.documentElement.dataset.theme = theme;
  const button = document.querySelector("#theme-toggle");
  if (button) {
    button.textContent = theme === "dark" ? "☀" : "☾";
    button.title = theme === "dark" ? "Включить светлую тему" : "Включить тёмную тему";
  }
}

applyTheme(preferredTheme());
document.querySelector("#theme-toggle")?.addEventListener("click", () => {
  const next = document.documentElement.dataset.theme === "dark" ? "light" : "dark";
  localStorage.setItem(themeKey, next);
  applyTheme(next);
});

window.addEventListener("unhandledrejection", event => {
  event.preventDefault();
  alert(event.reason?.message || "Операция не выполнена");
});

async function api(url, options = {}) {
  options.headers = { ...(options.headers || {}), "X-CSRF-Token": csrf(), "Idempotency-Key": options.idempotency || uuid() };
  const response = await fetch(url, options);
  if (!response.ok) throw new Error((await response.json().catch(() => ({ title: response.statusText }))).title);
  if (response.status === 204) return null;
  return response.json();
}

function node(tag, className, text) {
  const value = document.createElement(tag);
  if (className) value.className = className;
  if (text !== undefined) value.textContent = text;
  return value;
}

document.querySelector('[data-action="logout"]')?.addEventListener("click", async () => {
  await api("/api/v1/auth/logout", { method: "POST" });
  location.href = "/";
});

document.querySelectorAll("[data-select-group]").forEach(button => {
  button.onclick = () => document.querySelectorAll('[name="group_id"]').forEach(input => {
    input.value = button.dataset.selectGroup;
  });
});

document.querySelector("#group-form")?.addEventListener("submit", async event => {
  event.preventDefault();
  const form = new FormData(event.target);
  await api("/api/v1/groups", { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ code: form.get("code"), name: form.get("name"), is_demo: form.get("is_demo") === "on" }) });
  location.reload();
});

document.querySelector("#lab-form")?.addEventListener("submit", async event => {
  event.preventDefault();
  const form = new FormData(event.target);
  await api(`/api/v1/groups/${form.get("group_id")}/labs`, { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ code: form.get("code"), name: form.get("name"), time_limit_seconds: +form.get("time_limit_seconds"), top_n: +form.get("top_n") }) });
  alert("Лабораторная создана");
});

document.querySelector("#import-form")?.addEventListener("submit", async event => {
  event.preventDefault();
  const form = new FormData(event.target);
  const payload = new FormData();
  payload.set("group_id", form.get("group_id"));
  payload.set("kind", form.get("kind"));
  payload.set("archive", form.get("archive"));
  const value = await api("/api/v1/imports", { method: "POST", body: payload });
  document.querySelector("#import-result").textContent = JSON.stringify(value, null, 2);
  document.querySelector('#review-form [name="import_id"]').value = value.id;
  document.querySelector('#review-form [name="archive_sha256"]').value = value.original_sha256;
});

document.querySelector("#review-form")?.addEventListener("submit", async event => {
  event.preventDefault();
  const form = new FormData(event.target);
  const id = form.get("import_id");
  const value = await api(`/api/v1/imports/${id}/review`, { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ decision: form.get("decision"), archive_sha256: form.get("archive_sha256"), checklist: { paths_checked: form.get("paths_checked") === "on", ownership_checked: form.get("ownership_checked") === "on" }, reason: form.get("reason") }) });
  document.querySelector("#import-result").textContent = JSON.stringify(value, null, 2);
});

document.querySelector("#apply-import")?.addEventListener("click", async () => {
  const id = document.querySelector('#review-form [name="import_id"]').value;
  document.querySelector("#import-result").textContent = JSON.stringify(await api(`/api/v1/imports/${id}/apply`, { method: "POST" }), null, 2);
});

let defenseID;
let draftVersion = 1;
let defensePoll;
let defenseRefreshInFlight = false;
let repositoryLoaded = false;
let wheelCandidates = [];
let wheelSpinAnimation;
let wheelSettleAnimation;

async function openRepositoryFile(filePath) {
  const value = await api(`/api/v1/defenses/${defenseID}/repository/file?path=${encodeURIComponent(filePath)}`);
  document.querySelector("#source-title").textContent = value.path + (value.masked ? " · функция скрыта" : "");
  document.querySelector("#source-code code").textContent = value.content;
  document.querySelectorAll("#repository-files button").forEach(button => button.classList.toggle("is-active", button.dataset.path === filePath));
}

async function loadRepository(selectedPath) {
  if (repositoryLoaded || !defenseID) return;
  const value = await api(`/api/v1/defenses/${defenseID}/repository`);
  const root = document.querySelector("#repository-files");
  root.replaceChildren(...value.items.map(item => {
    const button = node("button", "repository-file", item.path);
    button.type = "button";
    button.dataset.path = item.path;
    button.title = `${item.path} · ${item.size} байт`;
    button.onclick = () => openRepositoryFile(item.path);
    return button;
  }));
  repositoryLoaded = true;
  if (selectedPath) await openRepositoryFile(selectedPath);
}

function wheelRotation(element) {
  const transform = getComputedStyle(element).transform;
  if (!transform || transform === "none") return 0;
  const values = transform.match(/^matrix\(([^)]+)\)$/)?.[1].split(",").map(Number);
  if (!values || values.length < 2) return 0;
  const angle = Math.atan2(values[1], values[0]) * 180 / Math.PI;
  return angle < 0 ? angle + 360 : angle;
}

function positionWheelLabels(finalRotation = 0) {
  const count = wheelCandidates.length;
  if (!count) return;
  const step = 360 / count;
  document.querySelectorAll("#function-wheel .wheel-option").forEach((option, index) => {
    const angle = step * index;
    option.style.transform = `translate(-50%,-50%) rotate(${angle}deg) translateY(-112px) rotate(${-angle - finalRotation}deg)`;
  });
}

function startWheel() {
  const wheel = document.querySelector("#function-wheel");
  const label = document.querySelector("#wheel-label");
  if (!wheel) return;
  wheelSpinAnimation?.cancel();
  wheelSettleAnimation?.cancel();
  delete wheel.dataset.selectedId;
  wheel.style.transform = "rotate(0deg)";
  wheel.classList.remove("is-selected");
  label.textContent = "Выбираем функцию…";
  if (!matchMedia("(prefers-reduced-motion: reduce)").matches) {
    wheelSpinAnimation = wheel.animate([{ transform: "rotate(0deg)" }, { transform: "rotate(360deg)" }], { duration: 900, iterations: Infinity, easing: "linear" });
  }
}

function renderWheelCandidates(candidates = []) {
  const wheel = document.querySelector("#function-wheel");
  if (!wheel) return;
  candidates = Array.isArray(candidates) ? candidates : [];
  const fingerprint = candidates.map(candidate => `${candidate.id}:${candidate.signature}`).join("|");
  if (wheel.dataset.candidates === fingerprint) return;
  wheel.dataset.candidates = fingerprint;
  wheelCandidates = candidates;
  wheel.querySelectorAll(".wheel-option").forEach(option => option.remove());
  if (!candidates.length) {
    wheel.style.removeProperty("background");
    return;
  }
  const colors = ["#ff6262", "#ffb443", "#f5e642", "#85dc66", "#43cdd9", "#668aff", "#ad6cf0", "#f064ad"];
  const step = 360 / candidates.length;
  wheel.style.background = `conic-gradient(from -${step / 2}deg,${candidates.map((_, index) => `${colors[index % colors.length]} ${index * step}deg ${(index + 1) * step}deg`).join(",")})`;
  candidates.forEach(candidate => {
    const option = node("span", "wheel-option", candidate.function_name);
    option.title = candidate.signature;
    wheel.append(option);
  });
  positionWheelLabels();
}

function settleWheel(selectedID, challenge) {
  const wheel = document.querySelector("#function-wheel");
  const label = document.querySelector("#wheel-label");
  if (!wheel || !wheelCandidates.length || wheel.dataset.selectedId === selectedID) return;
  let selectedIndex = wheelCandidates.findIndex(candidate => candidate.id === selectedID);
  if (selectedIndex < 0) selectedIndex = wheelCandidates.findIndex(candidate => candidate.signature === challenge.signature && candidate.file_path === challenge.file_path);
  if (selectedIndex < 0) return;

  const current = wheelRotation(wheel);
  wheelSpinAnimation?.cancel();
  wheel.dataset.selectedId = selectedID;
  const step = 360 / wheelCandidates.length;
  const finalRotation = -selectedIndex * step;
  const normalizedCurrent = ((current % 360) + 360) % 360;
  const normalizedFinal = ((finalRotation % 360) + 360) % 360;
  const forward = (normalizedFinal - normalizedCurrent + 360) % 360;
  const reduced = matchMedia("(prefers-reduced-motion: reduce)").matches;
  const target = current + (reduced ? 0 : 4 * 360) + forward;
  wheelSettleAnimation = wheel.animate([{ transform: `rotate(${current}deg)` }, { transform: `rotate(${target}deg)` }], { duration: reduced ? 1 : 2200, easing: "cubic-bezier(.12,.72,.18,1)", fill: "forwards" });
  wheelSettleAnimation.finished.then(() => {
    wheel.style.transform = `rotate(${finalRotation}deg)`;
    wheelSettleAnimation.cancel();
    positionWheelLabels(finalRotation);
    wheel.classList.add("is-selected");
    label.textContent = challenge.function_name;
    label.title = challenge.signature;
  }).catch(() => {});
}

const defenseStatusLabels = { ready: "Ожидает подтверждения преподавателя", preparing: "Подготовка задания…", active: "Защита идёт", passed: "Защита успешно сдана", failed: "Защита не пройдена", error: "Ошибка подготовки защиты", expired: "Время защиты истекло", cancelled: "Защита отменена" };

document.querySelectorAll("[data-start-defense]").forEach(button => {
  button.onclick = async () => {
    document.querySelector("#defense").classList.remove("hidden");
    document.querySelector("#defense").scrollIntoView({ behavior: "smooth", block: "start" });
    startWheel();
    repositoryLoaded = false;
    document.querySelector("#result")?.classList.add("hidden");
    const value = await api("/api/v1/defenses", { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ submission_version_id: button.dataset.startDefense }) });
    defenseID = value.id;
    stopDefensePolling();
    await pollDefense();
  };
});

async function refreshDefense() {
  if (!defenseID || defenseRefreshInFlight) return false;
  defenseRefreshInFlight = true;
  try {
    const value = await api(`/api/v1/defenses/${defenseID}`);
    const status = document.querySelector("#defense-status");
    const timer = document.querySelector("#timer");
    draftVersion = value.draft_version || draftVersion;
    renderWheelCandidates(value.wheel_candidates);
    if (value.current_draft && !document.querySelector("#answer").value) document.querySelector("#answer").value = value.current_draft;
    status.textContent = defenseStatusLabels[value.status] || value.status;
    if (value.challenge && value.status === "active") {
      settleWheel(value.selected_candidate_id, value.challenge);
      status.textContent = `${value.challenge.signature} · строки ${value.challenge.begin_line}–${value.challenge.end_line}`;
      if (!repositoryLoaded) {
        document.querySelector("#source-code code").textContent = value.challenge.masked_source || "Замаскированный исходник недоступен";
        await loadRepository(value.challenge.file_path);
      }
    }
    if (value.status === "active" && value.deadline_at) {
      const left = Math.max(0, Math.floor((Date.parse(value.deadline_at) - Date.now()) / 1000));
      timer.textContent = `Осталось ${Math.floor(left / 60)}:${String(left % 60).padStart(2, "0")}`;
    } else timer.textContent = "";
    return ["passed", "failed", "error", "expired", "cancelled"].includes(value.status);
  } finally {
    defenseRefreshInFlight = false;
  }
}

function stopDefensePolling() {
  clearTimeout(defensePoll);
  defensePoll = undefined;
}

async function pollDefense() {
  stopDefensePolling();
  try {
    if (await refreshDefense()) return;
  } catch (error) {
    const status = document.querySelector("#defense-status");
    if (status) status.textContent = `Не удалось обновить защиту: ${error.message}. Повторяем…`;
  }
  defensePoll = setTimeout(pollDefense, 1500);
}

document.querySelector("#save-draft")?.addEventListener("click", async () => {
  const value = await api(`/api/v1/defenses/${defenseID}/draft`, { method: "PUT", headers: { "Content-Type": "application/json", "If-Match": `"${draftVersion}"` }, body: JSON.stringify({ answer: document.querySelector("#answer").value }) });
  draftVersion = value.version;
});

const attemptOutcome = {
  pending: { label: "В очереди", title: "Проверка выполняется", tone: "pending" },
  passed: { label: "Успешно", title: "Защита успешно сдана", tone: "success" },
  failed: { label: "Не пройдено", title: "Проверка не пройдена", tone: "danger" },
  error: { label: "Ошибка системы", title: "Проверка не выполнена", tone: "danger" },
};
const attemptStages = [["configure_result", "Конфигурация CMake"], ["build_result", "Сборка проекта"], ["ctest_result", "Тесты CTest"]];

function createAttemptReport(attempt, compact = false) {
  const meta = attemptOutcome[attempt.outcome] || { label: attempt.outcome, title: "Результат проверки", tone: "pending" };
  const report = node("article", `attempt-report attempt-report--${meta.tone}${compact ? " attempt-report--compact" : ""}`);
  const header = node("header", "attempt-report__header");
  const heading = node("div");
  heading.append(node("span", `attempt-badge attempt-badge--${meta.tone}`, meta.label), node("h3", "", meta.title));
  heading.append(node("p", "attempt-report__meta", `Попытка №${attempt.attempt_no} · ${new Date(attempt.accepted_at).toLocaleString("ru-RU")}`));
  header.append(heading);
  if (compact) header.append(node("code", "attempt-report__id", attempt.defense_id));
  report.append(header);

  const stages = node("div", "attempt-stages");
  for (const [key, label] of attemptStages) {
    const result = attempt[key];
    const state = !result ? "waiting" : result.ok ? "success" : "danger";
    const card = node("div", `attempt-stage attempt-stage--${state}`);
    card.append(node("span", "attempt-stage__mark", state === "success" ? "✓" : state === "danger" ? "×" : "…"));
    const copy = node("div");
    copy.append(node("strong", "", label), node("span", "", !result ? "Ожидание" : result.ok ? "OK" : `Ошибка · код ${result.exit_code}`));
    card.append(copy);
    stages.append(card);
  }
  report.append(stages);

  const rawLogs = attemptStages.map(([key]) => attempt[key]?.log?.trim()).filter(Boolean);
  const logs = new Set(rawLogs).size === 1 && rawLogs.length ? [rawLogs[0]] : [...new Set(attemptStages.map(([key, label]) => attempt[key]?.log?.trim() ? `${label}\n${attempt[key].log.trim()}` : "").filter(Boolean))];
  if (logs.length) {
    const terminal = node("details", "attempt-terminal");
    terminal.open = attempt.outcome !== "passed";
    terminal.append(node("summary", "", "Лог компиляции и тестов"), node("pre", "attempt-terminal__log", logs.join("\n\n")));
    report.append(terminal);
  }

  const verdict = attempt.outcome === "passed" ? "Все этапы завершены успешно. Лабораторная защищена." : attempt.outcome === "pending" ? "Runner получил задание. Дождитесь завершения проверки." : attempt.outcome === "error" ? "Произошла ошибка инфраструктуры. Ответ студента не должен считаться неверным." : "В сборке или тестах есть ошибка. Изучите терминальный лог и исправьте ответ.";
  report.append(node("p", `attempt-verdict attempt-verdict--${meta.tone}`, verdict));
  return report;
}

function renderAttemptResult(attempt) {
  const root = document.querySelector("#result");
  if (!root) return;
  root.replaceChildren(createAttemptReport(attempt));
  root.classList.remove("hidden");
}

document.querySelector("#submit-attempt")?.addEventListener("click", async event => {
  const button = event.currentTarget;
  button.disabled = true;
  button.textContent = "Проверяем…";
  try {
    const value = await api(`/api/v1/defenses/${defenseID}/attempts`, { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ answer: document.querySelector("#answer").value }) });
    renderAttemptResult(value);
    await pollAttempt(value.id);
  } finally {
    button.disabled = false;
    button.textContent = "Отправить на проверку";
  }
});

document.querySelector("#cancel-defense")?.addEventListener("click", async () => {
  if (!defenseID || !confirm("Отменить активную защиту?")) return;
  await api(`/api/v1/defenses/${defenseID}/cancel`, { method: "POST" });
  stopDefensePolling();
  await refreshDefense();
});

const workspaceSplitKey = "cppdefense_workspace_split";

function setWorkspaceSplit(workspace, percent) {
  const value = Math.min(70, Math.max(22, Number(percent) || 33));
  workspace.style.setProperty("--source-pane-width", `${value}%`);
  workspace.querySelector("#workspace-resizer")?.setAttribute("aria-valuenow", String(Math.round(value)));
  localStorage.setItem(workspaceSplitKey, String(value));
}

function initializeWorkspaceResizer() {
  const workspace = document.querySelector(".code-workspace");
  const resizer = document.querySelector("#workspace-resizer");
  if (!workspace || !resizer) return;
  setWorkspaceSplit(workspace, localStorage.getItem(workspaceSplitKey) || 33);

  const resizeFromPointer = event => {
    const bounds = workspace.getBoundingClientRect();
    setWorkspaceSplit(workspace, (event.clientX - bounds.left) / bounds.width * 100);
  };
  resizer.addEventListener("pointerdown", event => {
    resizer.setPointerCapture(event.pointerId);
    workspace.classList.add("is-resizing");
    resizeFromPointer(event);
  });
  resizer.addEventListener("pointermove", event => {
    if (resizer.hasPointerCapture(event.pointerId)) resizeFromPointer(event);
  });
  const finishResize = event => {
    if (resizer.hasPointerCapture(event.pointerId)) resizer.releasePointerCapture(event.pointerId);
    workspace.classList.remove("is-resizing");
  };
  resizer.addEventListener("pointerup", finishResize);
  resizer.addEventListener("pointercancel", finishResize);
  resizer.addEventListener("dblclick", () => setWorkspaceSplit(workspace, 33));
  resizer.addEventListener("keydown", event => {
    const current = Number(resizer.getAttribute("aria-valuenow")) || 33;
    if (event.key === "ArrowLeft") setWorkspaceSplit(workspace, current - 2);
    else if (event.key === "ArrowRight") setWorkspaceSplit(workspace, current + 2);
    else if (event.key === "Home") setWorkspaceSplit(workspace, 22);
    else if (event.key === "End") setWorkspaceSplit(workspace, 70);
    else return;
    event.preventDefault();
  });
}

initializeWorkspaceResizer();

async function pollAttempt(id) {
  for (let count = 0; count < 120; count++) {
    await new Promise(resolve => setTimeout(resolve, 1500));
    const value = await api(`/api/v1/attempts/${id}`);
    renderAttemptResult(value);
    if (value.outcome !== "pending") {
      await refreshDefense();
      return;
    }
  }
  throw new Error("Проверка выполняется слишком долго. Обновите страницу и откройте историю попыток.");
}

document.querySelector("#load-history")?.addEventListener("click", async () => {
  const value = await api("/api/v1/history/attempts?limit=100");
  const root = document.querySelector("#history-result");
  if (!value.items.length) {
    root.replaceChildren(node("div", "empty-state", "История проверок пока пуста"));
    return;
  }
  root.replaceChildren(...value.items.map(attempt => createAttemptReport(attempt, true)));
});

document.querySelector("#load-pending-defenses")?.addEventListener("click", async () => {
  const value = await api("/api/v1/teacher/defenses/pending");
  const root = document.querySelector("#pending-defenses");
  root.replaceChildren(...value.items.map(defense => {
    const card = node("article", "card pending-defense");
    card.append(node("h3", "", `${defense.student_login} · ${defense.lab_code} · ${defense.lab_name}`));
    const list = node("div", "candidate-list");
    defense.wheel_candidates.forEach(candidate => {
      const button = node("button", "secondary-button", `${candidate.function_name} · ${candidate.file_path} · ${candidate.line_count} строк`);
      button.type = "button";
      button.title = candidate.signature;
      button.onclick = () => {
        document.querySelector('#configure-defense-form [name="defense_id"]').value = defense.id;
        document.querySelector('#configure-defense-form [name="candidate_id"]').value = candidate.id;
        document.querySelector('#configure-defense-form [name="selection_mode"]').value = "manual";
      };
      list.append(button);
    });
    card.append(list);
    return card;
  }));
});

document.querySelector("#configure-defense-form")?.addEventListener("submit", async event => {
  event.preventDefault();
  const form = new FormData(event.target);
  const id = form.get("defense_id");
  const mode = form.get("selection_mode");
  await api(`/api/v1/defenses/${id}/configure`, { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ selection_mode: mode, candidate_id: mode === "manual" ? form.get("candidate_id") : "", time_limit_seconds: +form.get("time_limit_seconds") }) });
  alert("Настройки сохранены. Подготовка защиты запущена");
  document.querySelector("#load-pending-defenses").click();
});

document.querySelector("#submissions-form")?.addEventListener("submit", async event => {
  event.preventDefault();
  const form = new FormData(event.target);
  const value = await api(`/api/v1/groups/${form.get("group_id")}/submissions`);
  const root = document.querySelector("#submissions-result");
  root.replaceChildren(...value.items.map(item => {
    const row = node("p");
    const link = node("a", "", `${item.github_login} · ${item.lab_code} · версия ${item.version_number}`);
    link.href = `/api/v1/submissions/${item.id}/archive`;
    row.append(link);
    return row;
  }));
});

document.querySelector("#load-pending")?.addEventListener("click", async () => {
  document.querySelector("#pending-result").textContent = JSON.stringify(await api("/api/v1/admin/pending-links"), null, 2);
});

document.querySelector("#approve-link-form")?.addEventListener("submit", async event => {
  event.preventDefault();
  const form = new FormData(event.target);
  await api(`/api/v1/admin/pending-links/${form.get("user_id")}/approve`, { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ student_record_id: form.get("student_record_id"), reason: "Подтверждено через кабинет" }) });
  alert("Студент привязан");
});

document.querySelector("#load-users")?.addEventListener("click", async () => {
  document.querySelector("#users-result").textContent = JSON.stringify(await api("/api/v1/admin/users"), null, 2);
});

document.querySelector("#role-form")?.addEventListener("submit", async event => {
  event.preventDefault();
  const form = new FormData(event.target);
  await api(`/api/v1/admin/users/${form.get("user_id")}/role`, { method: "PUT", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ role: form.get("role") }) });
  alert("Роль обновлена");
});

document.querySelector("#teacher-form")?.addEventListener("submit", async event => {
  event.preventDefault();
  const form = new FormData(event.target);
  await api(`/api/v1/admin/groups/${form.get("group_id")}/teachers`, { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ user_id: form.get("user_id") }) });
  alert("Преподаватель назначен");
});

document.querySelector("#view-form")?.addEventListener("submit", async event => {
  event.preventDefault();
  const form = new FormData(event.target);
  const role = form.get("role");
  const target = form.get("target_id");
  const body = { role };
  if (role === "teacher") body.group_id = target;
  if (role === "student") body.student_record_id = target;
  await api("/api/v1/admin/view-as-role", { method: "PUT", headers: { "Content-Type": "application/json" }, body: JSON.stringify(body) });
  location.reload();
});

document.querySelector("#clear-view")?.addEventListener("click", async () => {
  await api("/api/v1/admin/view-as-role", { method: "DELETE" });
  location.reload();
});
