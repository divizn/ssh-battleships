import { env } from "cloudflare:workers";

export type Entry = { name: string; wins: number };

const secrets = env as {
  UPSTASH_REDIS_REST_URL?: string;
  UPSTASH_REDIS_REST_TOKEN?: string;
};

// the game writes everything under this namespace, the upstash database is shared
const namespace = "battleships:";

type Reply = { result: unknown; error?: string };

async function pipeline(cmds: unknown[][]): Promise<Reply[]> {
  const res = await fetch(`${secrets.UPSTASH_REDIS_REST_URL!.replace(/\/$/, "")}/pipeline`, {
    method: "POST",
    headers: {
      authorization: `Bearer ${secrets.UPSTASH_REDIS_REST_TOKEN}`,
      "content-type": "application/json",
    },
    body: JSON.stringify(cmds),
  });
  if (!res.ok) throw new Error(`redis responded ${res.status}`);

  const replies = (await res.json()) as Reply[];
  const failed = replies.find((r) => r.error);
  if (failed) throw new Error(failed.error);
  return replies;
}

export async function top(n: number): Promise<Entry[]> {
  if (!secrets.UPSTASH_REDIS_REST_URL || !secrets.UPSTASH_REDIS_REST_TOKEN) return [];

  const [ranked] = await pipeline([
    ["ZRANGE", `${namespace}leaderboard`, 0, n - 1, "REV", "WITHSCORES"],
  ]);
  const flat = (ranked.result ?? []) as string[];
  if (flat.length === 0) return [];

  const ids = flat.filter((_, i) => i % 2 === 0);
  const wins = flat.filter((_, i) => i % 2 === 1).map(Number);
  const names = await pipeline(ids.map((id) => ["HGET", `${namespace}player:${id}`, "name"]));

  return ids.map((_, i) => ({
    name: (names[i].result as string | null) || "anonymous",
    wins: wins[i],
  }));
}
