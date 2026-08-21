import http from 'k6/http';
import { check } from 'k6';
import { Counter } from 'k6/metrics';

const BASE_URL = __ENV.BASE_URL || 'http://localhost:8080';

export const accepted_total = new Counter('accepted_total');
export const rejected_total = new Counter('rejected_total');

// Every examples/* demo is configured with the same bucket: capacity 5,
// refilling 5 tokens/min greedy (1 token per 12s) — see examples/*/main.go.
export const options = {
  scenarios: {
    // Drains the bucket: 8 requests against a capacity of 5.
    burst: {
      executor: 'shared-iterations',
      exec: 'burst',
      vus: 1,
      iterations: 8,
      maxDuration: '30s',
    },
    // Runs after enough wall-clock time has passed for one token to refill
    // (burst itself finishes in well under a second, so startTime is really
    // "time since the bucket was drained" plus a safety margin over the 12s
    // refill period). First request should get the refilled token, the
    // second should find the bucket empty again.
    refill: {
      executor: 'shared-iterations',
      exec: 'refill',
      vus: 1,
      iterations: 2,
      startTime: '15s',
      maxDuration: '10s',
    },
  },
  thresholds: {
    accepted_total: ['count==6'],
    rejected_total: ['count==4'],
  },
};

function hit() {
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

export function burst() {
  hit();
}

export function refill() {
  hit();
}
