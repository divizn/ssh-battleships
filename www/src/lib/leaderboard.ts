import { env } from "cloudflare:workers";

// rate is the player's overall win percentage, null until they have played a decided game
export type Entry = { name: string; wins: number; rate: number | null };

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

// live is written by the game server every minute and expires in 150 seconds, so an instance
// that was stopped on schedule, stopped by hand or simply died all read the same here.
export async function isLive(): Promise<boolean> {
  if (!secrets.UPSTASH_REDIS_REST_URL || !secrets.UPSTASH_REDIS_REST_TOKEN) return false;

  const [beat] = await pipeline([["GET", `${namespace}live`]]);
  return beat.result !== null;
}

// wins against people, the board worth reading
export async function top(n: number): Promise<Entry[]> {
  return board(`${namespace}leaderboard`, n);
}

// every win a tracked player has, bot games included
export async function topTotal(n: number): Promise<Entry[]> {
  return board(`${namespace}total`, n);
}

async function board(zset: string, n: number): Promise<Entry[]> {
  if (!secrets.UPSTASH_REDIS_REST_URL || !secrets.UPSTASH_REDIS_REST_TOKEN) return [];

  const [ranked] = await pipeline([["ZRANGE", zset, 0, n - 1, "REV", "WITHSCORES"]]);
  const flat = (ranked.result ?? []) as string[];
  if (flat.length === 0) return [];

  const ids = flat.filter((_, i) => i % 2 === 0);
  const wins = flat.filter((_, i) => i % 2 === 1).map(Number);
  // the score is this board's wins, the hash carries the overall record the rate comes from
  const profiles = await pipeline(
    ids.map((id) => ["HMGET", `${namespace}player:${id}`, "name", "wins", "losses"]),
  );

  return ids.map((_, i) => {
    const [name, won, lost] = (profiles[i].result ?? []) as (string | null)[];
    const decided = Number(won ?? 0) + Number(lost ?? 0);
    return {
      name: name || "anonymous",
      wins: wins[i],
      rate: decided > 0 ? Math.round((Number(won ?? 0) / decided) * 100) : null,
    };
  });
}
