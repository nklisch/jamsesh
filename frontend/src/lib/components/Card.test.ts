import { describe, it, expect } from 'vitest';
import { render, screen } from '@testing-library/svelte';
import Card from './Card.svelte';

describe('Card', () => {
  it('renders children inside a div', () => {
    render(Card, { props: { children: () => 'Card content' } });
    expect(screen.getByText('Card content')).toBeInTheDocument();
  });

  it('defaults to padding=md', () => {
    render(Card, { props: { children: () => 'Content' } });
    const card = screen.getByText('Content').closest('.card');
    expect(card).toHaveClass('card-p-md');
  });

  it('applies padding=sm class', () => {
    render(Card, { props: { padding: 'sm', children: () => 'Content' } });
    const card = screen.getByText('Content').closest('.card');
    expect(card).toHaveClass('card-p-sm');
  });

  it('applies padding=lg class', () => {
    render(Card, { props: { padding: 'lg', children: () => 'Content' } });
    const card = screen.getByText('Content').closest('.card');
    expect(card).toHaveClass('card-p-lg');
  });

  it('always has the base card class', () => {
    render(Card, { props: { children: () => 'Content' } });
    const card = screen.getByText('Content').closest('.card');
    expect(card).toHaveClass('card');
  });
});
