import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, waitFor } from '@testing-library/svelte';
import { fireEvent } from '@testing-library/svelte';
import OrgSettings from './OrgSettings.svelte';
import type { components } from '$lib/api/types.gen';

type Org = components['schemas']['Org'];
type MemberRef = components['schemas']['MemberRef'];

// ── Module mocks ─────────────────────────────────────────────────────────────

vi.mock('$lib/auth.svelte', () => ({
  auth: {
    currentUser: { id: 'user-1', email: 'creator@example.com', displayName: 'Creator' },
    isAuthenticated: true,
    signOut: vi.fn(),
  },
}));

vi.mock('$lib/router.svelte', () => ({
  current: { name: 'org-settings', params: { orgId: 'org-1' } },
  navigate: vi.fn(),
}));

const mockGET = vi.fn();
const mockPATCH = vi.fn();

vi.mock('$lib/api/client', () => ({
  client: {
    GET: (...args: unknown[]) => mockGET(...args),
    PATCH: (...args: unknown[]) => mockPATCH(...args),
  },
}));

vi.mock('$lib/components/AuthorDot.svelte', () => ({
  default: vi.fn().mockReturnValue({ render: vi.fn() }),
}));

// ── Fixtures ──────────────────────────────────────────────────────────────────

function makeOrg(overrides: Partial<Org> = {}): Org {
  return {
    id: 'org-1',
    name: 'Acme Corp',
    slug: 'acme',
    session_invite_policy: 'members_only',
    ...overrides,
  };
}

function makeMembers(role: 'creator' | 'member' = 'creator'): MemberRef[] {
  return [
    {
      account_id: 'user-1',
      email: 'creator@example.com' as unknown as MemberRef['email'],
      display_name: 'Creator',
      role,
      joined_at: new Date().toISOString(),
    },
  ];
}

function setupMocks(orgOverrides: Partial<Org> = {}, memberRole: 'creator' | 'member' = 'creator') {
  mockGET.mockImplementation((path: string) => {
    if (path === '/api/orgs/{orgID}') {
      return Promise.resolve({ data: makeOrg(orgOverrides), error: null });
    }
    if (path === '/api/orgs/{orgID}/members') {
      return Promise.resolve({ data: makeMembers(memberRole), error: null });
    }
    return Promise.resolve({ data: null, error: { message: 'Unexpected GET' } });
  });
}

// ── Tests ─────────────────────────────────────────────────────────────────────

describe('OrgSettings', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('renders a loading state before data arrives', () => {
    // Never resolve so we stay in loading
    mockGET.mockReturnValue(new Promise(() => {}));
    render(OrgSettings, { props: { orgId: 'org-1' } });
    expect(screen.getByText(/loading/i)).toBeInTheDocument();
  });

  it('renders the pane heading once loaded', async () => {
    setupMocks();
    render(OrgSettings, { props: { orgId: 'org-1' } });
    await waitFor(() => {
      expect(screen.getByRole('heading', { name: /session invites/i })).toBeInTheDocument();
    });
  });

  it('admin sees editable radios and an enabled Save button', async () => {
    setupMocks({}, 'creator');
    render(OrgSettings, { props: { orgId: 'org-1' } });

    await waitFor(() => {
      expect(screen.getByRole('heading', { name: /session invites/i })).toBeInTheDocument();
    });

    const membersRadio = screen.getByRole('radio', { name: /members only/i });
    const openRadio = screen.getByRole('radio', { name: /open/i });
    expect(membersRadio).not.toBeDisabled();
    expect(openRadio).not.toBeDisabled();

    // Save is disabled when not dirty (nothing changed yet)
    const saveBtn = screen.getByRole('button', { name: /save changes/i });
    expect(saveBtn).toBeDisabled();

    // No readonly banner for admins
    expect(screen.queryByText(/only org creators can change/i)).not.toBeInTheDocument();
  });

  it('Save button becomes enabled when admin changes policy', async () => {
    setupMocks({}, 'creator');
    render(OrgSettings, { props: { orgId: 'org-1' } });

    await waitFor(() => {
      expect(screen.getByRole('heading', { name: /session invites/i })).toBeInTheDocument();
    });

    const openRadio = screen.getByRole('radio', { name: /open/i });
    await fireEvent.click(openRadio);

    const saveBtn = screen.getByRole('button', { name: /save changes/i });
    expect(saveBtn).not.toBeDisabled();
  });

  it('non-admin sees disabled radios, disabled Save, and a warning banner', async () => {
    setupMocks({}, 'member');
    render(OrgSettings, { props: { orgId: 'org-1' } });

    await waitFor(() => {
      expect(screen.getByRole('heading', { name: /session invites/i })).toBeInTheDocument();
    });

    const membersRadio = screen.getByRole('radio', { name: /members only/i });
    const openRadio = screen.getByRole('radio', { name: /open/i });
    expect(membersRadio).toBeDisabled();
    expect(openRadio).toBeDisabled();

    const saveBtn = screen.getByRole('button', { name: /save changes/i });
    expect(saveBtn).toBeDisabled();

    expect(
      screen.getByText(/only org creators can change this setting/i),
    ).toBeInTheDocument();
  });

  it('Save calls PATCH and updates local state on 200', async () => {
    setupMocks({ session_invite_policy: 'members_only' }, 'creator');
    const updatedOrg = makeOrg({ session_invite_policy: 'open' });
    mockPATCH.mockResolvedValue({ data: updatedOrg, error: null });

    render(OrgSettings, { props: { orgId: 'org-1' } });

    await waitFor(() => {
      expect(screen.getByRole('heading', { name: /session invites/i })).toBeInTheDocument();
    });

    // Select "open"
    const openRadio = screen.getByRole('radio', { name: /open/i });
    await fireEvent.click(openRadio);

    const saveBtn = screen.getByRole('button', { name: /save changes/i });
    await fireEvent.click(saveBtn);

    await waitFor(() => {
      expect(mockPATCH).toHaveBeenCalledWith('/api/orgs/{orgID}', {
        params: { path: { orgID: 'org-1' } },
        body: { session_invite_policy: 'open' },
      });
    });

    // Transient success message should appear
    await waitFor(() => {
      expect(screen.getByRole('status')).toBeInTheDocument();
      expect(screen.getByRole('status')).toHaveTextContent('Saved');
    });

    // After save, policy is no longer dirty
    await waitFor(() => {
      const btn = screen.getByRole('button', { name: /save changes/i });
      expect(btn).toBeDisabled();
    });
  });

  it('Save error surfaces in banner', async () => {
    setupMocks({}, 'creator');
    mockPATCH.mockResolvedValue({
      data: null,
      error: { message: 'Server exploded', error: 'server.error' },
    });

    render(OrgSettings, { props: { orgId: 'org-1' } });

    await waitFor(() => {
      expect(screen.getByRole('heading', { name: /session invites/i })).toBeInTheDocument();
    });

    // Flip to open to make dirty
    await fireEvent.click(screen.getByRole('radio', { name: /open/i }));
    await fireEvent.click(screen.getByRole('button', { name: /save changes/i }));

    await waitFor(() => {
      expect(screen.getByText(/server exploded/i)).toBeInTheDocument();
    });
  });

  it('Save 403 error shows "only org creators" message in banner', async () => {
    setupMocks({}, 'creator');
    mockPATCH.mockResolvedValue({
      data: null,
      error: { error: 'auth.insufficient_permission', message: 'only the org creator can modify org settings' },
    });

    render(OrgSettings, { props: { orgId: 'org-1' } });

    await waitFor(() => {
      expect(screen.getByRole('heading', { name: /session invites/i })).toBeInTheDocument();
    });

    await fireEvent.click(screen.getByRole('radio', { name: /open/i }));
    await fireEvent.click(screen.getByRole('button', { name: /save changes/i }));

    await waitFor(() => {
      expect(screen.getByText(/only org creators can change this setting/i)).toBeInTheDocument();
    });
  });

  it('load error is shown when GET /api/orgs fails', async () => {
    mockGET.mockImplementation((path: string) => {
      if (path === '/api/orgs/{orgID}') {
        return Promise.resolve({ data: null, error: { message: 'Network failure' } });
      }
      return Promise.resolve({ data: makeMembers(), error: null });
    });

    render(OrgSettings, { props: { orgId: 'org-1' } });

    await waitFor(() => {
      expect(screen.getByRole('alert')).toBeInTheDocument();
      expect(screen.getByText(/failed to load org settings/i)).toBeInTheDocument();
    });
  });

  it('renders sidebar nav with active Session invites link', async () => {
    setupMocks();
    render(OrgSettings, { props: { orgId: 'org-1' } });

    await waitFor(() => {
      expect(screen.getByRole('heading', { name: /session invites/i })).toBeInTheDocument();
    });

    // The active link
    const activeLink = screen.getByRole('link', { name: /session invites/i });
    expect(activeLink).toHaveClass('active');

    // Dimmed "soon" links
    expect(screen.getByRole('link', { name: /members/i })).toHaveClass('dim');
    expect(screen.getByRole('link', { name: /billing/i })).toHaveClass('dim');
    expect(screen.getByRole('link', { name: /api keys/i })).toHaveClass('dim');
  });
});
