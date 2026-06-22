import { beforeEach, describe, expect, test } from 'bun:test';

import { useAgentStore } from './store';

function resetStore() {
  useAgentStore.setState({
    clientId: null,
    active: false,
    agentPresent: false,
    connectionState: 'connecting',
    lastRun: null,
  });
}

describe('useAgentStore', () => {
  beforeEach(resetStore);

  test('setStatus records agent presence, clientId, and active flag', () => {
    useAgentStore.getState().setStatus('client-1', true);
    const s = useAgentStore.getState();
    expect(s.clientId).toBe('client-1');
    expect(s.active).toBe(true);
    expect(s.agentPresent).toBe(true);
    expect(s.connectionState).toBe('connected');
  });

  test('setDisconnected clears agent state', () => {
    useAgentStore.getState().setStatus('client-1', true);
    useAgentStore.getState().setDisconnected();
    const s = useAgentStore.getState();
    expect(s.clientId).toBeNull();
    expect(s.active).toBe(false);
    expect(s.agentPresent).toBe(false);
    expect(s.connectionState).toBe('disconnected');
  });

  test('setConnected clears stale agent state from a prior bridge session', () => {
    // Simulate a bridge-backed tab that disconnected, then reconnected to a
    // plain `ghost serve` (liveness-only stream, no status event follows).
    useAgentStore.getState().setStatus('client-1', false);
    useAgentStore.getState().setDisconnected();
    useAgentStore.getState().setConnected();
    const s = useAgentStore.getState();
    // Without clearing on connect, the UI would keep showing "agent active in
    // another tab" with a stale clientId even though no agent is present.
    expect(s.clientId).toBeNull();
    expect(s.active).toBe(false);
    expect(s.agentPresent).toBe(false);
    expect(s.connectionState).toBe('connected');
  });

  test('a status event after setConnected restores agent presence', () => {
    useAgentStore.getState().setConnected();
    useAgentStore.getState().setStatus('client-2', true);
    const s = useAgentStore.getState();
    expect(s.clientId).toBe('client-2');
    expect(s.active).toBe(true);
    expect(s.agentPresent).toBe(true);
  });
});
