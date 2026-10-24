export const MSSQL_TZ_STORAGE_KEY = "mssql_log_timezone";
export const MSSQL_TZ_PRESET_KEY = "mssql_log_tz_preset";

export const TIMEZONE_PRESETS = [
  { value: "America/New_York", label: "Eastern (ET)", short: "ET" },
  { value: "UTC", label: "UTC", short: "UTC" },
  { value: "America/Chicago", label: "Central (CT)", short: "CT" },
  { value: "America/Denver", label: "Mountain (MT)", short: "MT" },
  { value: "America/Los_Angeles", label: "Pacific (PT)", short: "PT" },
  { value: "America/Anchorage", label: "Alaska (AKT)", short: "AKT" },
  { value: "Pacific/Honolulu", label: "Hawaii (HT)", short: "HT" },
  { value: "Europe/London", label: "London", short: "GMT/BST" },
  { value: "Europe/Paris", label: "Central Europe", short: "CET" },
  { value: "Asia/Tokyo", label: "Tokyo (JST)", short: "JST" },
  { value: "Asia/Shanghai", label: "China (CST)", short: "CST" },
  { value: "Australia/Sydney", label: "Sydney", short: "AEDT" },
];

export const CUSTOM_TZ_PRESET = "__custom__";

export function isValidTimezone(tz) {
  if (!tz || typeof tz !== "string") return false;
  const trimmed = tz.trim();
  if (!trimmed) return false;
  try {
    Intl.DateTimeFormat(undefined, { timeZone: trimmed });
    return true;
  } catch {
    return false;
  }
}

export function timezoneShortLabel(tz) {
  const preset = TIMEZONE_PRESETS.find((p) => p.value === tz);
  if (preset) return preset.short;
  if (tz === "UTC") return "UTC";
  const parts = tz.split("/");
  return parts[parts.length - 1]?.replace(/_/g, " ") || tz;
}

export function formatTimestampInZone(iso, timeZone) {
  if (!iso) return "";
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return "";
  const fmt = new Intl.DateTimeFormat("en-US", {
    timeZone,
    month: "2-digit",
    day: "2-digit",
    year: "numeric",
    hour: "2-digit",
    minute: "2-digit",
    second: "2-digit",
    hour12: false,
  });
  const parts = Object.fromEntries(fmt.formatToParts(d).map((p) => [p.type, p.value]));
  return `${parts.month}-${parts.day}-${parts.year} ${parts.hour}:${parts.minute}:${parts.second}`;
}

export function formatLogEntryLine(entry, timeZone) {
  let body = entry.message ?? "";
  if (entry.container) {
    body = `${entry.container} | ${body}`;
  }
  if (entry.stream) {
    body = `[${entry.stream}] ${body}`;
  }
  if (!entry.ts) {
    return body;
  }
  const stamp = formatTimestampInZone(entry.ts, timeZone);
  const label = timezoneShortLabel(timeZone);
  return `${stamp} ${label} ${body}`;
}

export function normalizeLogEntry(raw) {
  if (!raw || typeof raw !== "object") return null;
  return {
    id: raw.id ?? raw.ID ?? 0,
    ts: raw.ts ?? raw.TS ?? "",
    container: raw.container ?? raw.ContainerName ?? "",
    stream: raw.stream ?? raw.Stream ?? "",
    message: raw.message ?? raw.Message ?? "",
  };
}
