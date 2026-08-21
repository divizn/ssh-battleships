import type { APIRoute } from "astro";
import { isLive } from "@/lib/leaderboard";

// without this the answer is baked in at build time and never changes
export const prerender = false;

export const GET: APIRoute = async () => {
  try {
    return Response.json({ live: await isLive() }, { headers: { "cache-control": "no-store" } });
  } catch (err) {
    console.error("redis:", err);
    return new Response(null, { status: 503 });
  }
};
