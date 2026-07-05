// Connects the socket (real server if VITE_WS_URL is set, else the mock/fixture
// server so the table is demoable) and renders the table. Join params come from
// the URL (?join=CODE from an invite/Quick Seat). This is the priority route.

import { useEffect } from "react";
import { useSearchParams } from "react-router-dom";
import { useGame } from "@/store/gameStore";
import Table from "./Table";

export default function TablePage() {
  const connect = useGame((s) => s.connect);
  const disconnect = useGame((s) => s.disconnect);
  const [params] = useSearchParams();

  useEffect(() => {
    const url = import.meta.env.VITE_WS_URL as string | undefined;
    // Fixture mode when no real server is configured; the mock speaks the exact
    // same protocol so nothing downstream changes.
    connect({ url, mock: !url });
    return () => disconnect();
    // join code is read by the transport layer when wired to the real server;
    // the mock always seats you at the demo table.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  void params;

  return <Table />;
}
