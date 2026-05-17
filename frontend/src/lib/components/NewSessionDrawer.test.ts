import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/svelte';
import NewSessionDrawer from './NewSessionDrawer.svelte';
import type { components } from '$lib/api/types.gen';

type Session = components['schemas']['Session'];

// ── Module mocks ─────────────────────────────────────────────────────────────

const mockPOST = vi.fn();
vi.mock('$lib/api/client', () => ({
  client: { GET: vi.fn(), POST: (...args: unknown[]) => mockPOST(...args) },
}));

vi.mock('$lib/auth.svelte', () => ({
  auth: {
    currentUser: null,
    isAuthenticated: true,
    token: 'test-token',
  },
}));

// ── Fixtures ─────────────────────────────────────────────────────────────────

function makeSession(): Session {
  return {
    id: 'sess-new',
    org_id: 'org-1',
    name: 'New Session',
    goal: 'A goal',
    scope: JSON.stringify(['src/**']),
    default_mode: 'sync',
    status: 'active',
    created_at: new Date().toISOString(),
    members: [],
  };
}

// ── Tests ─────────────────────────────────────────────────────────────────────

describe('NewSessionDrawer', () => {
  const onCreated = vi.fn();
  const onClose = vi.fn();

  beforeEach(() => {
    vi.clearAllMocks();
  });

  afterEach(() => {
    vi.clearAllMocks();
  });

  function renderDrawer() {
    return render(NewSessionDrawer, {
      props: { orgId: 'org-1', oncreated: onCreated, onclose: onClose },
    });
  }

  it('renders the drawer heading', () => {
    renderDrawer();
    expect(screen.getByRole('heading', { name: /new session/i })).toBeInTheDocument();
  });

  it('renders form fields', () => {
    renderDrawer();
    expect(screen.getByLabelText(/name/i)).toBeInTheDocument();
    expect(screen.getByLabelText(/goal/i)).toBeInTheDocument();
    expect(screen.getByLabelText(/scope/i)).toBeInTheDocument();
    expect(screen.getByRole('group', { name: /default mode/i })).toBeInTheDocument();
  });

  it('calls onclose when Cancel is clicked', async () => {
    renderDrawer();
    await fireEvent.click(screen.getByRole('button', { name: /cancel/i }));
    expect(onClose).toHaveBeenCalledOnce();
  });

  it('calls onclose when close button (✕) is clicked', async () => {
    renderDrawer();
    await fireEvent.click(screen.getByRole('button', { name: /close/i }));
    expect(onClose).toHaveBeenCalledOnce();
  });

  it('calls onclose when Escape is pressed', async () => {
    renderDrawer();
    await fireEvent.keyDown(window, { key: 'Escape' });
    expect(onClose).toHaveBeenCalledOnce();
  });

  it('submit button is disabled when name is empty', () => {
    renderDrawer();
    const submitBtn = screen.getByRole('button', { name: /create session/i });
    expect(submitBtn).toBeDisabled();
  });

  it('submit button is enabled when name is filled in', async () => {
    renderDrawer();
    await fireEvent.input(screen.getByLabelText(/name/i), { target: { value: 'My Session' } });
    const submitBtn = screen.getByRole('button', { name: /create session/i });
    expect(submitBtn).not.toBeDisabled();
  });

  it('POSTs to the correct endpoint on submit', async () => {
    mockPOST.mockResolvedValue({ data: makeSession(), error: null });
    renderDrawer();

    await fireEvent.input(screen.getByLabelText(/name/i), { target: { value: 'Auth Fix' } });
    await fireEvent.input(screen.getByLabelText(/goal/i), { target: { value: 'Fix the auth flow' } });
    await fireEvent.input(screen.getByLabelText(/scope/i), { target: { value: 'src/auth/**, tests/**' } });

    await fireEvent.submit(screen.getByRole('button', { name: /create session/i }).closest('form')!);

    await waitFor(() => {
      expect(mockPOST).toHaveBeenCalledWith('/api/orgs/{orgID}/sessions', {
        params: { path: { orgID: 'org-1' } },
        body: {
          name: 'Auth Fix',
          goal: 'Fix the auth flow',
          scope: JSON.stringify(['src/auth/**', 'tests/**']),
          default_mode: 'sync',
        },
      });
    });
  });

  it('calls oncreated with the new session on success', async () => {
    const session = makeSession();
    mockPOST.mockResolvedValue({ data: session, error: null });
    renderDrawer();

    await fireEvent.input(screen.getByLabelText(/name/i), { target: { value: 'My Session' } });
    await fireEvent.submit(screen.getByRole('button', { name: /create session/i }).closest('form')!);

    await waitFor(() => {
      expect(onCreated).toHaveBeenCalledWith(session);
    });
  });

  it('shows an error message on POST failure', async () => {
    mockPOST.mockResolvedValue({ data: null, error: { message: 'Server error' } });
    renderDrawer();

    await fireEvent.input(screen.getByLabelText(/name/i), { target: { value: 'My Session' } });
    await fireEvent.submit(screen.getByRole('button', { name: /create session/i }).closest('form')!);

    await waitFor(() => {
      expect(screen.getByRole('alert')).toHaveTextContent(/failed to create session/i);
    });
  });

  it('does not call oncreated on failure', async () => {
    mockPOST.mockResolvedValue({ data: null, error: { message: 'Bad' } });
    renderDrawer();

    await fireEvent.input(screen.getByLabelText(/name/i), { target: { value: 'My Session' } });
    await fireEvent.submit(screen.getByRole('button', { name: /create session/i }).closest('form')!);

    await waitFor(() => expect(screen.getByRole('alert')).toBeInTheDocument());

    expect(onCreated).not.toHaveBeenCalled();
  });

  it('toggles default_mode between sync and isolated', async () => {
    renderDrawer();
    const isolatedBtn = screen.getByRole('button', { name: /isolated/i });
    const syncBtn = screen.getByRole('button', { name: /^sync/i });

    expect(syncBtn).toHaveAttribute('aria-pressed', 'true');
    expect(isolatedBtn).toHaveAttribute('aria-pressed', 'false');

    await fireEvent.click(isolatedBtn);

    expect(isolatedBtn).toHaveAttribute('aria-pressed', 'true');
    expect(syncBtn).toHaveAttribute('aria-pressed', 'false');
  });

  it('parses comma-separated scope into a JSON array', async () => {
    mockPOST.mockResolvedValue({ data: makeSession(), error: null });
    renderDrawer();

    await fireEvent.input(screen.getByLabelText(/name/i), { target: { value: 'Test' } });
    await fireEvent.input(screen.getByLabelText(/scope/i), { target: { value: 'src/**, docs/**' } });
    await fireEvent.submit(screen.getByRole('button', { name: /create session/i }).closest('form')!);

    await waitFor(() => {
      const call = mockPOST.mock.calls[0] as unknown[];
      const body = (call[1] as { body: { scope: string } }).body;
      expect(JSON.parse(body.scope)).toEqual(['src/**', 'docs/**']);
    });
  });
});
