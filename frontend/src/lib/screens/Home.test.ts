// Home.test.ts
// Tests: loading state, empty state, single-org auto-route, picker (multi-org),
// create-org flow (success + error + empty name), sign-out.

import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, fireEvent, waitFor, cleanup } from '@testing-library/svelte';
import Home from './Home.svelte';

// ── Module mocks ────────────────────────────────────────────────────────────

const mockPOST = vi.fn();

vi.mock('$lib/api/client', () => ({
  client: {
    POST: (...args: unknown[]) => mockPOST(...args),
  },
}));

const mockNavigate = vi.fn();
vi.mock('$lib/router.svelte', () => ({
  navigate: (...args: unknown[]) => mockNavigate(...args),
}));

// Mutable auth mock — tests override `orgs` / `currentUser` per scenario.
let mockOrgs: { id: string; name: string; slug: string; role: string }[] | null = null;
let mockCurrentUser: { id: string; email: string; displayName: string } | null = null;
const mockSignOut = vi.fn();
const mockAddOrg = vi.fn();

vi.mock('$lib/auth.svelte', () => ({
  auth: {
    get orgs() { return mockOrgs; },
    get currentUser() { return mockCurrentUser; },
    get isAuthenticated() { return true; },
    signOut: (...args: unknown[]) => mockSignOut(...args),
    addOrg: (...args: unknown[]) => mockAddOrg(...args),
  },
}));

// ── Helpers ──────────────────────────────────────────────────────────────────

function setOrgs(orgs: typeof mockOrgs) {
  mockOrgs = orgs;
}

function setCurrentUser(user: typeof mockCurrentUser) {
  mockCurrentUser = user;
}

// ── Tests ─────────────────────────────────────────────────────────────────────

describe('Home', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockOrgs = null;
    mockCurrentUser = null;
  });

  afterEach(() => {
    cleanup();
  });

  // ── Loading state ────────────────────────────────────────────────────────

  it('renders loading state when auth.orgs is null', () => {
    setOrgs(null);
    render(Home);
    const el = screen.getByText('Loading your workspaces');
    expect(el).toBeInTheDocument();
    expect(el).toHaveAttribute('aria-busy', 'true');
  });

  it('loading state has no org list and no create form', () => {
    setOrgs(null);
    render(Home);
    expect(screen.queryByRole('list')).not.toBeInTheDocument();
    expect(screen.queryByLabelText(/name your org|create another org/i)).not.toBeInTheDocument();
  });

  // ── Empty state ──────────────────────────────────────────────────────────

  it('renders empty state heading when auth.orgs is empty', () => {
    setOrgs([]);
    render(Home);
    expect(screen.getByText(/Welcome to jamsesh/)).toBeInTheDocument();
  });

  it('empty state shows welcome paragraph', () => {
    setOrgs([]);
    render(Home);
    expect(screen.getByText(/not in any orgs yet/)).toBeInTheDocument();
  });

  it('empty state renders the create form with "Name your org" label', () => {
    setOrgs([]);
    render(Home);
    expect(screen.getByLabelText('Name your org')).toBeInTheDocument();
  });

  it('empty state has no org list', () => {
    setOrgs([]);
    render(Home);
    expect(screen.queryByRole('list')).not.toBeInTheDocument();
  });

  it('empty state includes user display name in heading when currentUser is set', () => {
    setOrgs([]);
    setCurrentUser({ id: 'u1', email: 'ada@test.com', displayName: 'Ada' });
    render(Home);
    expect(screen.getByText(/Welcome to jamsesh, Ada/)).toBeInTheDocument();
  });

  // ── Single-org auto-route ────────────────────────────────────────────────

  it('navigates to the single org sessions page when auth.orgs has exactly one entry', async () => {
    setOrgs([{ id: 'org-1', name: 'acme', slug: 'acme', role: 'creator' }]);
    render(Home);
    await waitFor(() => {
      expect(mockNavigate).toHaveBeenCalledWith('/orgs/org-1/sessions');
    });
  });

  it('does not render the picker when auto-routing', () => {
    setOrgs([{ id: 'org-1', name: 'acme', slug: 'acme', role: 'creator' }]);
    render(Home);
    // The picker heading "Pick a workspace" should not render for single-org.
    expect(screen.queryByText('Pick a workspace')).not.toBeInTheDocument();
  });

  // ── Picker state (2+ orgs) ───────────────────────────────────────────────

  it('renders picker heading when auth.orgs has two or more entries', () => {
    setOrgs([
      { id: 'org-1', name: 'acme', slug: 'acme', role: 'creator' },
      { id: 'org-2', name: 'hooli', slug: 'hooli', role: 'member' },
    ]);
    render(Home);
    expect(screen.getByText('Pick a workspace')).toBeInTheDocument();
  });

  it('renders one list item per org', () => {
    setOrgs([
      { id: 'org-1', name: 'acme', slug: 'acme', role: 'creator' },
      { id: 'org-2', name: 'hooli', slug: 'hooli', role: 'member' },
      { id: 'org-3', name: 'pied-piper', slug: 'pied-piper', role: 'member' },
    ]);
    render(Home);
    const items = screen.getAllByRole('listitem');
    expect(items).toHaveLength(3);
  });

  it('each org row renders org name', () => {
    setOrgs([
      { id: 'org-1', name: 'acme', slug: 'acme', role: 'creator' },
      { id: 'org-2', name: 'hooli', slug: 'hooli', role: 'member' },
    ]);
    render(Home);
    expect(screen.getByText('acme')).toBeInTheDocument();
    expect(screen.getByText('hooli')).toBeInTheDocument();
  });

  it('each org row renders the slug path', () => {
    setOrgs([
      { id: 'org-1', name: 'acme', slug: 'acme', role: 'creator' },
      { id: 'org-2', name: 'hooli', slug: 'hooli', role: 'member' },
    ]);
    render(Home);
    expect(screen.getByText('/orgs/acme/sessions')).toBeInTheDocument();
  });

  it('each org row has an href pointing to the org sessions page', () => {
    setOrgs([
      { id: 'org-1', name: 'acme', slug: 'acme', role: 'creator' },
      { id: 'org-2', name: 'hooli', slug: 'hooli', role: 'member' },
    ]);
    render(Home);
    const links = screen.getAllByRole('link');
    expect(links[0]).toHaveAttribute('href', '/orgs/org-1/sessions');
    expect(links[1]).toHaveAttribute('href', '/orgs/org-2/sessions');
  });

  it('clicking an org row navigates via navigate() and prevents default', async () => {
    // Need 2 orgs so picker renders (single-org auto-routes)
    setOrgs([
      { id: 'org-1', name: 'acme', slug: 'acme', role: 'creator' },
      { id: 'org-2', name: 'hooli', slug: 'hooli', role: 'member' },
    ]);
    render(Home);
    const link = screen.getAllByRole('link')[0];
    await fireEvent.click(link);
    expect(mockNavigate).toHaveBeenCalledWith('/orgs/org-1/sessions');
  });

  // ── Role badges ──────────────────────────────────────────────────────────

  it('renders Creator badge for creator role (title-cased)', () => {
    setOrgs([
      { id: 'org-1', name: 'acme', slug: 'acme', role: 'creator' },
      { id: 'org-2', name: 'hooli', slug: 'hooli', role: 'member' },
    ]);
    render(Home);
    expect(screen.getByText('Creator')).toBeInTheDocument();
    expect(screen.getByText('Member')).toBeInTheDocument();
  });

  it('creator badge has role-creator CSS class', () => {
    setOrgs([
      { id: 'org-1', name: 'acme', slug: 'acme', role: 'creator' },
      { id: 'org-2', name: 'hooli', slug: 'hooli', role: 'member' },
    ]);
    render(Home);
    const creatorBadge = screen.getByText('Creator');
    expect(creatorBadge).toHaveClass('role-creator');
  });

  it('non-creator badge does not have role-creator class', () => {
    setOrgs([
      { id: 'org-1', name: 'acme', slug: 'acme', role: 'creator' },
      { id: 'org-2', name: 'hooli', slug: 'hooli', role: 'member' },
    ]);
    render(Home);
    const memberBadge = screen.getByText('Member');
    expect(memberBadge).not.toHaveClass('role-creator');
  });

  it('title-cases arbitrary unknown role values', () => {
    setOrgs([
      { id: 'org-1', name: 'acme', slug: 'acme', role: 'creator' },
      { id: 'org-2', name: 'hooli', slug: 'hooli', role: 'reviewer' },
    ]);
    render(Home);
    expect(screen.getByText('Reviewer')).toBeInTheDocument();
    expect(screen.getByText('Reviewer')).not.toHaveClass('role-creator');
  });

  // ── Picker: create form ──────────────────────────────────────────────────

  it('picker state renders the create form with "Create another org" label', () => {
    setOrgs([
      { id: 'org-1', name: 'acme', slug: 'acme', role: 'creator' },
      { id: 'org-2', name: 'hooli', slug: 'hooli', role: 'member' },
    ]);
    render(Home);
    expect(screen.getByLabelText('Create another org')).toBeInTheDocument();
  });

  it('picker state shows "or" divider between the list and create form', () => {
    setOrgs([
      { id: 'org-1', name: 'acme', slug: 'acme', role: 'creator' },
      { id: 'org-2', name: 'hooli', slug: 'hooli', role: 'member' },
    ]);
    render(Home);
    expect(screen.getByText('or')).toBeInTheDocument();
  });

  it('picker state submit also trims the name before posting', async () => {
    setOrgs([
      { id: 'org-1', name: 'acme', slug: 'acme', role: 'creator' },
      { id: 'org-2', name: 'hooli', slug: 'hooli', role: 'member' },
    ]);
    mockPOST.mockResolvedValue({ data: { id: 'n', name: 'foo', slug: 'foo' }, error: undefined });
    render(Home);
    const input = screen.getByLabelText('Create another org') as HTMLInputElement;
    input.value = '  foo  ';
    await fireEvent.input(input);
    await fireEvent.submit(input.closest('form')!);
    await waitFor(() => expect(mockPOST).toHaveBeenCalledWith('/api/orgs', { body: { name: 'foo' } }));
  });

  // ── Create org — success ─────────────────────────────────────────────────

  it('submitting a non-empty name calls POST /api/orgs with the trimmed name', async () => {
    setOrgs([]);
    mockPOST.mockResolvedValue({
      data: { id: 'new-org', name: 'acme', slug: 'acme' },
      error: undefined,
    });

    render(Home);
    const input = screen.getByLabelText('Name your org') as HTMLInputElement;
    input.value = '  acme  ';
    await fireEvent.input(input);

    const form = input.closest('form')!;
    await fireEvent.submit(form);

    await waitFor(() => {
      expect(mockPOST).toHaveBeenCalledWith('/api/orgs', { body: { name: 'acme' } });
    });
  });

  it('on 201 success calls auth.addOrg with id/name/slug and role creator', async () => {
    setOrgs([]);
    mockPOST.mockResolvedValue({
      data: { id: 'new-org', name: 'acme', slug: 'acme' },
      error: undefined,
    });

    render(Home);
    const input = screen.getByLabelText('Name your org') as HTMLInputElement;
    input.value = 'acme';
    await fireEvent.input(input);

    const form = input.closest('form')!;
    await fireEvent.submit(form);

    await waitFor(() => {
      expect(mockAddOrg).toHaveBeenCalledWith({
        id: 'new-org',
        name: 'acme',
        slug: 'acme',
        role: 'creator',
      });
    });
  });

  it('on 201 success navigates to the new org sessions page', async () => {
    setOrgs([]);
    mockPOST.mockResolvedValue({
      data: { id: 'new-org', name: 'acme', slug: 'acme' },
      error: undefined,
    });

    render(Home);
    const input = screen.getByLabelText('Name your org') as HTMLInputElement;
    input.value = 'acme';
    await fireEvent.input(input);

    const form = input.closest('form')!;
    await fireEvent.submit(form);

    await waitFor(() => {
      expect(mockNavigate).toHaveBeenCalledWith('/orgs/new-org/sessions');
    });
  });

  it('fires exactly one POST per submit', async () => {
    setOrgs([]);
    mockPOST.mockResolvedValue({
      data: { id: 'new-org', name: 'acme', slug: 'acme' },
      error: undefined,
    });

    render(Home);
    const input = screen.getByLabelText('Name your org') as HTMLInputElement;
    input.value = 'acme';
    await fireEvent.input(input);

    const form = input.closest('form')!;
    await fireEvent.submit(form);
    await fireEvent.submit(form);

    await waitFor(() => {
      expect(mockPOST).toHaveBeenCalledTimes(1);
    });
  });

  // ── Create org — error ───────────────────────────────────────────────────

  it('on server error sets createState to create-error and shows role="alert" message', async () => {
    setOrgs([]);
    mockPOST.mockResolvedValue({
      data: undefined,
      error: { message: 'Name already taken' },
    });

    render(Home);
    const input = screen.getByLabelText('Name your org') as HTMLInputElement;
    input.value = 'acme';
    await fireEvent.input(input);

    const form = input.closest('form')!;
    await fireEvent.submit(form);

    await waitFor(() => {
      const alert = screen.getByRole('alert');
      expect(alert).toBeInTheDocument();
      expect(alert).toHaveTextContent('Name already taken');
    });
  });

  it('on server error re-enables the Create org button', async () => {
    setOrgs([]);
    mockPOST.mockResolvedValue({ data: undefined, error: { message: 'oops' } });

    render(Home);
    const input = screen.getByLabelText('Name your org') as HTMLInputElement;
    input.value = 'acme';
    await fireEvent.input(input);

    const form = input.closest('form')!;
    await fireEvent.submit(form);

    await waitFor(() => {
      const btn = screen.getByRole('button', { name: /create org/i });
      expect(btn).not.toBeDisabled();
    });
  });

  it('on network failure sets createState to create-error with a server-unreachable message', async () => {
    setOrgs([]);
    mockPOST.mockRejectedValue(new TypeError('Failed to fetch'));

    render(Home);
    const input = screen.getByLabelText('Name your org') as HTMLInputElement;
    input.value = 'acme';
    await fireEvent.input(input);

    const form = input.closest('form')!;
    await fireEvent.submit(form);

    await waitFor(() => {
      const alert = screen.getByRole('alert');
      expect(alert).toBeInTheDocument();
      expect(alert).toHaveTextContent(/Could not reach the server/);
    });
  });

  // ── Create org — empty name guard ────────────────────────────────────────

  it('does not fire POST when org name is empty', async () => {
    setOrgs([]);
    render(Home);
    const form = screen.getByLabelText('Name your org').closest('form')!;
    await fireEvent.submit(form);
    expect(mockPOST).not.toHaveBeenCalled();
  });

  it('does not fire POST when org name is whitespace-only', async () => {
    setOrgs([]);
    render(Home);
    const input = screen.getByLabelText('Name your org') as HTMLInputElement;
    input.value = '   ';
    await fireEvent.input(input);

    const form = input.closest('form')!;
    await fireEvent.submit(form);
    expect(mockPOST).not.toHaveBeenCalled();
  });

  // ── Sign out ─────────────────────────────────────────────────────────────

  it('clicking Sign out calls auth.signOut()', async () => {
    setOrgs([]);
    render(Home);
    const btn = screen.getByRole('button', { name: /sign out/i });
    await fireEvent.click(btn);
    expect(mockSignOut).toHaveBeenCalledOnce();
  });

  // ── Topbar ───────────────────────────────────────────────────────────────

  it('renders the jamsesh wordmark in the topbar', () => {
    setOrgs([]);
    render(Home);
    // The wordmark is a div containing "jam" + span "·" + "sesh"; query by role/class
    // using querySelector for precision since "jam" also appears in the heading text.
    const wordmark = document.querySelector('.wordmark');
    expect(wordmark).toBeInTheDocument();
    expect(wordmark!.textContent).toMatch(/jamsesh|jam.*sesh/);
  });

  it('shows user email in topbar when currentUser is set', () => {
    setOrgs([]);
    setCurrentUser({ id: 'u1', email: 'ada@test.com', displayName: 'Ada' });
    render(Home);
    expect(screen.getByText('ada@test.com')).toBeInTheDocument();
  });
});
