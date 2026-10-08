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

const entrySplash = document.querySelector("#entry-splash");
entrySplash?.addEventListener("animationend", () => entrySplash.remove(), { once: true });

window.addEventListener("unhandledrejection", event => {
  event.preventDefault();
  alert(event.reason?.message || "Операция не выполнена");
});

class ApiError extends Error {
  constructor(status, problem) {
    super(problem.title || `HTTP ${status}`);
    this.name = "ApiError";
    this.status = status;
    this.problem = problem;
  }
}

async function api(url, options = {}) {
  options.headers = { ...(options.headers || {}), "X-CSRF-Token": csrf(), "Idempotency-Key": options.idempotency || uuid() };
  const response = await fetch(url, options);
  if (!response.ok) {
    const problem = await response.json().catch(() => ({ title: response.statusText }));
    throw new ApiError(response.status, problem);
  }
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
  await api(`/api/v1/groups/${form.get("group_id")}/labs`, { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ code: form.get("code"), name: form.get("name"), description: form.get("description") }) });
  alert("Лабораторная создана");
});

function renderOperationResult(root, value, error = false) {
  root.replaceChildren();
  const card = node("div", `operation-card ${error ? "operation-card--error" : "operation-card--success"}`);
  card.append(node("strong", "", error ? "Операция не выполнена" : "Готово"));
  if (error) {
    const problem = value?.problem || {};
    card.append(node("p", "", problem.title || value.message || "Неизвестная ошибка"));
    const details = node("dl", "operation-details");
    [["Код", problem.code || `HTTP_${value.status || "ERROR"}`], ["Request ID", problem.request_id], ["Проблемный файл", problem.file]].forEach(([label, content]) => {
      if (!content) return;
      details.append(node("dt", "", label), node("dd", "", String(content)));
    });
    card.append(details);
  } else {
    card.append(node("p", "", `Импорт ${value.id || ""} загружен и проверен.`));
    if (value.original_sha256) card.append(node("code", "", value.original_sha256));
  }
  root.append(card);
}

async function renderImportOperation(root, operation) {
  try {
    const value = await operation();
    renderOperationResult(root, value);
    return value;
  } catch (error) {
    renderOperationResult(root, error, true);
    return null;
  }
}

document.querySelector("#import-form")?.addEventListener("submit", async event => {
  event.preventDefault();
  const form = new FormData(event.target);
  const payload = new FormData();
  payload.set("group_id", form.get("group_id"));
  payload.set("kind", form.get("kind"));
  payload.set("archive", form.get("archive"));
  const result = document.querySelector("#import-result");
  const value = await renderImportOperation(result, () => api("/api/v1/imports", { method: "POST", body: payload }));
  if (value) {
    document.querySelector('#review-form [name="import_id"]').value = value.id;
    document.querySelector('#review-form [name="archive_sha256"]').value = value.original_sha256;
  }
});

document.querySelector("#review-form")?.addEventListener("submit", async event => {
  event.preventDefault();
  const form = new FormData(event.target);
  const id = form.get("import_id");
  const result = document.querySelector("#import-result");
  await renderImportOperation(result, () => api(`/api/v1/imports/${id}/review`, { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ decision: form.get("decision"), archive_sha256: form.get("archive_sha256"), checklist: { paths_checked: form.get("paths_checked") === "on", ownership_checked: form.get("ownership_checked") === "on" }, reason: form.get("reason") }) }));
});

document.querySelector("#apply-import")?.addEventListener("click", async () => {
  const id = document.querySelector('#review-form [name="import_id"]').value;
  const result = document.querySelector("#import-result");
  await renderImportOperation(result, () => api(`/api/v1/imports/${id}/apply`, { method: "POST" }));
});

let defenseID;
let draftVersion = 1;
let defensePoll;
let defenseRefreshInFlight = false;
let repositoryLoaded = false;
let wheelCandidates = [];
let wheelSpinAnimation;
let wheelSettleAnimation;

const answerIndent = "    ";
const cppTokenPattern = /\/\*[\s\S]*?\*\/|\/\/[^\n]*|"(?:\\[\s\S]|[^"\\])*"|'(?:\\[\s\S]|[^'\\])*'|^\s*#\s*[A-Za-z_]\w*|\b(?:alignas|alignof|auto|bool|break|case|catch|char|class|concept|const|consteval|constexpr|constinit|const_cast|continue|co_await|co_return|co_yield|decltype|default|delete|do|double|dynamic_cast|else|enum|explicit|export|extern|false|float|for|friend|goto|if|inline|int|long|mutable|namespace|new|noexcept|nullptr|operator|override|private|protected|public|register|reinterpret_cast|requires|return|short|signed|sizeof|static|static_assert|static_cast|struct|switch|template|this|thread_local|throw|true|try|typedef|typeid|typename|union|unsigned|using|virtual|void|volatile|wchar_t|while)\b|\b(?:0[xX][\dA-Fa-f]+|0[bB][01]+|\d+(?:\.\d+)?(?:[eE][+-]?\d+)?)[uUlLfF]*\b/gm;

function escapeSyntaxText(value) {
  return value.replace(/[&<>]/g, character => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;" })[character]);
}

function escapeRegularExpression(value) {
  return value.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
}

function highlightSyntaxText(value, selectedIdentifier) {
  if (!selectedIdentifier) return escapeSyntaxText(value);
  const pattern = new RegExp(`\\b${escapeRegularExpression(selectedIdentifier)}\\b`, "g");
  let result = "";
  let offset = 0;
  for (const match of value.matchAll(pattern)) {
    result += escapeSyntaxText(value.slice(offset, match.index));
    result += `<mark class="syntax-match">${escapeSyntaxText(match[0])}</mark>`;
    offset = match.index + match[0].length;
  }
  return result + escapeSyntaxText(value.slice(offset));
}

function highlightCpp(source, selectedIdentifier = "") {
  let result = "";
  let offset = 0;
  for (const match of source.matchAll(cppTokenPattern)) {
    const token = match[0];
    result += highlightSyntaxText(source.slice(offset, match.index), selectedIdentifier);
    let kind = "keyword";
    if (token.startsWith("//") || token.startsWith("/*")) kind = "comment";
    else if (token.startsWith("\"") || token.startsWith("'")) kind = "string";
    else if (token.trimStart().startsWith("#")) kind = "preprocessor";
    else if (/^(?:0[xX]|0[bB]|\d)/.test(token)) kind = "number";
    result += `<span class="syntax-${kind}">${escapeSyntaxText(token)}</span>`;
    offset = match.index + token.length;
  }
  return result + highlightSyntaxText(source.slice(offset), selectedIdentifier);
}

function editAnswerIndent(value, selectionStart, selectionEnd, outdent = false) {
  const start = Math.max(0, Math.min(selectionStart, value.length));
  const end = Math.max(start, Math.min(selectionEnd, value.length));
  if (!outdent && start === end) {
    return {
      value: value.slice(0, start) + answerIndent + value.slice(end),
      selectionStart: start + answerIndent.length,
      selectionEnd: start + answerIndent.length,
    };
  }

  const blockStart = value.lastIndexOf("\n", Math.max(0, start - 1)) + 1;
  const selectedBlock = value.slice(blockStart, end);
  const lines = selectedBlock.split("\n");
  let changedBeforeStart = 0;
  let totalChange = 0;
  const editedLines = lines.map((line, index) => {
    if (index === lines.length - 1 && line === "" && selectedBlock.endsWith("\n")) return line;
    if (!outdent) {
      totalChange += answerIndent.length;
      if (index === 0) changedBeforeStart = answerIndent.length;
      return answerIndent + line;
    }
    const removable = Math.min(answerIndent.length, line.length - line.trimStart().length);
    totalChange -= removable;
    if (index === 0) changedBeforeStart = -removable;
    return line.slice(removable);
  });
  return {
    value: value.slice(0, blockStart) + editedLines.join("\n") + value.slice(end),
    selectionStart: Math.max(blockStart, start + changedBeforeStart),
    selectionEnd: Math.max(blockStart, end + totalChange),
  };
}

function renderAnswerHighlight() {
  const answer = document.querySelector("#answer");
  const highlightCode = document.querySelector("#answer-highlight code");
  if (!answer || !highlightCode) return;
  const selected = answer.value.slice(answer.selectionStart, answer.selectionEnd);
  const selectedIdentifier = /^[A-Za-z_]\w*$/.test(selected) ? selected : "";
  highlightCode.innerHTML = highlightCpp(answer.value, selectedIdentifier) + (answer.value.endsWith("\n") ? " " : "\n");
}

function initializeAnswerEditor() {
  const answer = document.querySelector("#answer");
  if (!answer) return;
  const highlight = document.querySelector("#answer-highlight");
  const syncHighlightScroll = () => {
    if (!highlight) return;
    highlight.scrollTop = answer.scrollTop;
    highlight.scrollLeft = answer.scrollLeft;
  };
  answer.addEventListener("input", renderAnswerHighlight);
  answer.addEventListener("select", renderAnswerHighlight);
  answer.addEventListener("scroll", syncHighlightScroll);
  answer.addEventListener("keydown", event => {
    if (event.key !== "Tab" || event.altKey || event.ctrlKey || event.metaKey) return;
    event.preventDefault();
    const scrollTop = answer.scrollTop;
    const scrollLeft = answer.scrollLeft;
    const edited = editAnswerIndent(answer.value, answer.selectionStart, answer.selectionEnd, event.shiftKey);
    answer.value = edited.value;
    answer.setSelectionRange(edited.selectionStart, edited.selectionEnd);
    renderAnswerHighlight();
    answer.scrollTop = scrollTop;
    answer.scrollLeft = scrollLeft;
    syncHighlightScroll();
  });
  renderAnswerHighlight();
}

initializeAnswerEditor();

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

function selectedWheelRotation(selectedIndex, candidateCount) {
  return candidateCount > 0 ? -selectedIndex * (360 / candidateCount) : 0;
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
  label.textContent = "Выбираем задание…";
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
  const finalRotation = selectedWheelRotation(selectedIndex, wheelCandidates.length);
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

const defenseStatusLabels = { ready: "Ожидает настройки преподавателя", preparing: "Подготовка задания…", active: "Защита идёт", passed: "Защита успешно сдана", failed: "Защита не пройдена", error: "Ошибка подготовки защиты", expired: "Время защиты истекло", cancelled: "Защита отменена" };
const preparationStageLabels = { analyzing_project: "Анализируем проект и ищем задания…", waiting_for_teacher: "Ожидаем настройки преподавателя", materializing_challenge: "Формируем выбранное задание…", ready: "Задание готово" };

function initializeStudentLabTable() {
  const root = document.querySelector("#student-labs");
  if (!root) return;
  const firstRows = new Map();
  root.querySelectorAll("tr[data-lab-code]").forEach(row => {
    const key = row.dataset.labCode;
    const first = firstRows.get(key);
    if (!first) {
      firstRows.set(key, row);
      return;
    }
    const option = row.querySelector(".lab-version-select option");
    if (option) first.querySelector(".lab-version-select")?.append(option);
    row.remove();
  });
  firstRows.forEach(row => {
    const select = row.querySelector(".lab-version-select");
    const download = row.querySelector("[data-download-submission]");
    const start = row.querySelector("[data-start-defense]");
    if (!select) return;
    const options = [...select.options].sort((left, right) => Number(right.dataset.version) - Number(left.dataset.version));
    select.replaceChildren(...options);
    const syncVersion = () => {
      const submissionID = select.value;
      if (download) download.href = `/api/v1/submissions/${submissionID}/archive`;
      if (start) start.dataset.startDefense = submissionID;
    };
    select.addEventListener("change", syncVersion);
    syncVersion();
  });
}

initializeStudentLabTable();

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
    if (value.current_draft && !document.querySelector("#answer").value) {
      document.querySelector("#answer").value = value.current_draft;
      renderAnswerHighlight();
    }
    status.textContent = preparationStageLabels[value.preparation_stage] || defenseStatusLabels[value.status] || value.status;
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

let teacherDefense;
let teacherFiles = [];
let teacherFilePath = "";
const teacherSourceCache = new Map();

const entityLabels = { function: "Функция", class: "Класс", struct: "Структура" };

function teacherCandidateAllowed(candidate) {
  const mode = document.querySelector('#configure-defense-form [name="entity_mode"]')?.value;
  return mode === "functions_classes_structs" || candidate.entity_type === "function";
}

function setConfigurationError(message = "") {
  const root = document.querySelector("#configuration-error");
  if (!root) return;
  root.textContent = message;
  root.classList.toggle("hidden", !message);
}

function selectTeacherCandidate(candidate) {
  const form = document.querySelector("#configure-defense-form");
  form.elements.candidate_id.value = candidate.id;
  const selected = document.querySelector("#selected-candidate");
  selected.textContent = `${entityLabels[candidate.entity_type] || "Сущность"}: ${candidate.signature.trim()} · ${candidate.file_path}:${candidate.begin_line}–${candidate.end_line}${candidate.is_test_file ? " · тестовый файл" : ""}`;
  document.querySelectorAll(".teacher-entity-option").forEach(option => option.classList.toggle("is-selected", option.dataset.candidateId === candidate.id));
  setConfigurationError();
}

function renderTeacherEntities(path, source) {
  const root = document.querySelector("#teacher-entity-list");
  if (!root || !teacherDefense) return;
  const candidates = (teacherDefense.candidate_catalog || []).filter(candidate => candidate.file_path === path && teacherCandidateAllowed(candidate));
  if (!candidates.length) {
    root.replaceChildren(node("div", "empty-state", "В этом файле нет доступных сущностей"));
    return;
  }
  const lines = source.split("\n");
  root.replaceChildren(...candidates.map(candidate => {
    const button = node("button", "teacher-entity-option");
    button.type = "button";
    button.dataset.candidateId = candidate.id;
    const title = node("span", "teacher-entity-option__title", `${entityLabels[candidate.entity_type] || candidate.entity_type} · ${candidate.function_name}`);
    const meta = node("span", "teacher-entity-option__meta", `строки ${candidate.begin_line}–${candidate.end_line}${candidate.is_test_file ? " · тестовый файл" : ""}`);
    const code = node("pre", "teacher-entity-option__code", lines.slice(Math.max(0, candidate.begin_line - 1), candidate.end_line).join("\n"));
    button.append(title, meta, code);
    button.onclick = () => selectTeacherCandidate(candidate);
    if (document.querySelector('#configure-defense-form [name="candidate_id"]').value === candidate.id) button.classList.add("is-selected");
    return button;
  }));
}

async function openTeacherRepositoryFile(path) {
  if (!teacherDefense) return;
  teacherFilePath = path;
  let value = teacherSourceCache.get(`${teacherDefense.id}:${path}`);
  if (!value) {
    value = await api(`/api/v1/defenses/${teacherDefense.id}/repository/file?path=${encodeURIComponent(path)}`);
    teacherSourceCache.set(`${teacherDefense.id}:${path}`, value);
  }
  document.querySelector("#teacher-source-code code").textContent = value.content;
  document.querySelectorAll("#teacher-repository-files button").forEach(button => button.classList.toggle("is-active", button.dataset.path === path));
  renderTeacherEntities(path, value.content);
}

async function loadTeacherRepository() {
  const value = await api(`/api/v1/defenses/${teacherDefense.id}/repository`);
  teacherFiles = value.items;
  const root = document.querySelector("#teacher-repository-files");
  root.replaceChildren(...teacherFiles.map(item => {
    const button = node("button", "repository-file", item.path);
    button.type = "button";
    button.dataset.path = item.path;
    button.onclick = () => openTeacherRepositoryFile(item.path);
    return button;
  }));
  const firstCandidate = (teacherDefense.candidate_catalog || []).find(teacherCandidateAllowed);
  const initialPath = firstCandidate?.file_path || teacherFiles[0]?.path;
  if (initialPath) await openTeacherRepositoryFile(initialPath);
}

function syncConfigurationMode() {
  const form = document.querySelector("#configure-defense-form");
  if (!form || !teacherDefense) return;
  const manual = form.elements.selection_mode.value === "manual";
  document.querySelector("#manual-candidate-workspace").classList.toggle("hidden", !manual);
  document.querySelector("#selected-candidate").classList.toggle("hidden", !manual);
  if (!manual) {
    form.elements.candidate_id.value = "";
    setConfigurationError();
  } else if (teacherFilePath && teacherSourceCache.has(`${teacherDefense.id}:${teacherFilePath}`)) {
    renderTeacherEntities(teacherFilePath, teacherSourceCache.get(`${teacherDefense.id}:${teacherFilePath}`).content);
  }
}

async function choosePendingDefense(defense) {
  teacherDefense = defense;
  teacherFilePath = "";
  const form = document.querySelector("#configure-defense-form");
  form.classList.remove("hidden");
  form.elements.defense_id.value = defense.id;
  form.elements.candidate_id.value = "";
  document.querySelector("#configuration-title").textContent = `${defense.student_login} · ${defense.lab_code}`;
  document.querySelector("#configuration-hint").textContent = defense.lab_name;
  document.querySelector("#selected-candidate").textContent = "В ручном режиме выберите функцию, класс или структуру.";
  syncConfigurationMode();
  await loadTeacherRepository();
  form.scrollIntoView({ behavior: "smooth", block: "start" });
}

async function loadPendingDefenses() {
  const value = await api("/api/v1/teacher/defenses/pending");
  const root = document.querySelector("#pending-defenses");
  if (!value.items.length) {
    root.replaceChildren(node("div", "empty-state", "Защит, ожидающих настройки, нет"));
    return;
  }
  root.replaceChildren(...value.items.map(defense => {
    const card = node("article", "card pending-defense");
    card.append(node("span", "badge", defense.lab_code), node("h3", "", defense.lab_name));
    card.append(node("p", "pending-defense__student", `Студент: ${defense.student_login}`));
    const button = node("button", "secondary-button", "Настроить защиту");
    button.type = "button";
    button.onclick = () => choosePendingDefense(defense);
    card.append(button);
    return card;
  }));
}

document.querySelector("#load-pending-defenses")?.addEventListener("click", loadPendingDefenses);
document.querySelector('#configure-defense-form [name="selection_mode"]')?.addEventListener("change", syncConfigurationMode);
document.querySelector('#configure-defense-form [name="entity_mode"]')?.addEventListener("change", () => {
  const form = document.querySelector("#configure-defense-form");
  const selected = (teacherDefense?.candidate_catalog || []).find(candidate => candidate.id === form.elements.candidate_id.value);
  if (selected && !teacherCandidateAllowed(selected)) {
    form.elements.candidate_id.value = "";
    document.querySelector("#selected-candidate").textContent = "Выбранная сущность недоступна в этом режиме. Выберите другую.";
  }
  if (teacherDefense && teacherFilePath && teacherSourceCache.has(`${teacherDefense.id}:${teacherFilePath}`)) {
    renderTeacherEntities(teacherFilePath, teacherSourceCache.get(`${teacherDefense.id}:${teacherFilePath}`).content);
  }
});

document.querySelector("#configure-defense-form")?.addEventListener("submit", async event => {
  event.preventDefault();
  const form = new FormData(event.target);
  const mode = form.get("selection_mode");
  const minutes = Number(form.get("time_limit_minutes"));
  const wheelSize = Number(form.get("wheel_size"));
  if (minutes < 1 || minutes > 30 || wheelSize < 2 || wheelSize > 12) {
    setConfigurationError("Время должно быть от 1 до 30 минут, колесо — от 2 до 12 секторов.");
    return;
  }
  if (mode === "manual" && !form.get("candidate_id")) {
    setConfigurationError("Выберите сущность для восстановления.");
    return;
  }
  const catalog = (teacherDefense?.candidate_catalog || []).filter(teacherCandidateAllowed);
  const selected = catalog.find(candidate => candidate.id === form.get("candidate_id"));
  const nonTestCandidates = catalog.filter(candidate => !candidate.is_test_file && candidate.id !== selected?.id);
  const enoughCandidates = mode === "automatic"
    ? nonTestCandidates.length >= wheelSize
    : Boolean(selected) && nonTestCandidates.length >= wheelSize - 1;
  if (!enoughCandidates) {
    const available = mode === "automatic" ? nonTestCandidates.length : nonTestCandidates.length + (selected ? 1 : 0);
    setConfigurationError(`Для колеса из ${wheelSize} секторов недостаточно подходящих сущностей. Доступно: ${available}. Уменьшите число секторов или смените режим.`);
    return;
  }
  try {
    await api(`/api/v1/defenses/${form.get("defense_id")}/configure`, { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ selection_mode: mode, candidate_id: mode === "manual" ? form.get("candidate_id") : "", entity_mode: form.get("entity_mode"), wheel_size: wheelSize, time_limit_seconds: minutes * 60 }) });
    event.target.classList.add("hidden");
    teacherDefense = undefined;
    await loadPendingDefenses();
  } catch (error) {
    setConfigurationError(error.problem?.title || error.message);
  }
});

document.querySelector("#submissions-form")?.addEventListener("submit", async event => {
  event.preventDefault();
  const form = new FormData(event.target);
  const value = await api(`/api/v1/groups/${form.get("group_id")}/submissions`);
  const root = document.querySelector("#submissions-result");
  if (!value.items.length) {
    root.replaceChildren(node("div", "empty-state", "В этой группе пока нет загруженных работ"));
    return;
  }
  const groups = new Map();
  value.items.forEach(item => {
    const key = `${item.github_login}:${item.lab_code}`;
    if (!groups.has(key)) groups.set(key, []);
    groups.get(key).push(item);
  });
  const table = node("table", "lab-table lab-table--teacher");
  const head = document.createElement("thead");
  const heading = document.createElement("tr");
  ["Студент и лабораторная", "Версия", "Действия"].forEach(label => heading.append(node("th", "", label)));
  head.append(heading);
  const body = document.createElement("tbody");
  groups.forEach(items => {
    items.sort((left, right) => Number(right.version_number) - Number(left.version_number));
    const current = items[0];
    const row = document.createElement("tr");
    const titleCell = document.createElement("td");
    const title = node("div", "lab-title-cell");
    title.append(node("strong", "", current.lab_name || current.lab_code), node("span", "lab-table__meta", `@${current.github_login} · ${current.lab_code}`));
    titleCell.append(title);
    const versionCell = document.createElement("td");
    const select = node("select", "lab-version-select");
    select.setAttribute("aria-label", `Версия ${current.lab_name || current.lab_code} студента ${current.github_login}`);
    items.forEach(item => {
      const option = node("option", "", `Версия ${item.version_number}`);
      option.value = item.id;
      select.append(option);
    });
    versionCell.append(select);
    const actionsCell = document.createElement("td");
    const actions = node("div", "lab-row-actions");
    const download = node("a", "secondary-button", "Скачать ZIP");
    download.href = `/api/v1/submissions/${select.value}/archive`;
    select.addEventListener("change", () => {
      download.href = `/api/v1/submissions/${select.value}/archive`;
    });
    actions.append(download);
    actionsCell.append(actions);
    row.append(titleCell, versionCell, actionsCell);
    body.append(row);
  });
  table.append(head, body);
  const wrapper = node("div", "lab-table-wrap");
  wrapper.append(table);
  root.replaceChildren(wrapper);
});

async function loadPendingLinks() {
  const value = await api("/api/v1/admin/pending-links");
  const root = document.querySelector("#pending-result");
  if (!value.items.length) {
    root.replaceChildren(node("div", "empty-state", "Ожидающих привязки пользователей нет"));
    return;
  }
  root.replaceChildren(...value.items.map(item => {
    const card = node("article", "management-item");
    card.append(node("strong", "", item.display_name || item.github_login), node("span", "", `GitHub: ${item.github_login}`));
    card.append(node("span", "", item.group_code ? `Предложенная группа: ${item.group_code}` : "Совпадающая запись студента не найдена"));
    if (item.student_record_id) {
      const button = node("button", "secondary-button", "Выбрать связь");
      button.type = "button";
      button.onclick = () => {
        const form = document.querySelector("#approve-link-form");
        form.classList.remove("hidden");
        form.elements.user_id.value = item.user_id;
        form.elements.student_record_id.value = item.student_record_id;
        document.querySelector("#link-summary").textContent = `${item.github_login} → группа ${item.group_code}`;
      };
      card.append(button);
    }
    return card;
  }));
}

document.querySelector("#load-pending")?.addEventListener("click", loadPendingLinks);

document.querySelector("#approve-link-form")?.addEventListener("submit", async event => {
  event.preventDefault();
  const form = new FormData(event.target);
  await api(`/api/v1/admin/pending-links/${form.get("user_id")}/approve`, { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ student_record_id: form.get("student_record_id"), reason: "Подтверждено через кабинет" }) });
  event.target.classList.add("hidden");
  await loadPendingLinks();
});

async function loadUsers() {
  const value = await api("/api/v1/admin/users");
  const root = document.querySelector("#users-result");
  const roleLabels = { student: "студент", teacher: "преподаватель", admin: "администратор" };
  const statusLabels = { pending: "ожидает подтверждения", active: "активен", rejected: "отклонён", blocked: "заблокирован" };
  root.replaceChildren(...value.items.map(item => {
    const card = node("article", "management-item management-item--user");
    const name = node("strong", "", item.display_name || item.github_login);
    const summary = node("span", "", `@${item.github_login} · ${roleLabels[item.role] || "роль не назначена"} · ${statusLabels[item.status] || item.status}`);
    const details = node("details", "technical-details");
    details.append(node("summary", "", "Технические данные"), node("code", "", `User ID: ${item.id}\nGitHub ID: ${item.github_id}`));
    const button = node("button", "secondary-button", "Изменить роль");
    button.type = "button";
    button.onclick = () => {
      const form = document.querySelector("#role-form");
      form.elements.user_id.value = item.id;
      form.elements.role.value = item.role || "student";
      form.scrollIntoView({ behavior: "smooth", block: "center" });
    };
    card.append(name, summary, details, button);
    return card;
  }));
}

document.querySelector("#load-users")?.addEventListener("click", loadUsers);

document.querySelector("#role-form")?.addEventListener("submit", async event => {
  event.preventDefault();
  const form = new FormData(event.target);
  await api(`/api/v1/admin/users/${form.get("user_id")}/role`, { method: "PUT", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ role: form.get("role") }) });
  await loadUsers();
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
