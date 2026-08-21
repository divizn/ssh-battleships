export type Status = "up" | "asleep" | "unknown";

// unknown means the heartbeat could not be read, which is not the same as the server being down
export const badge: Record<Status, { border: string; dot: string; label: string }> = {
  up: { border: "border-live/30", dot: "bg-live", label: "server up" },
  asleep: { border: "border-border", dot: "bg-miss", label: "server asleep" },
  unknown: { border: "border-border", dot: "bg-warn", label: "status unknown" },
};
