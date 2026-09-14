// Event contract (contract-first, plan §4.3-2).
// Go side mirrors these constants in internal/events/bus.go.

export const EVENTS = {
  CONNECTION_TEST: 'connection:test',
  ERROR: 'app:error',
  CONSOLE_MONITOR: 'console:monitor', // [sessionID, string[]]
  PUBSUB_MESSAGE: 'pubsub:message',   // [connID, channel, [kind, channel, payload]]
  BULK_PROGRESS: 'bulk:progress',     // [runID, done, total, msg]
  BULK_DONE: 'bulk:done',             // [runID, summary]
} as const

export interface ConnectionTestEvent {
  id: string
  status: string
}

export interface ErrorEvent {
  source: string
  message: string
}

// --- P1 tree bindings (mirror of Go app package types) ----------------------

export interface DBInfo {
  index: number
  keys: number
  label?: string
}

export interface ConnSummary {
  id: string
  mode: 'standalone' | 'cluster' | 'sentinel'
  version: string
  address: string
  databases: DBInfo[]
}

export interface TreeNode {
  name: string
  fullPath: string
  isNamespace: boolean
  keyType?: string
  count?: number
  children?: TreeNode[]
}

export interface TreeResult {
  nodes: TreeNode[]
  total: number
  complete: boolean
}
