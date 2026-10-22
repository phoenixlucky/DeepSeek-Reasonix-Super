import { useSyncExternalStore } from "react";
import { onModelFilterChange, readModelFilter } from "./prefs";

export { MODEL_FILTER_KEY, writeModelFilter } from "./prefs";

export function useModelFilter(): readonly string[] {
  return useSyncExternalStore(onModelFilterChange, readModelFilter, readModelFilter);
}
