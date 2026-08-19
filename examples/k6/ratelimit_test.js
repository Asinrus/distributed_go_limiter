import http from 'k6/http';
import { check } from 'k6';
import { Counter } from 'k6/metrics';

const BASE_URL = __ENV.BASE_URL || 'http://localhost:8080';

export const accepted_total = new Counter('accepted_total');
export const rejected_total = new Counter('rejected_total');

export const options = {
  scenarios: {
    sequential: {
      executor: 'shared-iterations',
      vus: 1,
      iterations: 8,
      maxDuration: '30s',
    },
  },
  thresholds: {
    accepted_total: ['count==5'],
    rejected_total: ['count==3'],
  },
};

export default function () {
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
