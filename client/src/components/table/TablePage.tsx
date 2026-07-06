// Connects the socket (real server if VITE_WS_URL is set, else the mock/fixture
// server so the table is demoable) and renders the table. The table to join
// comes from the URL (?join=<tableId> from an invite / Quick Seat / lobby row)
// and the auth token from storage, so a real online seat actually joins the
// server-side table. This is the priority route.

import { useEffect } from "react";
import { useSearchParams } from "react-router-dom";
import { useGame } from "@/store/gameStore";
import { getStoredToken } from "@/net/api";
import Table from "./Table";

export default function TablePage() {
  const connect = useGame((s) => s.connect);
  const disconnect = useGame((s) => s.disconnect);
  const [params] = useSearchParams();

  useEffect(() => {
    const url = import.meta.env.VITE_WS_URL as string | undefined;
    const join = params.get("join") ?? undefined;
    const token = getStoredToken() ?? undefined;
    // Fixture mode when no real server is configured; the mock speaks the exact
    // same protocol so nothing downstream changes. Against the real server the
    // WsClient auto-sends JoinTable + resync for tableId on every (re)open.
    connect({ url, mock: !url, token, tableId: join });
    return () => disconnect();
    // Connect once on mount; join/token are read from the URL + storage at that
    // moment and the transport handles rejoin on reconnect.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  return <Table />;
}
