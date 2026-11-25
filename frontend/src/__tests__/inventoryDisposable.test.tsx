import React from 'react';
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import InventoryView from '../components/InventoryView';
import { InventoryServer } from '../api';

const baseServer = (overrides: Partial<InventoryServer>): InventoryServer => ({
  id: 'srv1',
  name: 'host-1',
  ip: '10.0.0.1',
  description: '',
  createdAt: 1,
  updatedAt: 1,
  ...overrides,
});

const renderView = (ui: Partial<React.ComponentProps<typeof InventoryView>> = {}) => {
  const props = {
    servers: [] as InventoryServer[],
    disposableAvailable: false,
    onCreateDisposable: vi.fn().mockResolvedValue(undefined),
    onAddServer: vi.fn().mockResolvedValue(undefined),
    onUpdateServer: vi.fn().mockResolvedValue(undefined),
    onDeleteServer: vi.fn().mockResolvedValue(undefined),
    onConnectServer: vi.fn(),
    ...ui,
  };
  render(<InventoryView {...props} />);
  return props;
};

describe('InventoryView disposable containers', () => {
  beforeEach(() => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({ ok: true }));
  });
  afterEach(() => {
    cleanup();
    vi.unstubAllGlobals();
  });

  it('marks disposable rows as temporary', async () => {
    const props = renderView({
      servers: [
        baseServer({ id: 'd1', name: 'disposable-abc', ip: '172.18.0.5', isDisposable: true, containerId: 'c1' }),
        baseServer({ id: 's1' }),
      ],
    });

    const disposableRow = screen.getByText('disposable-abc').closest('.inventory-card');
    const regularRow = screen.getByText('host-1').closest('.inventory-card');
    expect(disposableRow?.className).toContain('is-disposable');
    expect(regularRow?.className).not.toContain('is-disposable');
    expect(screen.getByText('TEMP')).toBeTruthy();

    await waitFor(() => expect(screen.getByRole('button', { name: 'Connect' })).toBeTruthy());
    expect(screen.getByRole('button', { name: 'Destroy' })).toBeTruthy();
    expect(screen.queryByRole('button', { name: 'Edit' })).toBeTruthy(); // only the regular row's
    expect(screen.getAllByRole('button', { name: 'Edit' })).toHaveLength(1);
    expect(screen.getAllByRole('button', { name: 'Delete' })).toHaveLength(1);

    fireEvent.click(screen.getByRole('button', { name: 'Destroy' }));
    await waitFor(() => expect(props.onDeleteServer).toHaveBeenCalledWith('d1'));
  });

  it('enables the create button only when the deployment supports it', async () => {
    const unavailable = renderView({ disposableAvailable: false });
    const button = screen.getByRole('button', { name: /Disposable container/ });
    expect((button as HTMLButtonElement).disabled).toBe(true);
    fireEvent.click(button);
    expect(unavailable.onCreateDisposable).not.toHaveBeenCalled();
  });

  it('shows a disposable as out of service while it is being destroyed', async () => {
    let finish: () => void = () => {};
    const onDelete = vi.fn(
      () =>
        new Promise<void>((resolve) => {
          finish = resolve;
        }),
    );
    renderView({
      servers: [baseServer({ id: 'd1', name: 'disposable-abc', ip: '172.18.0.5', isDisposable: true })],
      onDeleteServer: onDelete,
    });
    await waitFor(() => expect(screen.getByRole('button', { name: 'Connect' })).toBeTruthy());

    fireEvent.click(screen.getByRole('button', { name: 'Destroy' }));
    const row = screen.getByText('disposable-abc').closest('.inventory-card');
    expect(row?.className).toContain('is-destroying');
    expect(row?.getAttribute('aria-disabled')).toBe('true');
    // Nothing left to click while the container is being removed.
    expect(screen.queryByRole('button', { name: 'Connect' })).toBeNull();
    expect(screen.queryByRole('button', { name: 'Destroy' })).toBeNull();
    expect(screen.getByRole('status').textContent).toContain('Destroying');

    fireEvent.click(row as HTMLElement);
    expect(onDelete).toHaveBeenCalledTimes(1);

    finish();
    await waitFor(() => expect(screen.getByText('disposable-abc').closest('.inventory-card')?.className).not.toContain('is-destroying'));
  });

  it('makes a row usable again when destroying fails', async () => {
    renderView({
      servers: [baseServer({ id: 'd1', name: 'disposable-abc', ip: '172.18.0.5', isDisposable: true })],
      onDeleteServer: vi.fn().mockRejectedValue(new Error('docker is down')),
    });
    await waitFor(() => expect(screen.getByRole('button', { name: 'Connect' })).toBeTruthy());
    fireEvent.click(screen.getByRole('button', { name: 'Destroy' }));

    await waitFor(() => expect(screen.getByText('docker is down')).toBeTruthy());
    expect(screen.getByText('disposable-abc').closest('.inventory-card')?.className).not.toContain('is-destroying');
    expect(screen.getByRole('button', { name: 'Destroy' })).toBeTruthy();
  });

  it('creates a disposable container when the button is used', async () => {
    const props = renderView({ disposableAvailable: true });
    fireEvent.click(screen.getByRole('button', { name: /Disposable container/ }));
    await waitFor(() => expect(props.onCreateDisposable).toHaveBeenCalledTimes(1));
  });
});
