import type { DragEvent } from "react";

export const PATH_TYPE = "application/x-reasonix-path";

export function workspaceDrag(root: string | undefined, path: string) {
  return {
    draggable: !!root,
    onDragStart: (event: DragEvent<HTMLElement>) => {
      if (!root) return;
      const absolute = root.replace(/[\\/]+$/, "") + "/" + path;
      event.dataTransfer.effectAllowed = "copy";
      event.dataTransfer.setData(PATH_TYPE, absolute);
      event.dataTransfer.setData("text/plain", absolute);
    },
  };
}
