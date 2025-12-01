// tests/load/load_test.js
import http from 'k6/http';
import { check, sleep } from 'k6';
import { Rate } from 'k6/metrics';

const errorRate = new Rate('errors');

export const options = {
  stages: [
    { duration: '2m', target: 100 },  // Ramp up
    { duration: '5m', target: 100 },  // Stay at 100 users
    { duration: '2m', target: 200 },  // Ramp to 200
    { duration: '5m', target: 200 },  // Stay at 200
    { duration: '2m', target: 0 },    // Ramp down
  ],
  thresholds: {
    http_req_duration: ['p(95)<500', 'p(99)<1000'],
    http_req_failed: ['rate<0.01'],
    errors: ['rate<0.05'],
  },
};

const API_BASE_URL = __ENV.API_BASE_URL || 'http://localhost:8000';

export default function () {
  // Search catalog
  const searchRes = http.post(
    `${API_BASE_URL}/api/v1/catalog/search`,
    JSON.stringify({ query: 'pride' }),
    { headers: { 'Content-Type': 'application/json' } }
  );

  check(searchRes, {
    'search status 200': (r) => r.status === 200,
  }) || errorRate.add(1);

  sleep(1);

  // Get random item from search results
  const items = JSON.parse(searchRes.body).items;
  if (items && items.length > 0) {
    const itemId = items[0].id;
    const itemRes = http.get(`${API_BASE_URL}/api/v1/catalog/items/${itemId}`);

    check(itemRes, {
      'item fetch status 200': (r) => r.status === 200,
    }) || errorRate.add(1);
  }

  sleep(2);
}
