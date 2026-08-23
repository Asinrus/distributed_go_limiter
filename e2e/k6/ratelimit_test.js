import http from 'k6/http';
import { check, sleep } from 'k6';
import { Counter } from 'k6/metrics';

const BASE_URL = __ENV.BASE_URL || 'http://localhost:8080';

export const accepted_total = new Counter('accepted_total');
export const rejected_total = new Counter('rejected_total');

// Every examples/* demo is configured with the same bucket: capacity 5,
// refilling 5 tokens/min greedy (1 token per 12s) — see examples/*/main.go.
export const options = {
  scenarios: {
    // Fully self-contained: fires 8 requests cold and asserts the burst
    // limit. Assumes nothing about any other scenario.
    burst: {
      executor: 'shared-iterations',
      exec: 'burst',
      vus: 1,
      iterations: 8,
      maxDuration: '30s',
    },
    // Also fully self-contained: drains the bucket itself first (its own
    // "activity"), then waits past one refill period, then asserts the
    // refilled token is served and the bucket is empty again right after.
    // Does not assume burst ran, ran first, or left the bucket in any
    // particular state -- if this scenario were the only one in the file,
    // it would still pass. startTime only avoids running concurrently
    // against burst's shared bucket (same source IP); it is a scheduling
    // necessity, not a logical dependency on burst's outcome.
    refill: {
      executor: 'shared-iterations',
      exec: 'refill',
      vus: 1,
      iterations: 1,
      startTime: '30s',
      maxDuration: '30s',
    },
  },
  thresholds: {
    accepted_total: ['count==5'],
    rejected_total: ['count==3'],
    'checks{scenario:refill}': ['rate==1'],
  },
};

export function burst() {
  const res = http.get(`${BASE_URL}/`);
  check(res, {
    'status is 200 or 429': (r) => r.status === 200 || r.status === 429,
  });
  if (res.status === 200) {
    accepted_total.add(1);
  } else if (res.status === 429) {
    rejected_total.add(1);
  }
}

export function refill() {
  // Drain the bucket ourselves first -- capacity is 5, so 5 requests
  // guarantee an empty bucket regardless of starting state. Outcomes here
  // aren't asserted: the starting fullness is unknown by design, since this
  // scenario doesn't assume burst (or anything else) already ran.
  for (let i = 0; i < 5; i++) {
    http.get(`${BASE_URL}/`);
  }

  sleep(15); // past one refill period (12s) + margin

  const refilled = http.get(`${BASE_URL}/`);
  check(refilled, { 'refilled token is accepted': (r) => r.status === 200 });

  const emptyAgain = http.get(`${BASE_URL}/`);
  check(emptyAgain, { 'bucket empty again right after': (r) => r.status === 429 });
}
