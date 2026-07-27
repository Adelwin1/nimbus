import { getAccessToken } from "@/lib/token-storage";
import type {
  LiveConnectedEvent,
  LiveDeploymentEvent,
  LiveHealthEvent,
  LiveStreamError,
} from "@/types/deployment";
import type { LiveIncidentEvent } from "@/types/incident";

const API_BASE_URL =
  process.env.NEXT_PUBLIC_API_URL ?? "http://localhost:8080/api/v1";

type LiveEventHandlers = {
  onConnected?: (event: LiveConnectedEvent) => void;
  onHealth?: (event: LiveHealthEvent) => void;
  onDeployment?: (event: LiveDeploymentEvent) => void;
  onIncident?: (event: LiveIncidentEvent) => void;
  onStreamError?: (event: LiveStreamError) => void;
  onConnectionError?: (error: Error) => void;
};

type ParsedSSEEvent = {
  id: string;
  event: string;
  data: string;
};

export function subscribeToApplicationEvents(
  applicationId: string,
  handlers: LiveEventHandlers,
): () => void {
  const controller = new AbortController();

  void runEventStream(applicationId, handlers, controller.signal);

  return () => {
    controller.abort();
  };
}

async function runEventStream(
  applicationId: string,
  handlers: LiveEventHandlers,
  signal: AbortSignal,
): Promise<void> {
  while (!signal.aborted) {
    try {
      await openEventStream(applicationId, handlers, signal);
    } catch (error) {
      if (signal.aborted) {
        return;
      }

      handlers.onConnectionError?.(
        error instanceof Error
          ? error
          : new Error("The live event connection failed."),
      );
    }

    if (!signal.aborted) {
      await delay(2000, signal);
    }
  }
}

async function openEventStream(
  applicationId: string,
  handlers: LiveEventHandlers,
  signal: AbortSignal,
): Promise<void> {
  const token = getAccessToken();

  if (!token) {
    throw new Error("Authentication is required for live events.");
  }

  const response = await fetch(`${API_BASE_URL}/apps/${applicationId}/events`, {
    method: "GET",
    headers: {
      Accept: "text/event-stream",
      Authorization: `Bearer ${token}`,
    },
    cache: "no-store",
    signal,
  });

  if (!response.ok) {
    throw new Error(await readErrorMessage(response));
  }

  if (!response.body) {
    throw new Error("The live event response did not include a stream.");
  }

  const reader = response.body.getReader();
  const decoder = new TextDecoder();

  let buffer = "";

  try {
    while (!signal.aborted) {
      const { value, done } = await reader.read();

      if (done) {
        break;
      }

      buffer += decoder.decode(value, {
        stream: true,
      });

      buffer = buffer.replace(/\r\n/g, "\n");

      let separatorIndex = buffer.indexOf("\n\n");

      while (separatorIndex !== -1) {
        const block = buffer.slice(0, separatorIndex);

        buffer = buffer.slice(separatorIndex + 2);

        const event = parseSSEBlock(block);

        if (event) {
          dispatchSSEEvent(event, handlers);
        }

        separatorIndex = buffer.indexOf("\n\n");
      }
    }
  } finally {
    reader.releaseLock();
  }

  if (!signal.aborted) {
    throw new Error("The live event connection closed.");
  }
}

function parseSSEBlock(block: string): ParsedSSEEvent | null {
  if (!block.trim()) {
    return null;
  }

  const lines = block.split("\n");

  let id = "";
  let event = "message";
  const dataLines: string[] = [];

  for (const line of lines) {
    if (line.startsWith(":")) {
      continue;
    }

    if (line.startsWith("id:")) {
      id = line.slice(3).trimStart();
      continue;
    }

    if (line.startsWith("event:")) {
      event = line.slice(6).trimStart();
      continue;
    }

    if (line.startsWith("data:")) {
      dataLines.push(line.slice(5).trimStart());
    }
  }

  if (dataLines.length === 0) {
    return null;
  }

  return {
    id,
    event,
    data: dataLines.join("\n"),
  };
}

function dispatchSSEEvent(
  event: ParsedSSEEvent,
  handlers: LiveEventHandlers,
): void {
  try {
    const data: unknown = JSON.parse(event.data);

    switch (event.event) {
      case "connected":
        handlers.onConnected?.(data as LiveConnectedEvent);
        break;

      case "health":
        handlers.onHealth?.(data as LiveHealthEvent);
        break;

      case "deployment":
        handlers.onDeployment?.(data as LiveDeploymentEvent);
        break;

      case "incident":
        handlers.onIncident?.(data as LiveIncidentEvent);
        break;

      case "stream_error":
        handlers.onStreamError?.(data as LiveStreamError);
        break;

      default:
        break;
    }
  } catch {
    handlers.onConnectionError?.(
      new Error(`Nimbus received an invalid ${event.event} event.`),
    );
  }
}

async function readErrorMessage(response: Response): Promise<string> {
  try {
    const payload = (await response.json()) as {
      error?: {
        message?: string;
      };
    };

    return (
      payload.error?.message ??
      `Live events failed with HTTP ${response.status}.`
    );
  } catch {
    return `Live events failed with HTTP ${response.status}.`;
  }
}

function delay(milliseconds: number, signal: AbortSignal): Promise<void> {
  return new Promise((resolve) => {
    const timeout = window.setTimeout(resolve, milliseconds);

    signal.addEventListener(
      "abort",
      () => {
        window.clearTimeout(timeout);
        resolve();
      },
      {
        once: true,
      },
    );
  });
}
