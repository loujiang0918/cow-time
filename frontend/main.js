// 前端只负责渲染：每秒向 Go 取一次状态，并把用户的操作转交给 Go。
import { Call, Window } from "/wails/runtime.js";

// 服务在 Wails 绑定表里的全名是「包路径.类型名」。
// internal/service/fqn_test.go 会检查它与 Go 侧的实际包路径一致。
const SERVICE = "cowtime/internal/service.Service";

const call = (method, ...args) => Call.ByName(`${SERVICE}.${method}`, ...args);

const $ = (id) => document.getElementById(id);

const el = {
  display: $("display"),
  settings: $("settings"),
  monthCurrency: $("month-currency"),
  todayCurrency: $("today-currency"),
  state: $("state"),
  holidayRow: $("holiday-row"),
  holidayLabel: $("holiday-label"),
  holidayDays: $("holiday-days"),
  holidayUnit: $("holiday-unit"),
  form: $("settings-form"),
  salary: $("salary"),
  start: $("start"),
  lunchStart: $("lunch-start"),
  lunchEnd: $("lunch-end"),
  end: $("end"),
  weekdayTabs: $("weekday-tabs"),
  weekdayNote: $("weekday-note"),
  resetDay: $("reset-day"),
  stylePicker: $("style-picker"),
  transparency: $("transparency"),
  transparencyValue: $("transparency-value"),
  stars: $("stars"),
  hint: $("hint"),
  save: $("save"),
};

// 不计薪时显示的原因。
const STATE_LABELS = {
  beforeWork: "未上班",
  lunch: "午休中",
  afterWork: "已下班",
};
const DAY_OFF_LABELS = {
  weekend: "休息日",
  holiday: "假期",
};

// 背景风格的取值，与 Go 侧 config 包里的常量一致。
const BACKGROUND_STYLES = ["plain", "aurora", "neon", "stars"];
const DEFAULT_BACKGROUND_STYLE = "aurora";
// 每种背景风格的初始透明度，单位是百分比，与 Go 侧 config.DefaultTransparency 一致。
const DEFAULT_TRANSPARENCY = { plain: 28, aurora: 60, neon: 32, stars: 42 };
// 拖动滑块之后，隔这么久再把透明度写入配置。
const TRANSPARENCY_SAVE_DELAY = 200;

// 滚动数字的参数，与样式表里每一位数字的高度和过渡时长一致。
const DIGIT_HEIGHT = 22;
const ROLL_MS = 280;

// 星空风格里星星的数量。
const STAR_COUNT = 28;

// 作息的四个时间项，以及它们对应的输入框。星期用 "1" 到 "7" 表示，"1" 是周一。
const TIME_FIELDS = [
  ["start", el.start],
  ["lunchStart", el.lunchStart],
  ["lunchEnd", el.lunchEnd],
  ["end", el.end],
];
const SCHEDULE_NAMES = ["默认", "周一", "周二", "周三", "周四", "周五", "周六", "周日"];

let current = null; // 最近一次从 Go 拿到的状态
let settingsOpen = false;
let settingsDeclined = false; // 还没设置月薪时点了取消：本次运行不再自动打开设置

// 设置视图里正在编辑的作息：base 是默认作息，overrides 是每个星期与默认不同的地方。
let hoursDraft = { base: {}, overrides: {} };
let selectedWeekday = "0"; // "0" 表示「默认」

// 设置视图里拖动过的背景风格的透明度，单位是百分比。没有拖动过的风格取已经保存的值。
let transparencyDraft = {};
let transparencyTimer = 0; // 等待写入配置的定时器，0 表示没有待写入的改动

async function tick() {
  try {
    render(await call("GetStatus"));
  } catch (err) {
    console.error("获取状态失败", err);
  }
}

function render(status) {
  current = status;

  // 设置视图打开期间预览选中的风格，平时显示已经保存的风格。
  applyBackgroundStyle(settingsOpen ? pickedStyle() : status.settings.backgroundStyle);

  if (status.salaryConfigured) {
    setMoney(odometers.month, el.monthCurrency, status.monthEarned);
    setMoney(odometers.today, el.todayCurrency, status.todayEarned);
  } else {
    setMoney(odometers.month, el.monthCurrency, null);
    setMoney(odometers.today, el.todayCurrency, null);
    if (!settingsOpen && !settingsDeclined) openSettings();
  }

  renderState(status);
  renderHoliday(status.holiday);
}

/* ---------- 金额 ---------- */

// 金额保留的小数位数。
const DECIMAL_PLACES = 3;

const money = (value) =>
  value.toLocaleString("zh-CN", {
    minimumFractionDigits: DECIMAL_PLACES,
    maximumFractionDigits: DECIMAL_PLACES,
  });

function setMoney(odometer, currencyEl, value) {
  if (value === null) {
    odometer.set("--");
    currencyEl.hidden = true;
    return;
  }
  odometer.set(money(value));
  currencyEl.hidden = false;
}

// makeOdometer 把 el 变成里程表式的滚动数字。
// 每一位数字是一条竖直纸带，上面依次是 0 到 9；数字变化时纸带向上移动，停在新的数字上。
function makeOdometer(el) {
  let slots = [];
  let text = "";

  function build(str) {
    el.textContent = "";
    slots = [];
    for (const ch of str) {
      if (isDigit(ch)) {
        const digit = document.createElement("span");
        digit.className = "od-digit";
        const reel = document.createElement("span");
        reel.className = "od-reel";
        for (let i = 0; i < 20; i++) {
          const row = document.createElement("span");
          row.textContent = String(i % 10);
          reel.appendChild(row);
        }
        digit.appendChild(reel);
        el.appendChild(digit);
        slots.push({ reel, cur: +ch, pos: +ch, timer: 0 });
      } else {
        const char = document.createElement("span");
        char.className = "od-char";
        char.textContent = ch;
        el.appendChild(char);
        slots.push(null);
      }
    }
    text = str;
    slots.forEach((slot) => {
      if (!slot) return;
      slot.reel.style.transition = "none";
      slot.reel.style.transform = translateY(slot.pos);
      void slot.reel.offsetWidth;
      slot.reel.style.transition = "";
    });
  }

  function set(str) {
    // 位数或标点的位置变了，就直接重建，不做动画。
    if (!sameShape(text, str)) {
      build(str);
      return;
    }
    [...str].forEach((ch, i) => {
      const slot = slots[i];
      if (!slot) return;
      const next = +ch;
      if (next === slot.cur) return;
      const target = slot.pos + ((next - slot.cur + 10) % 10);
      slot.cur = next;
      slot.reel.style.transform = translateY(target);
      clearTimeout(slot.timer);
      slot.timer = setTimeout(() => {
        // 纸带有两轮 0-9：滚过第一轮之后，趁没人看见跳回开头。
        if (target >= 10) {
          slot.pos = target - 10;
          slot.reel.style.transition = "none";
          slot.reel.style.transform = translateY(slot.pos);
          void slot.reel.offsetWidth;
          slot.reel.style.transition = "";
        } else {
          slot.pos = target;
        }
      }, ROLL_MS + 20);
    });
    text = str;
  }

  function translateY(row) {
    return `translateY(${-row * DIGIT_HEIGHT}px)`;
  }

  return { set };
}

function isDigit(ch) {
  return ch >= "0" && ch <= "9";
}

// sameShape 报告两个字符串的长度相同，且数字和标点的位置一一对应。
function sameShape(a, b) {
  return a.length === b.length && [...a].every((ch, i) => isDigit(ch) === isDigit(b[i]));
}

const odometers = {
  month: makeOdometer($("month")),
  today: makeOdometer($("today")),
};

function renderState(status) {
  const label =
    status.state === "dayOff"
      ? DAY_OFF_LABELS[status.dayOffReason] ?? "休息日"
      : (STATE_LABELS[status.state] ?? "");
  el.state.textContent = label;
  el.state.hidden = label === "";
}

function renderHoliday(holiday) {
  el.holidayRow.classList.toggle("is-ongoing", holiday.kind === "ongoing");

  if (holiday.kind === "upcoming") {
    el.holidayLabel.textContent = `距${holiday.name}`;
  } else if (holiday.kind === "ongoing") {
    el.holidayLabel.textContent = `${holiday.name} 假期中，还剩`;
  } else {
    el.holidayLabel.textContent = "假期";
  }

  const unknown = holiday.kind === "unknown";
  el.holidayDays.textContent = unknown ? "数据待更新" : String(holiday.daysLeft);
  el.holidayUnit.hidden = unknown;
}

/* ---------- 背景风格 ---------- */

// pickedStyle 返回设置视图里当前选中的背景风格。
function pickedStyle() {
  const checked = el.stylePicker.querySelector("input:checked");
  return checked ? checked.value : DEFAULT_BACKGROUND_STYLE;
}

// applyBackgroundStyle 把背景风格写到 body 上，样式表据此切换卡片的外观。
// 没有对应样式的取值按默认风格处理。
function applyBackgroundStyle(style) {
  const resolved = BACKGROUND_STYLES.includes(style) ? style : DEFAULT_BACKGROUND_STYLE;
  if (document.body.dataset.style !== resolved) {
    document.body.dataset.style = resolved;
  }
  // 透明度是底色透明的程度，换算成底色的不透明程度交给样式表。
  document.body.style.setProperty("--card-alpha", String(1 - transparencyOf(resolved) / 100));
}

// transparencyOf 返回某种背景风格的透明度，单位是百分比。
// 设置视图打开期间取正在编辑的值，平时取已经保存的值。
function transparencyOf(style) {
  if (settingsOpen && style in transparencyDraft) {
    return transparencyDraft[style];
  }
  return current?.transparency?.[style] ?? DEFAULT_TRANSPARENCY[style];
}

/* ---------- 设置视图：透明度 ---------- */

// showTransparency 让滑块和百分比显示出当前选中的背景风格的透明度。
function showTransparency() {
  const value = transparencyOf(pickedStyle());
  el.transparency.value = String(value);
  el.transparencyValue.textContent = `${value}%`;
}

// onTransparencyInput 在滑块拖动时被调用：立即改变卡片底色，并安排一次保存。
function onTransparencyInput() {
  transparencyDraft[pickedStyle()] = Number(el.transparency.value);
  el.transparencyValue.textContent = `${el.transparency.value}%`;
  applyBackgroundStyle(pickedStyle());
  clearTimeout(transparencyTimer);
  transparencyTimer = setTimeout(saveTransparency, TRANSPARENCY_SAVE_DELAY);
}

// saveTransparency 把当前选中的背景风格的透明度写入配置。
// Go 拒绝保存时，滑块回到上一次保存的值，并显示原因。
async function saveTransparency() {
  clearTimeout(transparencyTimer);
  transparencyTimer = 0;
  const style = pickedStyle();
  const value = transparencyDraft[style];
  const result = await call("SetTransparency", style, value);
  if (result.ok) {
    current.transparency[style] = value;
    return;
  }
  console.error("保存透明度失败", result.message);
  delete transparencyDraft[style];
  if (settingsOpen) {
    showTransparency();
    applyBackgroundStyle(pickedStyle());
    el.hint.textContent = result.message;
  }
}

// makeStars 撒一把大小、节奏各不相同的星星。
function makeStars(sky) {
  for (let i = 0; i < STAR_COUNT; i++) {
    const star = document.createElement("span");
    const size = (1 + Math.random() * 1.6).toFixed(1);
    star.style.left = `${(Math.random() * 96).toFixed(1)}%`;
    star.style.top = `${(Math.random() * 92).toFixed(1)}%`;
    star.style.width = `${size}px`;
    star.style.height = `${size}px`;
    star.style.setProperty("--dur", `${(1.6 + Math.random() * 2.6).toFixed(2)}s`);
    star.style.setProperty("--delay", `${(-Math.random() * 4).toFixed(2)}s`);
    sky.appendChild(star);
  }
}

makeStars(el.stars);

/* ---------- 设置视图：作息 ---------- */

// cloneOverrides 复制一份覆盖项。来源可能不存在（配置里没有这一项）。
function cloneOverrides(overrides) {
  return Object.fromEntries(Object.entries(overrides ?? {}).map(([weekday, partial]) => [weekday, { ...partial }]));
}

// effectiveHours 返回某个星期的生效作息：默认作息套上这一天的覆盖。weekday 为 "0" 时就是默认作息。
function effectiveHours(weekday) {
  return { ...hoursDraft.base, ...hoursDraft.overrides[weekday] };
}

// hasOverride 报告某个星期是否有与默认作息不同的地方。
function hasOverride(weekday) {
  return Object.keys(hoursDraft.overrides[weekday] ?? {}).length > 0;
}

// showWeekday 选中一个星期标签，并把这一天生效的作息填进四个时间输入框。
function showWeekday(weekday) {
  selectedWeekday = weekday;
  for (const [field, input] of TIME_FIELDS) {
    input.value = effectiveHours(weekday)[field] ?? "";
  }
  refreshTabs();
}

// refreshTabs 刷新星期标签的状态：选中的标签、有单独设置的标签上的小圆点，
// 「恢复默认」按钮是否出现，以及选中周末时下面的说明是否变亮。
// 它不改动时间输入框，所以输入过程中也可以调用。
function refreshTabs() {
  for (const input of el.weekdayTabs.querySelectorAll("input")) {
    input.checked = input.value === selectedWeekday;
    input.parentElement.classList.toggle("is-overridden", input.value !== "0" && hasOverride(input.value));
  }
  el.resetDay.hidden = selectedWeekday === "0" || !hasOverride(selectedWeekday);
  el.weekdayNote.classList.toggle("is-active", selectedWeekday === "6" || selectedWeekday === "7");
}

// setTimeField 把一个时间输入框的新值写回编辑状态。
// 选的是「默认」时改的是默认作息；选的是某一天时记成这一天的覆盖，改回和默认相同就不再算覆盖。
function setTimeField(field, value) {
  if (selectedWeekday === "0") {
    hoursDraft.base[field] = value;
    pruneOverrides();
    refreshTabs();
    return;
  }
  const override = hoursDraft.overrides[selectedWeekday] ?? {};
  if (value === hoursDraft.base[field]) {
    delete override[field];
  } else {
    override[field] = value;
  }
  if (Object.keys(override).length > 0) {
    hoursDraft.overrides[selectedWeekday] = override;
  } else {
    delete hoursDraft.overrides[selectedWeekday];
  }
  refreshTabs();
}

// pruneOverrides 去掉已经和默认作息相同的覆盖项。默认作息改动之后调用。
function pruneOverrides() {
  for (const [weekday, override] of Object.entries(hoursDraft.overrides)) {
    for (const field of Object.keys(override)) {
      if (override[field] === hoursDraft.base[field]) {
        delete override[field];
      }
    }
    if (Object.keys(override).length === 0) {
      delete hoursDraft.overrides[weekday];
    }
  }
}

/* ---------- 设置视图 ---------- */

async function openSettings() {
  if (settingsOpen) return;
  settingsOpen = true;

  fillForm(current.settings);
  el.settings.hidden = false;
  el.display.hidden = true;
  el.salary.focus();
  await call("EnterSettings");
}

async function closeSettings() {
  if (!settingsOpen) return;
  // 离开设置视图之前，把还没写入配置的透明度改动写进去。
  if (transparencyTimer) saveTransparency();
  settingsOpen = false;

  el.display.hidden = false;
  el.settings.hidden = true;
  await call("LeaveSettings");
}

function cancelSettings() {
  if (current && !current.salaryConfigured) {
    settingsDeclined = true;
  }
  closeSettings();
  // 放弃刚才预览的风格，回到已经保存的那一种。
  applyBackgroundStyle(current.settings.backgroundStyle);
}

function fillForm(settings) {
  el.salary.value = settings.monthlySalary > 0 ? settings.monthlySalary : "";
  hoursDraft = { base: { ...settings.workHours }, overrides: cloneOverrides(settings.weekdayHours) };
  showWeekday("0");
  transparencyDraft = {};
  for (const input of el.stylePicker.querySelectorAll("input")) {
    input.checked = input.value === settings.backgroundStyle;
  }
  showTransparency();
  updateHint();
}

function readForm() {
  return {
    monthlySalary: Number(el.salary.value),
    workHours: { ...hoursDraft.base },
    weekdayHours: cloneOverrides(hoursDraft.overrides),
    backgroundStyle: pickedStyle(),
  };
}

// 把 "HH:MM" 换算成当天的分钟数；输入为空时返回 null。
function minutes(time) {
  if (time === "") return null;
  const [hour, minute] = time.split(":").map(Number);
  return hour * 60 + minute;
}

// hoursError 返回一套作息不合法的原因，合法时返回空字符串。规则与 Go 侧的 workhours.Parse 一致。
// 提示都控制在一行以内。
function hoursError(hours) {
  const start = minutes(hours.start);
  const end = minutes(hours.end);
  if (start === null || end === null) return "请填写上班时间和下班时间";

  const lunchStart = minutes(hours.lunchStart);
  const lunchEnd = minutes(hours.lunchEnd);
  if ((lunchStart === null) !== (lunchEnd === null)) return "午休开始和结束要同时填写或留空";
  if (lunchStart === null) {
    return start < end ? "" : "上班时间必须早于下班时间";
  }
  if (!(start < lunchStart && lunchStart < lunchEnd && lunchEnd < end)) {
    return "上班 < 午休开始 < 午休结束 < 下班";
  }
  return "";
}

// 返回表单的校验提示；表单合法时返回空字符串。
// 默认作息和七天的生效作息都要合法。
function formError() {
  const salary = Number(el.salary.value);
  if (el.salary.value === "" || !(salary > 0)) return "月薪必须大于 0";

  for (let weekday = 0; weekday <= 7; weekday++) {
    const error = hoursError(effectiveHours(String(weekday)));
    if (error === "") continue;
    // 出错的不是当前选中的星期时，指明是哪一天。
    return String(weekday) === selectedWeekday ? error : `${SCHEDULE_NAMES[weekday]}的作息时间有误`;
  }
  return "";
}

function updateHint() {
  const message = formError();
  el.hint.textContent = message;
  el.save.disabled = message !== "";
}

$("open-settings").addEventListener("click", openSettings);
$("close-settings").addEventListener("click", cancelSettings);
$("cancel").addEventListener("click", cancelSettings);
$("quit").addEventListener("click", () => Window.Close());

el.form.addEventListener("input", updateHint);
for (const [field, input] of TIME_FIELDS) {
  input.addEventListener("input", () => setTimeField(field, input.value));
}
el.weekdayTabs.addEventListener("change", (event) => showWeekday(event.target.value));
el.resetDay.addEventListener("click", () => {
  delete hoursDraft.overrides[selectedWeekday];
  showWeekday(selectedWeekday);
  updateHint();
});
el.stylePicker.addEventListener("change", () => {
  // 切换背景风格时，滑块跳到那一种风格的透明度。
  showTransparency();
  applyBackgroundStyle(pickedStyle());
});
el.transparency.addEventListener("input", onTransparencyInput);
el.transparency.addEventListener("change", saveTransparency);
el.form.addEventListener("submit", async (event) => {
  event.preventDefault();
  const result = await call("SaveSettings", readForm());
  if (!result.ok) {
    el.hint.textContent = result.message;
    el.save.disabled = true;
    return;
  }
  await closeSettings();
  await tick();
});

await tick();
setInterval(tick, 1000);
