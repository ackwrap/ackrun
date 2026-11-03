import { request } from "./api";
import type { ActionResponse } from "./types";
import type { NodeExposure, NodeExposureRequest } from "./advancedTypes";

export const advancedApi = {
  getNodeExposures: () => request<NodeExposure[]>("/advanced/node-exposures"),
  createNodeExposure: (body: NodeExposureRequest) =>
    request<NodeExposure>("/advanced/node-exposures", {
      method: "POST",
      body: JSON.stringify(body),
    }),
  updateNodeExposure: (id: number, body: NodeExposureRequest) =>
    request<NodeExposure>(`/advanced/node-exposures/${id}`, {
      method: "PUT",
      body: JSON.stringify(body),
    }),
  deleteNodeExposure: (id: number) =>
    request<ActionResponse>(`/advanced/node-exposures/${id}`, {
      method: "DELETE",
    }),
};
