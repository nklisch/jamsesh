import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen } from '@testing-library/svelte';
import WsStatusBanner from './WsStatusBanner.svelte';

// Mock the wsStatus rune store with a module-scoped mutable map. The
// component re-reads the status via `wsStatus.for(sessionId)` on each
// mount, so transitions are exercised by unmounting and re-rendering.
const _statuses = new Map<string, 'connecting' | 'open' | 'reconnecting' | null>();

vi.mock('$lib/ws.svelte', () => ({
  wsStatus: {
    for(sessionId: string) {
      return _statuses.get(sessionId) ?? null;
    },
  },
}));

describe('WsStatusBanner', () => {
  beforeEach(() => {
    _statuses.clear();
  });

  afterEach(() => {
    _statuses.clear();
  });

  it('renders nothing when status is null (no subscription)', () => {
    const { container } = render(WsStatusBanner, { props: { sessionId: 'sess-1' } });
    expect(screen.queryByRole('status')).toBeNull();
    expect(container.querySelector('.ws-status')).toBeNull();
  });

  it("renders nothing when status is 'open'", () => {
    _statuses.set('sess-1', 'open');
    const { container } = render(WsStatusBanner, { props: { sessionId: 'sess-1' } });
    expect(screen.queryByRole('status')).toBeNull();
    expect(container.querySelector('.ws-status')).toBeNull();
  });

  it("renders nothing when status is 'connecting'", () => {
    _statuses.set('sess-1', 'connecting');
    const { container } = render(WsStatusBanner, { props: { sessionId: 'sess-1' } });
    expect(screen.queryByRole('status')).toBeNull();
    expect(container.querySelector('.ws-status')).toBeNull();
  });

  it("renders a role=status banner with 'Reconnecting…' when status is 'reconnecting'", () => {
    _statuses.set('sess-1', 'reconnecting');
    render(WsStatusBanner, { props: { sessionId: 'sess-1' } });

    const banner = screen.getByRole('status');
    expect(banner).toBeInTheDocument();
    expect(banner).toHaveTextContent(/reconnecting/i);
  });

  it('announces politely (aria-live="polite") when reconnecting', () => {
    _statuses.set('sess-1', 'reconnecting');
    render(WsStatusBanner, { props: { sessionId: 'sess-1' } });

    const banner = screen.getByRole('status');
    expect(banner).toHaveAttribute('aria-live', 'polite');
  });

  it('reads status for the supplied sessionId, not a sibling session', () => {
    _statuses.set('sess-other', 'reconnecting');
    _statuses.set('sess-1', 'open');
    render(WsStatusBanner, { props: { sessionId: 'sess-1' } });

    // sess-1 is open → banner absent, even though sess-other is reconnecting.
    expect(screen.queryByRole('status')).toBeNull();
  });

  it("banner disappears when status transitions from 'reconnecting' back to 'open'", () => {
    // Reconnecting state — banner is present.
    _statuses.set('sess-1', 'reconnecting');
    const initial = render(WsStatusBanner, { props: { sessionId: 'sess-1' } });
    expect(screen.getByRole('status')).toBeInTheDocument();
    initial.unmount();

    // Transition: socket reopened.
    _statuses.set('sess-1', 'open');
    const next = render(WsStatusBanner, { props: { sessionId: 'sess-1' } });
    expect(screen.queryByRole('status')).toBeNull();
    expect(next.container.querySelector('.ws-status')).toBeNull();
  });
});
