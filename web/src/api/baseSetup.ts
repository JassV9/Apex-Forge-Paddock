import { coreApi, unwrap } from "./client";
import type { components } from "./schema";

export type BaseSetupReply = components["schemas"]["BaseSetupReply"];
export type BaseSetup = BaseSetupReply["base_setup"];
export type BaseSetupChange = BaseSetup["changes"][number];
export type BaseSetupDriver = components["schemas"]["BaseSetupCreate"]["driver"];

/**
 * Builds a base setup from what the session's car measured. Core answers 409 when nothing
 * qualifies; that reply says what the car measured, so it is returned like a success.
 */
export async function buildBaseSetup(
  sessionId: number,
  driver: BaseSetupDriver,
): Promise<BaseSetupReply> {
  const reply = await coreApi.POST("/sessions/{id}/base-setup", {
    params: { path: { id: sessionId } },
    body: { driver },
  });
  if (reply.response.status === 409 && reply.error !== undefined) {
    return reply.error as BaseSetupReply;
  }
  return unwrap(reply, "POST /sessions/{id}/base-setup");
}
