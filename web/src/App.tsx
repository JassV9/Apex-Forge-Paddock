import { Workspace } from "./components/workspace/Workspace";
import { READ_ONLY } from "./config";
import { useRoute } from "./hooks/useRoute";

/** `readOnly` is the public demo: recorded data, and nothing a visitor can change. */
export function App({ readOnly = READ_ONLY }: { readOnly?: boolean }) {
  return <Workspace route={useRoute()} readOnly={readOnly} />;
}
