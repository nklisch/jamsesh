import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/svelte';
import ThemeToggle from './ThemeToggle.svelte';

// Mock localStorage
const localStorageMock = (() => {
  let store: Record<string, string> = {};
  return {
    getItem: (key: string) => store[key] ?? null,
    setItem: (key: string, value: string) => { store[key] = value; },
    removeItem: (key: string) => { delete store[key]; },
    clear: () => { store = {}; },
  };
})();

// Spy on document.documentElement setAttribute / removeAttribute
const setAttributeSpy = vi.spyOn(document.documentElement, 'setAttribute');
const removeAttributeSpy = vi.spyOn(document.documentElement, 'removeAttribute');

beforeEach(() => {
  localStorageMock.clear();
  Object.defineProperty(window, 'localStorage', {
    value: localStorageMock,
    writable: true,
  });
  setAttributeSpy.mockClear();
  removeAttributeSpy.mockClear();
});

afterEach(() => {
  document.documentElement.removeAttribute('data-theme');
});

describe('ThemeToggle', () => {
  it('renders a button with aria-label indicating system theme by default', () => {
    render(ThemeToggle, {});
    expect(screen.getByRole('button', { name: 'System theme' })).toBeInTheDocument();
  });

  it('cycles system → light on first click', async () => {
    render(ThemeToggle, {});
    await fireEvent.click(screen.getByRole('button'));
    expect(screen.getByRole('button', { name: 'Light theme' })).toBeInTheDocument();
  });

  it('cycles light → dark on second click', async () => {
    render(ThemeToggle, {});
    const btn = screen.getByRole('button');
    await fireEvent.click(btn); // system → light
    await fireEvent.click(btn); // light → dark
    expect(screen.getByRole('button', { name: 'Dark theme' })).toBeInTheDocument();
  });

  it('cycles dark → system on third click', async () => {
    render(ThemeToggle, {});
    const btn = screen.getByRole('button');
    await fireEvent.click(btn); // system → light
    await fireEvent.click(btn); // light → dark
    await fireEvent.click(btn); // dark → system
    expect(screen.getByRole('button', { name: 'System theme' })).toBeInTheDocument();
  });

  it('persists theme to localStorage on click', async () => {
    render(ThemeToggle, {});
    await fireEvent.click(screen.getByRole('button')); // → light
    expect(localStorageMock.getItem('jamsesh.theme')).toBe('light');
  });

  it('applies data-theme="light" to <html> when cycling to light', async () => {
    render(ThemeToggle, {});
    await fireEvent.click(screen.getByRole('button')); // → light
    expect(setAttributeSpy).toHaveBeenCalledWith('data-theme', 'light');
  });

  it('applies data-theme="dark" to <html> when cycling to dark', async () => {
    render(ThemeToggle, {});
    const btn = screen.getByRole('button');
    await fireEvent.click(btn); // → light
    await fireEvent.click(btn); // → dark
    expect(setAttributeSpy).toHaveBeenCalledWith('data-theme', 'dark');
  });

  it('removes data-theme from <html> when cycling back to system', async () => {
    render(ThemeToggle, {});
    const btn = screen.getByRole('button');
    await fireEvent.click(btn); // → light
    await fireEvent.click(btn); // → dark
    await fireEvent.click(btn); // → system
    expect(removeAttributeSpy).toHaveBeenCalledWith('data-theme');
  });

  it('reads saved theme from localStorage on mount', async () => {
    localStorageMock.setItem('jamsesh.theme', 'dark');
    render(ThemeToggle, {});
    // After mount with saved 'dark', button should show Dark theme label
    // (onMount is async; check after a tick)
    await new Promise((r) => setTimeout(r, 0));
    expect(screen.getByRole('button', { name: 'Dark theme' })).toBeInTheDocument();
  });
});
