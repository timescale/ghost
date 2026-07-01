import { ResultsCacheContext } from '@timescale/popsql-query-widget-cdn';
import { useContext } from 'react';

import type { ResultsCacheClient } from './runData';

// useResultsCacheClient returns the widget's in-process results-cache client
// from context, or null before it has initialized. The widget package doesn't
// re-export the context's value type, so this centralizes the cast every
// consumer would otherwise repeat. It lives apart from runData.ts (which is
// import-safe for pure unit tests) because importing the widget package pulls
// in browser-only globals.
export function useResultsCacheClient(): ResultsCacheClient | null {
  const { client } = useContext(ResultsCacheContext) as {
    client: ResultsCacheClient | null;
  };
  return client;
}
