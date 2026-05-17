// NotFound.test.ts

import { describe, it, expect, vi, afterEach } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/svelte';
import NotFound from './NotFound.svelte';

vi.mock('$lib/router.svelte', () => ({
  navigate: vi.fn(),
  current: { name: 'not-found', params: {} },
}));

describe('NotFound', () => {
  afterEach(() => {
    vi.clearAllMocks();
  });

  it('renders a 404 code', () => {
    render(NotFound);
    expect(screen.getByText('404')).toBeInTheDocument();
  });

  it('renders "Page not found" heading', () => {
    render(NotFound);
    expect(screen.getByRole('heading', { name: /page not found/i })).toBeInTheDocument();
  });

  it('renders a link to /login', () => {
    render(NotFound);
    const link = screen.getByRole('link', { name: /go to sign in/i });
    expect(link).toBeInTheDocument();
    expect(link).toHaveAttribute('href', '/login');
  });

  it('clicking the link calls navigate("/login")', async () => {
    const { navigate } = await import('$lib/router.svelte');
    render(NotFound);

    const link = screen.getByRole('link', { name: /go to sign in/i });
    await fireEvent.click(link);

    expect(navigate).toHaveBeenCalledWith('/login');
  });
});
