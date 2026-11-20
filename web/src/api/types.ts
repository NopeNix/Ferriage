// GENERATED from the peer OpenAPI contract — never edit by hand.
// Source: internal/api (GET /openapi.json). Regenerate with
//   node web/scripts/gen-types.mjs
// or keep it honest with: npm run gen:check
//
// Field names and nullability mirror the Go structs; required[]
// in the contract decides which fields are optional here.

export interface Error {
  error: string;
}

export interface Health {
  status: string;
}

export interface Peer {
  peer_id: string;
  version: string;
  commit: string;
}

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

export interface Stacks {
  stacks: Stack[];
}

export interface DeploySvc {
  name: string;
  health: string;
  has_healthcheck: boolean;
  running: boolean;
}

export interface DeployResult {
  stack: string;
  services: DeploySvc[];
  healthy: boolean;
  detail?: string;
}

export interface VolumeDesc {
  name: string;
}

export interface SnapshotManifest {
  format_version: number;
  stack: string;
  created_at: string;
  created_by?: string;
  files: string[];
  volumes?: VolumeDesc[];
}

export interface SyncEntry {
  path: string;
  sha256: string;
  size: number;
  mtime: string;
  updated_at: string;
  peer_id: string;
}

export interface SyncManifest {
  peer_id: string;
  generated_at: string;
  entries: SyncEntry[];
}

export interface OnboardStep {
  name: string;
  ok: boolean;
  detail?: string;
}

export interface OnboardResponse {
  address: string;
  docker: string;
  compose: string;
  container: string;
  peer_url: string;
  healthy: boolean;
  steps: OnboardStep[];
  error?: string;
}

export interface AuditEntry {
  ts: string;
  actor: string;
  action: string;
  target?: string;
  status: number;
  detail?: string;
}

export interface Audit {
  entries: AuditEntry[];
}

export interface ComposeFile {
  name: string;
  file: string;
  content: string;
  compose_file_used?: string;
}

export interface ComposeSave {
  content: string;
}

export interface ContainerStats {
  container: string;
  cpu_percent: number;
  memory_bytes: number;
  memory_limit: number;
  memory_percent: number;
  net_rx_bytes: number;
  net_tx_bytes: number;
}

export interface ExecRequest {
  cmd: string[];
  tty?: boolean;
}

export interface ExecResult {
  exec_id: string;
  running: boolean;
  exit_code: number;
  output: string;
  truncated: boolean;
}

export interface LogLine {
  ts?: string;
  stream: string;
  message: string;
}

export interface Logs {
  container: string;
  lines: LogLine[];
  truncated: boolean;
}

export interface NotifyStatus {
  enabled: boolean;
  channels: number;
  last_event?: string;
  last_at?: string;
  last_error?: string;
}

export interface NotifyTest {
  sent: boolean;
  event: string;
}

export interface OnboardRequest {
  address: string;
  user?: string;
  private_key: string;
  image?: string;
  peer_url?: string;
  port?: number;
}

export interface Stats {
  stats: ContainerStats[];
}
