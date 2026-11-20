// Typed view of the peer HTTP API (CONCEPT: API-first). Shapes mirror
// internal/adopt.Stack and the /peer handler — keep in sync by hand
// until the typed-API pass (Phase 3) generates them.

export interface Service {
  name: string;
  container: string;
  image: string;
  state: string;
  running: boolean;
}

export interface Stack {
  name: string;
  compose_file?: string;
  working_dir?: string;
  services: Service[];
  running_count: number;
  container_count: number;
}

export interface StacksResponse {
  stacks: Stack[];
}

export interface PeerInfo {
  peer_id: string;
  version: string;
  commit: string;
}
