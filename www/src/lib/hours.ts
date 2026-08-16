import schedule from "../../../hours.json";

const clock = (h: number) => `${h % 12 || 12}${h < 12 ? "am" : "pm"}`;

const days = schedule.days.slice(0, -1).join(", ") + " and " + schedule.days.at(-1);

export const hours = `It is up on ${days}, ${clock(schedule.open)} to ${clock(schedule.close)} UK time.`;
