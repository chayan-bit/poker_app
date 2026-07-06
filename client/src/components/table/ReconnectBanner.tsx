// A thin top banner shown on socket drop. Auto-resume happens in the client;
// this never folds the player and never blocks the table.

import { memo } from "react";
import { useGame } from "@/store/gameStore";

function ReconnectBannerImpl() {
  const status = useGame((s) => s.status);
  if (status === "open" || status === "closed") return null;

  const text =
    status === "connecting" ? "Connecting..." : "Reconnecting - resuming your seat...";

  return (
    <div
      className="absolute left-0 right-0 z-30 flex items-center justify-center gap-2 py-1.5 text-xs font-medium"
      style={{ top: "env(safe-area-inset-top)", background: "rgba(232,180,76,0.14)", color: "var(--gold)" }}
      role="status"
    >
      <span className="inline-block h-2 w-2 animate-pulse rounded-full bg-current" />
      {text}
    </div>
  );
}

export const ReconnectBanner = memo(ReconnectBannerImpl);
