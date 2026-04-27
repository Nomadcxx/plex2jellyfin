import { describe, expect, it } from 'vitest';
import { render, screen } from '@testing-library/react';
import { ProgressCard } from './ProgressCard';

describe('ProgressCard', () => {
  it('renders phase and percentage', () => {
    render(
      <ProgressCard
        title="Re-scan"
        events={[
          { type: 'progress', phase: 'walking', msg: '/storage', current: 100, total: 1000 },
        ]}
      />,
    );
    expect(screen.getByText(/walking/i)).toBeInTheDocument();
    expect(screen.getByText(/10%/)).toBeInTheDocument();
  });

  it('shows done state', () => {
    render(<ProgressCard title="Re-scan" events={[{ type: 'done' }]} />);
    expect(screen.getByText(/complete/i)).toBeInTheDocument();
  });
});
