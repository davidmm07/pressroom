import { Client, cacheExchange, fetchExchange } from 'urql';

const OPERATOR_KEY = 'pressroom.operator';

/** The operator's email names who approves and reviews. Behind Google IAP the
 * server takes it from the verified identity instead and ignores this. */
export function getOperator(): string {
  try {
    return localStorage.getItem(OPERATOR_KEY) ?? 'cx-lead@example.com';
  } catch {
    return 'cx-lead@example.com';
  }
}

export function setOperator(email: string): void {
  try {
    localStorage.setItem(OPERATOR_KEY, email);
  } catch {
    // Storage can be unavailable (private windows); the default still works.
  }
}

export const client = new Client({
  url: import.meta.env.VITE_GRAPHQL_URL ?? '/graphql',
  exchanges: [cacheExchange, fetchExchange],
  fetchOptions: () => {
    const headers: Record<string, string> = { 'X-Pressroom-Actor': getOperator() };
    const token = import.meta.env.VITE_API_TOKEN;
    if (token) headers.Authorization = `Bearer ${token}`;
    return { headers };
  },
});
