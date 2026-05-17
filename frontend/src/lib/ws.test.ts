// Tests for the WebSocket subscription manager.
//
// Mocks the global WebSocket constructor so tests run without a live
// server. Verifies:
//   - subscribe() opens exactly one socket per sessionId
//   - A second subscribe() for the same sessionId reuses the socket
//   - Messages of a matching type fire the handler
//   - Messages of a non-matching type do NOT fire the handler
//   - The unsubscribe closure removes the handler
//   - close() tears down the socket and handler registry

import { describe, test, expect, beforeEach, vi, afterEach } from 'vitest';

// ------------------------------------------------------------------
// MockWebSocket — a synchronous test double for global.WebSocket
// ------------------------------------------------------------------

type EventMap = {
  message: MessageEvent[];
  close: CloseEvent[];
  open: Event[];
  error: Event[];
};

class MockWebSocket {
  static instances: MockWebSocket[] = [];

  readonly url: string;
  readonly protocol: string;
  readyState: number = WebSocket.CONNECTING;

  private listeners: { [K in keyof EventMap]?: Array<(e: EventMap[K][number]) => void> } = {};

  constructor(url: string, protocol?: string | string[]) {
    this.url = url;
    this.protocol = Array.isArray(protocol) ? protocol[0] : (protocol ?? '');
    MockWebSocket.instances.push(this);
  }

  addEventListener<K extends keyof EventMap>(
    type: K,
    listener: (e: EventMap[K][number]) => void,
  ): void {
    if (!this.listeners[type]) this.listeners[type] = [] as any;
    (this.listeners[type] as any[]).push(listener);
  }

  removeEventListener<K extends keyof EventMap>(
    type: K,
    listener: (e: EventMap[K][number]) => void,
  ): void {
    if (!this.listeners[type]) return;
    this.listeners[type] = (this.listeners[type] as any[]).filter(
      (l: any) => l !== listener,
    ) as any;
  }

  /** Trigger a fake message event (test helper). */
  emit(type: 'message', data: unknown): void;
  emit(type: 'close'): void;
  emit(type: string, data?: unknown): void {
    if (type === 'message') {
      const ev = new MessageEvent('message', { data: JSON.stringify(data) });
      ((this.listeners['message'] as any[]) ?? []).forEach((l) => l(ev));
    } else if (type === 'close') {
      const ev = new CloseEvent('close');
      ((this.listeners['close'] as any[]) ?? []).forEach((l) => l(ev));
    }
  }

  close(): void {
    this.readyState = WebSocket.CLOSED;
    this.emit('close');
  }

  // Satisfy the WebSocket interface for TS — unused in tests.
  send(_data: string | ArrayBuffer | Blob): void {}
  onopen: ((ev: Event) => void) | null = null;
  onclose: ((ev: CloseEvent) => void) | null = null;
  onmessage: ((ev: MessageEvent) => void) | null = null;
  onerror: ((ev: Event) => void) | null = null;
  readonly CONNECTING = 0 as const;
  readonly OPEN = 1 as const;
  readonly CLOSING = 2 as const;
  readonly CLOSED = 3 as const;
  readonly bufferedAmount = 0;
  readonly extensions = '';
  readonly binaryType: BinaryType = 'blob';
  readonly dispatchEvent = (_e: Event) => true;
}

// ------------------------------------------------------------------
// Fixtures
// ------------------------------------------------------------------

function installMockWebSocket() {
  MockWebSocket.instances = [];
  vi.stubGlobal('WebSocket', MockWebSocket);
}

function uninstallMockWebSocket() {
  vi.unstubAllGlobals();
}

// ------------------------------------------------------------------
// Tests
// ------------------------------------------------------------------

describe('ws — subscribe / lifecycle', () => {
  beforeEach(async () => {
    installMockWebSocket();
    localStorage.clear();
    vi.resetModules();

    // Prime auth with a token so open() doesn't throw.
    const { auth } = await import('$lib/auth.svelte');
    auth.setTokens('test-token', 'test-refresh');
  });

  afterEach(() => {
    uninstallMockWebSocket();
    vi.restoreAllMocks();
  });

  test('subscribe() opens a WebSocket for the sessionId', async () => {
    const { subscribe } = await import('$lib/ws.svelte');

    const unsub = subscribe('sess-1', 'commit.arrived', vi.fn());

    expect(MockWebSocket.instances).toHaveLength(1);
    expect(MockWebSocket.instances[0].url).toBe('/ws/sessions/sess-1');

    unsub();
  });

  test('subscribe() uses jamsesh.bearer.<token> sub-protocol', async () => {
    const { subscribe } = await import('$lib/ws.svelte');

    const unsub = subscribe('sess-2', 'any', vi.fn());

    expect(MockWebSocket.instances[0].protocol).toBe('jamsesh.bearer.test-token');

    unsub();
  });

  test('second subscribe() to same sessionId reuses the socket', async () => {
    const { subscribe } = await import('$lib/ws.svelte');

    const unsub1 = subscribe('sess-3', 'event.a', vi.fn());
    const unsub2 = subscribe('sess-3', 'event.b', vi.fn());

    // Still only one WebSocket instance opened.
    expect(MockWebSocket.instances).toHaveLength(1);

    unsub1();
    unsub2();
  });

  test('second subscribe() to different sessionId opens a new socket', async () => {
    const { subscribe } = await import('$lib/ws.svelte');

    const unsub1 = subscribe('sess-4a', 'ev', vi.fn());
    const unsub2 = subscribe('sess-4b', 'ev', vi.fn());

    expect(MockWebSocket.instances).toHaveLength(2);

    unsub1();
    unsub2();
  });

  test('message of matching type fires the handler', async () => {
    const { subscribe } = await import('$lib/ws.svelte');

    const handler = vi.fn();
    const unsub = subscribe('sess-5', 'commit.arrived', handler);

    const ws = MockWebSocket.instances[0];
    ws.emit('message', { type: 'commit.arrived', sha: 'abc123' });

    expect(handler).toHaveBeenCalledTimes(1);
    expect(handler).toHaveBeenCalledWith({ type: 'commit.arrived', sha: 'abc123' });

    unsub();
  });

  test('message of non-matching type does not fire the handler', async () => {
    const { subscribe } = await import('$lib/ws.svelte');

    const handler = vi.fn();
    const unsub = subscribe('sess-6', 'commit.arrived', handler);

    const ws = MockWebSocket.instances[0];
    ws.emit('message', { type: 'merge.succeeded', draft_sha: 'def456' });

    expect(handler).not.toHaveBeenCalled();

    unsub();
  });

  test('unsubscribe closure removes the handler — no more calls after unsub', async () => {
    const { subscribe } = await import('$lib/ws.svelte');

    const handler = vi.fn();
    const unsub = subscribe('sess-7', 'some.event', handler);

    const ws = MockWebSocket.instances[0];
    ws.emit('message', { type: 'some.event', payload: 1 });
    expect(handler).toHaveBeenCalledTimes(1);

    unsub();

    ws.emit('message', { type: 'some.event', payload: 2 });
    // Handler removed — should not have been called again.
    expect(handler).toHaveBeenCalledTimes(1);
  });

  test('multiple handlers for the same type all receive the message', async () => {
    const { subscribe } = await import('$lib/ws.svelte');

    const h1 = vi.fn();
    const h2 = vi.fn();
    const unsub1 = subscribe('sess-8', 'tick', h1);
    const unsub2 = subscribe('sess-8', 'tick', h2);

    MockWebSocket.instances[0].emit('message', { type: 'tick' });

    expect(h1).toHaveBeenCalledTimes(1);
    expect(h2).toHaveBeenCalledTimes(1);

    unsub1();
    unsub2();
  });

  test('close() closes the socket and clears handlers', async () => {
    const { subscribe, close } = await import('$lib/ws.svelte');

    const handler = vi.fn();
    subscribe('sess-9', 'ev', handler);

    const ws = MockWebSocket.instances[0];
    close('sess-9');

    // After close, emitting on the raw ws must not reach handler because
    // the handler registry was deleted.
    ws.emit('message', { type: 'ev' });
    expect(handler).not.toHaveBeenCalled();
  });

  test('socket close event causes a new socket on next subscribe', async () => {
    const { subscribe } = await import('$lib/ws.svelte');

    const unsub1 = subscribe('sess-10', 'ev', vi.fn());
    expect(MockWebSocket.instances).toHaveLength(1);

    // Simulate the server closing the connection.
    MockWebSocket.instances[0].close();

    // Now subscribe again — should open a fresh socket.
    const unsub2 = subscribe('sess-10', 'ev', vi.fn());
    expect(MockWebSocket.instances).toHaveLength(2);

    unsub1();
    unsub2();
  });

  test('malformed JSON messages are silently dropped', async () => {
    const { subscribe } = await import('$lib/ws.svelte');

    const handler = vi.fn();
    const unsub = subscribe('sess-11', 'ev', handler);

    const ws = MockWebSocket.instances[0];
    // Emit a raw MessageEvent with invalid JSON directly.
    const badEv = new MessageEvent('message', { data: '{not json}}}' });
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    ((ws as any).listeners['message'] ?? []).forEach((l: any) => l(badEv));

    expect(handler).not.toHaveBeenCalled();

    unsub();
  });
});
